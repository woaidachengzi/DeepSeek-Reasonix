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
	"reasonix/internal/provider"
	"reasonix/internal/remote/controller"
	"reasonix/internal/tool"
)

type submitProviderCall struct {
	ctx     context.Context
	request provider.Request
}
type submitGateProvider struct{ calls chan submitProviderCall }

func (p *submitGateProvider) Name() string { return "isolated-submit-fixture" }
func (p *submitGateProvider) Stream(ctx context.Context, request provider.Request) (<-chan provider.Chunk, error) {
	chunks := make(chan provider.Chunk)
	go func() {
		defer close(chunks)
		select {
		case chunks <- provider.Chunk{Type: provider.ChunkText, Text: "partial fixture answer"}:
		case <-ctx.Done():
			return
		}
		p.calls <- submitProviderCall{ctx, request}
		<-ctx.Done()
	}()
	return chunks, nil
}

func TestDesktopSessionSubmitActualAgentServeClient(t *testing.T) {
	for _, detached := range []bool{false, true} {
		name := "foreground"
		if detached {
			name = "detached"
		}
		t.Run(name, func(t *testing.T) {
			f, client := desktopViewFixture(t)
			path := f.active
			if detached {
				path = filepath.Join(f.dir, "detached-submit.jsonl")
				saveServeTestSession(t, path)
			} else {
				f.server.ctl().Close()
			}
			session, err := agent.LoadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			p := &submitGateProvider{calls: make(chan submitProviderCall, 4)}
			exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
			done := make(chan event.Event, 4)
			ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: f.dir, SessionPath: path, Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.TurnDone {
					done <- e
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
					t.Error("owned submit fixture did not settle")
				}
				ctrl.Close()
			})
			view, err := client.SessionView(context.Background(), canonical)
			if err != nil || view.Ownership != "serve" || view.RuntimeState == nil || view.RuntimeState.Running {
				t.Fatal("owned idle view unavailable", err)
			}
			scope := controller.SessionSubmitScope{SessionPath: canonical, RuntimeEpoch: view.RuntimeState.RuntimeEpoch, Revision: view.RuntimeState.Revision}
			text := "/new"
			authenticated := httptest.NewServer(f.server.Handler())
			t.Cleanup(authenticated.Close)
			payload, _ := json.Marshal(desktopSessionSubmitRequest{1, canonical, scope.RuntimeEpoch, scope.Revision, text})
			response, err := http.Post(authenticated.URL+"/desktop/session-submit", "application/json", strings.NewReader(string(payload)))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 401 {
				t.Fatal("message bypassed token gate")
			}
			wrong := scope
			wrong.RuntimeEpoch += "-other"
			if _, err := client.SubmitSessionTurn(context.Background(), wrong, text); !errors.Is(err, controller.ErrTurnSubmitChanged) {
				t.Fatal("wrong owner accepted message", err)
			}
			saved := filepath.Join(f.dir, "saved-submit.jsonl")
			saveServeTestSession(t, saved)
			wrong = scope
			wrong.SessionPath = agent.CanonicalSessionPath(saved)
			if _, err := client.SubmitSessionTurn(context.Background(), wrong, text); !errors.Is(err, controller.ErrTurnSubmitChanged) {
				t.Fatal("saved session was adopted", err)
			}
			if detached {
				d.admissionMu.Lock()
				_, err = client.SubmitSessionTurn(context.Background(), scope, text)
				d.admissionMu.Unlock()
				if !errors.Is(err, controller.ErrTurnSubmitChanged) {
					t.Fatal("message queued behind idle retirement/rebuild gate", err)
				}
				f.server.detachedMu.Lock()
				d.retiring = true
				f.server.detachedMu.Unlock()
				_, err = client.SubmitSessionTurn(context.Background(), scope, text)
				f.server.detachedMu.Lock()
				d.retiring = false
				f.server.detachedMu.Unlock()
				if err == nil {
					t.Fatal("retiring owner accepted message")
				}
			}
			var foreign atomic.Bool
			foreign.Store(true)
			withForeignWriterLease(t, path, &foreign)
			if _, err := client.SubmitSessionTurn(context.Background(), scope, text); !errors.Is(err, controller.ErrTurnSubmitChanged) {
				t.Fatal("foreign writer accepted message", err)
			}
			foreign.Store(false)
			select {
			case <-p.calls:
				t.Fatal("rejected message reached provider")
			default:
			}
			receipt, err := client.SubmitSessionTurn(context.Background(), scope, text)
			if err != nil || receipt.Scope() != scope || !receipt.Accepted {
				t.Fatal("actual message not admitted", receipt, err)
			}
			var call submitProviderCall
			select {
			case call = <-p.calls:
			case <-time.After(10 * time.Second):
				t.Fatal("actual provider did not start")
			}
			found := false
			for _, message := range call.request.Messages {
				if message.Role == provider.RoleUser && message.Content == text {
					found = true
				}
			}
			if !found {
				t.Fatal("slash text did not reach actual Agent as a user question")
			}
			if _, err := client.SubmitSessionTurn(context.Background(), scope, "duplicate"); !errors.Is(err, controller.ErrTurnSubmitChanged) || call.ctx.Err() != nil {
				t.Fatal("duplicate changed active turn", err)
			}
			if f.server.ctl() != foreground || ctrl.SessionPath() != path || f.server.sessionMirrored(saved) {
				t.Fatal("submit switched or adopted a session")
			}
			active, err := client.SessionView(context.Background(), canonical)
			if err != nil || active.RuntimeState == nil || active.RuntimeState.TurnID == "" {
				t.Fatal("active turn identity unavailable", err)
			}
			stop := controller.SessionCancelScope{SessionPath: canonical, RuntimeEpoch: active.RuntimeState.RuntimeEpoch, TurnID: active.RuntimeState.TurnID}
			if _, err := client.CancelSessionTurn(context.Background(), stop); err != nil {
				t.Fatal(err)
			}
			select {
			case terminal := <-done:
				if terminal.TurnID != stop.TurnID || terminal.Status != event.TurnInterrupted {
					t.Fatal("wrong actual terminal")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("actual turn did not stop")
			}
			deadline := time.Now().Add(5 * time.Second)
			for ctrl.RuntimeStateSnapshot().Phase != "idle" && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if ctrl.RuntimeStateSnapshot().Phase != "idle" {
				t.Fatal("idle state was not published")
			}
			if _, err := client.SubmitSessionTurn(context.Background(), scope, "old idle page"); !errors.Is(err, controller.ErrTurnSubmitChanged) {
				t.Fatal("old idle revision accepted after completion", err)
			}
			count := 0
			for _, message := range ctrl.History() {
				if message.Role == provider.RoleUser && message.Content == text {
					count++
				}
			}
			if count != 1 {
				t.Fatal("actual admitted user question count", count)
			}
		})
	}
}

