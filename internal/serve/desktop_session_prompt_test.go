package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/remote/controller"
	"reasonix/internal/tool"
)

func TestDesktopSessionPromptActualControllerServeClient(t *testing.T) {
	for _, detached := range []bool{false, true} {
		name := "foreground"
		if detached {
			name = "detached"
		}
		t.Run(name, func(t *testing.T) {
			f, client := desktopViewFixture(t)
			path := f.active
			if detached {
				path = filepath.Join(f.dir, "detached-prompt.jsonl")
				saveServeTestSession(t, path)
			} else {
				f.server.ctl().Close()
			}
			session, err := agent.LoadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			p := &submitGateProvider{calls: make(chan submitProviderCall, 2)}
			exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
			prompts := make(chan event.Event, 2)
			decisions := make(chan event.Event, 2)
			ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: f.dir, SessionPath: path, Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.AskRequest {
					prompts <- e
				}
				if e.Kind == event.PromptAnswered {
					decisions <- e
				}
			})})
			canonical := agent.CanonicalSessionPath(path)
			var d *detachedSession
			if detached {
				d = &detachedSession{path: canonical, ctrl: ctrl}
				f.server.detachedMu.Lock()
				f.server.detached[canonical] = d
				f.server.detachedMu.Unlock()
			} else {
				f.server.mu.Lock()
				f.server.ctrl = ctrl
				f.server.mu.Unlock()
			}
			foreground := f.server.ctl()
			t.Cleanup(func() {
				ctrl.Cancel()
				deadline := time.Now().Add(5 * time.Second)
				for ctrl.Running() && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if ctrl.Running() {
					t.Error("prompt fixture did not settle")
				}
				ctrl.Close()
			})
			view, err := client.SessionView(context.Background(), canonical)
			if err != nil || view.RuntimeState == nil {
				t.Fatal(err)
			}
			if _, err := client.SubmitSessionTurn(context.Background(), controller.SessionSubmitScope{SessionPath: canonical, RuntimeEpoch: view.RuntimeState.RuntimeEpoch, Revision: view.RuntimeState.Revision}, "question fixture"); err != nil {
				t.Fatal(err)
			}
			var call submitProviderCall
			select {
			case call = <-p.calls:
			case <-time.After(5 * time.Second):
				t.Fatal("provider did not start")
			}
			answers := make(chan []event.AskAnswer, 1)
			go func() {
				result, _ := ctrl.Ask(call.ctx, []event.AskQuestion{{ID: "q1", Prompt: "Choose?", Options: []event.AskOption{{Label: "A"}}}})
				answers <- result
			}()
			select {
			case <-prompts:
			case <-time.After(5 * time.Second):
				t.Fatal("actual Ask not emitted")
			}
			identities := ctrl.PendingPromptIdentities()
			if len(identities) != 1 {
				t.Fatal(identities)
			}
			id := identities[0]
			input := controller.SessionPromptRequest{SessionPromptScope: controller.SessionPromptScope{SessionPath: canonical, RuntimeEpoch: ctrl.RuntimeStateSnapshot().RuntimeEpoch, TurnID: id.TurnID, PromptID: id.PromptID, PromptRuntimeEpoch: id.RuntimeEpoch, Kind: string(id.Kind)}, Answer: json.RawMessage(`{"questions":[{"questionId":"q1","selected":["A"]}]}`)}
			wire, _ := json.Marshal(desktopSessionPromptRequest{1, input})
			authenticated := httptest.NewServer(f.server.Handler())
			t.Cleanup(authenticated.Close)
			response, err := http.Post(authenticated.URL+"/desktop/session-prompt", "application/json", strings.NewReader(string(wire)))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 401 {
				t.Fatal("decision bypassed authentication")
			}
			wrong := input
			wrong.RuntimeEpoch += "-other"
			if _, err := client.ResolveSessionPrompt(context.Background(), wrong); !errors.Is(err, controller.ErrPromptChanged) {
				t.Fatal("wrong instance decided prompt")
			}
			wrong = input
			wrong.PromptRuntimeEpoch = "other-routing"
			if _, err := client.ResolveSessionPrompt(context.Background(), wrong); !errors.Is(err, controller.ErrPromptChanged) {
				t.Fatal("wrong routing decided prompt")
			}
			saved := filepath.Join(f.dir, "saved-prompt.jsonl")
			saveServeTestSession(t, saved)
			wrong = input
			wrong.SessionPath = agent.CanonicalSessionPath(saved)
			if _, err := client.ResolveSessionPrompt(context.Background(), wrong); !errors.Is(err, controller.ErrPromptChanged) {
				t.Fatal("saved session adopted", err)
			}
			if detached {
				d.admissionMu.Lock()
				_, err = client.ResolveSessionPrompt(context.Background(), input)
				d.admissionMu.Unlock()
				if !errors.Is(err, controller.ErrPromptChanged) {
					t.Fatal("decision queued behind retirement", err)
				}
				f.server.detachedMu.Lock()
				d.retiring = true
				f.server.detachedMu.Unlock()
				_, err = client.ResolveSessionPrompt(context.Background(), input)
				f.server.detachedMu.Lock()
				d.retiring = false
				f.server.detachedMu.Unlock()
				if err == nil {
					t.Fatal("retiring owner decided prompt")
				}
			}
			var foreign atomic.Bool
			foreign.Store(true)
			withForeignWriterLease(t, path, &foreign)
			if _, err := client.ResolveSessionPrompt(context.Background(), input); !errors.Is(err, controller.ErrPromptChanged) {
				t.Fatal("foreign writer accepted decision", err)
			}
			foreign.Store(false)
			select {
			case <-answers:
				t.Fatal("rejected decision woke prompt")
			default:
			}
			receipt, err := client.ResolveSessionPrompt(context.Background(), input)
			if err != nil || !receipt.Resolved || receipt.SessionPromptScope != input.SessionPromptScope {
				t.Fatal(receipt, err)
			}
			select {
			case got := <-answers:
				if len(got) != 1 || got[0].QuestionID != "q1" || len(got[0].Selected) != 1 || got[0].Selected[0] != "A" {
					t.Fatal("actual answer changed")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("actual Ask remained blocked")
			}
			select {
			case got := <-decisions:
				if got.TurnID != id.TurnID || got.ItemID != id.PromptID {
					t.Fatal("wrong durable transition")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("decision not persisted")
			}
			if _, err := client.ResolveSessionPrompt(context.Background(), input); err == nil {
				t.Fatal("duplicate decision accepted")
			}
			if f.server.ctl() != foreground || ctrl.SessionPath() != path || call.ctx.Err() != nil {
				t.Fatal("decision changed owner/turn")
			}
		})
	}
}

func TestDesktopSessionPromptStrictBodyAndBindingGate(t *testing.T) {
	f := newOwnershipFixture(t)
	input := desktopSessionPromptRequest{1, controller.SessionPromptRequest{SessionPromptScope: controller.SessionPromptScope{SessionPath: agent.CanonicalSessionPath(f.active), RuntimeEpoch: "fixture", TurnID: "turn", PromptID: "prompt", Kind: "approval"}, Answer: json.RawMessage(`{"allow":false}`)}}
	data, _ := json.Marshal(input)
	for _, body := range []string{`{}`, string(data) + ` {}`, strings.TrimSuffix(string(data), "}") + `,"private":"x"}`, strings.Repeat("x", (256<<10)+1), strings.Replace(string(data), `"allow":false`, `"allow":false,"action":"accept"`, 1)} {
		response, err := http.Post(f.srv.URL+"/desktop/session-prompt", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatalf("bad body status %d", response.StatusCode)
		}
	}
	f.server.bindMu.Lock()
	response, err := http.Post(f.srv.URL+"/desktop/session-prompt", "application/json", strings.NewReader(string(data)))
	f.server.bindMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("decision queued behind binding")
	}
}
