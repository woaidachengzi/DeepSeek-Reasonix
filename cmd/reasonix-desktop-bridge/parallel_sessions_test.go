package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

type parallelHTTPRuntime struct{ *bridgeTestRuntime }

func (r *parallelHTTPRuntime) Submit(input string) {
	r.submits = append(r.submits, input)
	r.state = "running"
}
func (r *parallelHTTPRuntime) Cancel() { r.cancelCalls++; r.state = "idle" }

func TestBridgeParallelSessionsRouteBackgroundRequests(t *testing.T) {
	a := &parallelHTTPRuntime{&bridgeTestRuntime{path: "/sessions/a.jsonl", state: "idle", history: []desktopbridge.HistoryMessage{{Role: "user", Content: "A history"}}}}
	b := &parallelHTTPRuntime{&bridgeTestRuntime{path: "/sessions/b.jsonl", state: "idle", history: []desktopbridge.HistoryMessage{{Role: "user", Content: "B history"}}}}
	pool := desktopbridge.NewRuntimePool(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, req desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		if req.SessionID == "a" {
			return a, nil
		}
		return b, nil
	}))
	defer pool.Shutdown()
	bridge := newBridgeServer(testToken, "parallel-instance", pool)
	handler := bridge.handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	for _, id := range []string{"a", "b"} {
		if r := call("POST", "/v1/sessions:switch", `{"sessionId":"`+id+`"}`); r.Code != 200 {
			t.Fatalf("switch: %d %s", r.Code, r.Body.String())
		}
		if r := call("POST", "/v1/sessions/"+id+":submit", `{"input":"run"}`); r.Code != 202 {
			t.Fatalf("submit: %d %s", r.Code, r.Body.String())
		}
	}
	for _, id := range []string{"a", "b"} {
		if r := call("GET", "/v1/sessions/"+id+"/snapshot", ""); r.Code != 200 || !strings.Contains(r.Body.String(), `"state":"running"`) {
			t.Fatalf("snapshot: %d %s", r.Code, r.Body.String())
		}
		if r := call("GET", "/v1/sessions/"+id+"/history", ""); r.Code != 200 || !strings.Contains(r.Body.String(), strings.ToUpper(id)+" history") {
			t.Fatalf("history: %d %s", r.Code, r.Body.String())
		}
	}
	var inventory struct {
		Sessions []desktopbridge.SessionView `json:"sessions"`
		Selected string                      `json:"selectedSessionId"`
	}
	live := call("GET", "/v1/sessions:live", "")
	if live.Code != 200 || json.Unmarshal(live.Body.Bytes(), &inventory) != nil || len(inventory.Sessions) != 2 || inventory.Selected != "b" {
		t.Fatal("live inventory omitted background owner", live.Body.String())
	}
	if r := call("POST", "/v1/sessions/a:cancel", `{}`); r.Code != 202 {
		t.Fatalf("background cancel: %d %s", r.Code, r.Body.String())
	}
	if a.state != "idle" || b.state != "running" || b.cancelCalls != 0 || a.shutdownCalls != 0 {
		t.Fatal("background cancel reached another instance")
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/v1/sessions:live", nil))
	if unauthorized.Code != 401 {
		t.Fatal("live inventory bypassed authentication")
	}
	capacity := httptest.NewRecorder()
	bridge.writeRuntimeError(capacity, desktopbridge.ErrRuntimeCapacity, "unused")
	if capacity.Code != 429 || !strings.Contains(capacity.Body.String(), "runtime_capacity") {
		t.Fatal("capacity did not have an explicit response")
	}
}

type parallelTrackingFactory struct {
	*controllerFactory
	controllers map[string]*controllerRuntime
}

func (f *parallelTrackingFactory) Open(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
	raw, err := f.controllerFactory.Open(ctx, request)
	if err == nil {
		f.controllers[request.SessionID] = raw.(*controllerRuntime)
	}
	return raw, err
}

func awaitParallelState(t *testing.T, pool *desktopbridge.RuntimePool, id string, pending bool) desktopbridge.OwnedCommandView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if view, ok := pool.CommandSessionSnapshot(id); ok && (pending && len(view.State.Pending) == 1 || !pending && view.Session.State == "idle") {
			return view
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("parallel owner %s did not reach requested state", id)
	return desktopbridge.OwnedCommandView{}
}