func TestDesktopSessionSubmitStrictBodyAndNoReclaim(t *testing.T) {
	f := newOwnershipFixture(t)
	path := agent.CanonicalSessionPath(f.active)
	valid := desktopSessionSubmitRequest{1, path, "fixture-epoch", 1, "question\nline2"}
	data, _ := json.Marshal(valid)
	for _, body := range []string{`{}`, string(data) + ` {}`, strings.TrimSuffix(string(data), "}") + `,"model":"PRIVATE"}`, strings.Repeat("x", (1<<20)+1)} {
		response, err := http.Post(f.srv.URL+"/desktop/session-submit", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("malformed message accepted", response.StatusCode)
		}
	}
	for _, input := range []desktopSessionSubmitRequest{{1, path, "epoch", 0, "question"}, {1, path, "epoch", 1, " \n "}, {1, path, "epoch", 1, "bad\x00text"}, {1, path, "epoch", 1, strings.Repeat("x", (512<<10)+1)}, {1, path, "epoch", 9_007_199_254_740_992, "question"}} {
		if status, _ := f.post(t, "/desktop/session-submit", input); status != 400 {
			t.Fatal("invalid message accepted", status)
		}
	}
	if status, _ := f.post(t, "/desktop/session-submit?session=other", valid); status != 400 {
		t.Fatal("query changed selection", status)
	}
	f.server.bindMu.Lock()
	status, _ := f.post(t, "/desktop/session-submit", valid)
	f.server.bindMu.Unlock()
	if status != 409 {
		t.Fatal("message queued behind a binding replacement", status)
	}
	f.server.markMirrored(mirroredSession{path: f.active, mirrorID: "owned", phase: mirrorPhaseExternal})
	if status, _ := f.post(t, "/desktop/session-submit", valid); status != 409 || !f.server.sessionMirrored(f.active) {
		t.Fatal("message reclaimed mirrored session")
	}
}