func TestParallelActualControllersPromptsBindingsAndPersistence(t *testing.T) {
	profile := t.TempDir()
	for name, value := range map[string]string{"REASONIX_HOME": profile, "REASONIX_STATE_HOME": profile, "REASONIX_CACHE_HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file"} {
		t.Setenv(name, value)
	}
	fixture, endpoint := newOwnedCommandFixtureProvider(t)
	cfg := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\",\"beta\"]\ndefault=\"alpha\"\n", endpoint)
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	events := desktopbridge.NewEventStream(1024)
	factory := &parallelTrackingFactory{controllerFactory: newControllerFactory(events), controllers: make(map[string]*controllerRuntime)}
	pool := desktopbridge.NewRuntimePool(factory)
	defer pool.Shutdown()
	driver := newPreviewDesktopDriver(pool)
	defer driver.Close()
	route := bot.DesktopWatchRoute{Platform: bot.PlatformFeishu, ConnectionID: "fixture", ChatType: bot.ChatDM, ChatID: "owner"}
	ctx := context.Background()
	if _, err := pool.Open(ctx, desktopbridge.OpenRequest{SessionID: "a", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Submit(ctx, "a", "question A"); err != nil {
		t.Fatal(err)
	}
	callA := awaitOwnedCommand(t, fixture.calls)
	if _, err := driver.ExecuteDesktopCommand(ctx, bot.DesktopCommand{Route: route, ActorID: "owner", Action: "takeover", TargetID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Switch(ctx, desktopbridge.OpenRequest{SessionID: "b", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Submit(ctx, "b", "question B"); err != nil {
		t.Fatal(err)
	}
	callB := awaitOwnedCommand(t, fixture.calls)
	if callA.ctx.Err() != nil || callB.ctx.Err() != nil || !driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("navigation cancelled a turn or revoked exact binding")
	}
	catalogue := newPreviewDesktopCatalogue(pool, nil)
	defer catalogue.Close()
	entries, err := catalogue.Refresh(ctx)
	if err != nil || len(entries) != 2 || !entries[0].running || !entries[1].running || !entries[0].detached {
		t.Fatal("catalogue lost background task", err)
	}
	answersA, answersB := make(chan []event.AskAnswer, 1), make(chan []event.AskAnswer, 1)
	ask := func(id string, call ownedCommandCall, out chan<- []event.AskAnswer) {
		answers, _ := factory.controllers[id].controller.Ask(call.ctx, []event.AskQuestion{{ID: "q", Prompt: "Choose", Options: []event.AskOption{{Label: "A"}, {Label: "B"}}}})
		out <- answers
	}
	go ask("a", callA, answersA)
	go ask("b", callB, answersB)
	pendingA, pendingB := awaitParallelState(t, pool, "a", true), awaitParallelState(t, pool, "b", true)
	if pendingA.Session.State != "paused" || pendingB.Session.State != "paused" {
		t.Fatal("pending questions must expose paused owners")
	}
	prompts := newPreviewDesktopPrompts(pool)
	defer prompts.Close()
	ticketB, _, err := prompts.Issue(ctx, route, "owner", pendingB.Scope, pendingB.State.Pending[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.AnswerQuestion("a", pendingA.State.Pending[0].ID, []desktopbridge.AskAnswer{{QuestionID: "q", Selected: []string{"A"}}}); err != nil {
		t.Fatal(err)
	}
	if answer := awaitOwnedCommand(t, answersA); len(answer) != 1 || answer[0].Selected[0] != "A" {
		t.Fatal("A answer lost")
	}
	select {
	case <-answersB:
		t.Fatal("answer to A resumed B")
	default:
	}
	if _, err := pool.Switch(ctx, desktopbridge.OpenRequest{SessionID: "a", WorkspaceRoot: pendingA.Session.WorkspaceRoot}); err != nil {
		t.Fatal(err)
	}
	sameTicket, _, err := prompts.Issue(ctx, route, "owner", pendingB.Scope, pendingB.State.Pending[0])
	if err != nil || sameTicket != ticketB {
		t.Fatal("navigation revoked B prompt ticket", err)
	}
	if _, err := pool.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-callA.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("A cancel did not reach provider")
	}
	if callB.ctx.Err() != nil {
		t.Fatal("A cancel reached B provider")
	}
	awaitParallelState(t, pool, "a", false)
	if _, err := pool.ApplySettings(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if current, ok := pool.CommandSessionSnapshot("b"); !ok || current.Scope != pendingB.Scope || callB.ctx.Err() != nil {
		t.Fatal("settings apply changed background owner")
	}
	if _, err := pool.AnswerQuestion("b", pendingB.State.Pending[0].ID, []desktopbridge.AskAnswer{{QuestionID: "q", Selected: []string{"B"}}}); err != nil {
		t.Fatal(err)
	}
	if answer := awaitOwnedCommand(t, answersB); len(answer) != 1 || answer[0].Selected[0] != "B" {
		t.Fatal("B answer lost")
	}
	close(fixture.finish)
	awaitParallelState(t, pool, "b", false)
	history, err := pool.History("b")
	if err != nil || len(history.Messages) < 2 || !strings.Contains(history.Messages[len(history.Messages)-1].Content, "owned result") {
		t.Fatal("B completion was not durable", err)
	}
	frames, resync, _, off := events.Subscribe(0)
	defer off()
	asks := map[string]bool{}
	for _, frame := range frames {
		if frame.EventKind == "ask_request" {
			asks[frame.SessionID] = true
		}
	}
	if resync || !asks["a"] || !asks["b"] {
		t.Fatal("parallel prompts lost event routing")
	}
}
