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

type cancelGateProvider struct{ ready chan context.Context }

func (p *cancelGateProvider) Name() string { return "isolated-stop-fixture" }
func (p *cancelGateProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	chunks := make(chan provider.Chunk)
	go func() {
		defer close(chunks)
		select {
		case chunks <- provider.Chunk{Type: provider.ChunkText, Text: "partial fixture answer"}:
		case <-ctx.Done():
			return
		}
		p.ready <- ctx
		<-ctx.Done()
	}()
	return chunks, nil
}

func TestDesktopSessionCancelActualAgentServeClient(t *testing.T) {
	f, client := desktopViewFixture(t)
	f.server.ctl().Close()
	session, err := agent.LoadSession(f.active)
	if err != nil {
		t.Fatal(err)
	}
	p := &cancelGateProvider{ready: make(chan context.Context, 4)}
	exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	done := make(chan event.Event, 4)
	ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: f.dir, SessionPath: f.active, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	})})
	f.server.mu.Lock()
	f.server.ctrl = ctrl
	f.server.mu.Unlock()
	t.Cleanup(func() {
		ctrl.Cancel()
		deadline := time.Now().Add(5 * time.Second)
		for ctrl.Running() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if ctrl.Running() {
			t.Error("owned turn did not settle")
		}
		ctrl.Close()
	})
	waitReady := func() context.Context {
		t.Helper()
		select {
		case ctx := <-p.ready:
			return ctx
		case <-time.After(10 * time.Second):
			t.Fatal("provider did not start")
			return nil
		}
	}
	waitDone := func() event.Event {
		t.Helper()
		select {
		case e := <-done:
			return e
		case <-time.After(10 * time.Second):
			t.Fatal("provider did not stop")
			return event.Event{}
		}
	}
	path := agent.CanonicalSessionPath(f.active)
	ctrl.Submit("actual first question")
	firstCtx := waitReady()
	view, err := client.SessionView(context.Background(), path)
	if err != nil || view.RuntimeState == nil || !view.RuntimeState.Running || view.RuntimeState.TurnID == "" {
		t.Fatal("active runtime unavailable", err)
	}
	scope := controller.SessionCancelScope{SessionPath: path, RuntimeEpoch: view.RuntimeState.RuntimeEpoch, TurnID: view.RuntimeState.TurnID}
	unauthenticated, _ := json.Marshal(desktopSessionCancelRequest{1, path, scope.RuntimeEpoch, scope.TurnID})
	// desktopViewFixture's original f.srv predates its token gate. Inspect a
	// newly constructed authenticated Handler, as used by the shared client.
	authenticated := httptest.NewServer(f.server.Handler())
	t.Cleanup(authenticated.Close)
	resp, err := http.Post(authenticated.URL+"/desktop/session-cancel", "application/json", strings.NewReader(string(unauthenticated)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 || firstCtx.Err() != nil {
		t.Fatal("unauthenticated stop reached actual provider")
	}
	wrong := scope
	wrong.RuntimeEpoch = "stale-instance"
	if _, err := client.CancelSessionTurn(context.Background(), wrong); !errors.Is(err, controller.ErrTurnCancelChanged) || firstCtx.Err() != nil {
		t.Fatal("wrong instance stopped actual turn", err)
	}
	receipt, err := client.CancelSessionTurn(context.Background(), scope)
	if err != nil || receipt.Scope() != scope || !receipt.Cancelled {
		t.Fatal("actual cancellation failed", receipt, err)
	}
	terminal := waitDone()
	if firstCtx.Err() != context.Canceled || terminal.TurnID != scope.TurnID || terminal.Status != event.TurnInterrupted {
		t.Fatal("wrong actual terminal", terminal.Status)
	}
	ctrl.Submit("actual replacement question")
	secondCtx := waitReady()
	if _, err := client.CancelSessionTurn(context.Background(), scope); !errors.Is(err, controller.ErrTurnCancelChanged) || secondCtx.Err() != nil {
		t.Fatal("stale HTTP stop crossed replacement", err)
	}
	view, err = client.SessionView(context.Background(), path)
	if err != nil || view.RuntimeState == nil || view.RuntimeState.TurnID == scope.TurnID {
		t.Fatal("replacement identity unavailable", err)
	}
	current := controller.SessionCancelScope{SessionPath: path, RuntimeEpoch: view.RuntimeState.RuntimeEpoch, TurnID: view.RuntimeState.TurnID}
	// A saved catalogue member is not an already-owned runtime.
	saved := filepath.Join(f.dir, "saved.jsonl")
	saveServeTestSession(t, saved)
	savedScope := current
	savedScope.SessionPath = agent.CanonicalSessionPath(saved)
	if _, err := client.CancelSessionTurn(context.Background(), savedScope); !errors.Is(err, controller.ErrTurnCancelChanged) || secondCtx.Err() != nil {
		t.Fatal("saved session adopted or foreground cancelled", err)
	}
	if _, err := client.CancelSessionTurn(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	waitDone()
	if f.server.ctl() != ctrl || ctrl.SessionPath() != f.active || f.server.sessionMirrored(saved) {
		t.Fatal("stop switched, adopted or mirrored a session")
	}
}

func TestDesktopSessionCancelRejectsMalformedAndUnowned(t *testing.T) {
	f := newOwnershipFixture(t)
	path := agent.CanonicalSessionPath(f.active)
	valid := desktopSessionCancelRequest{1, path, "fixture-epoch", "fixture-turn"}
	data, _ := json.Marshal(valid)
	for _, body := range []string{`{}`, string(data) + ` {}`, strings.TrimSuffix(string(data), "}") + `,"prompt":"PRIVATE"}`, strings.Repeat("x", 41<<10)} {
		resp, err := http.Post(f.srv.URL+"/desktop/session-cancel", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("malformed body accepted: %d", resp.StatusCode)
		}
	}
	f.server.markMirrored(mirroredSession{path: f.active, mirrorID: "owned", phase: mirrorPhaseExternal})
	if status, _ := f.post(t, "/desktop/session-cancel", valid); status != 409 || !f.server.sessionMirrored(f.active) {
		t.Fatal("stop reclaimed external session")
	}
}

func TestDesktopSessionCancelActualDetachedAndForeignWriter(t *testing.T) {
	f, client := desktopViewFixture(t)
	path := filepath.Join(f.dir, "detached-stop.jsonl")
	saveServeTestSession(t, path)
	session, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	p := &cancelGateProvider{ready: make(chan context.Context, 1)}
	exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	done := make(chan event.Event, 1)
	ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: f.dir, SessionPath: path, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	})})
	t.Cleanup(func() {
		ctrl.Cancel()
		deadline := time.Now().Add(5 * time.Second)
		for ctrl.Running() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if ctrl.Running() {
			t.Error("detached fixture did not settle")
		}
		ctrl.Close()
	})
	canonical := agent.CanonicalSessionPath(path)
	d := &detachedSession{path: canonical, ctrl: ctrl}
	f.server.detachedMu.Lock()
	f.server.detached[canonical] = d
	f.server.detachedMu.Unlock()
	ctrl.Submit("actual detached question")
	var ctx context.Context
	select {
	case ctx = <-p.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("detached provider did not start")
	}
	view, err := client.SessionView(context.Background(), canonical)
	if err != nil || view.Current || view.Ownership != "serve" || view.RuntimeState == nil || view.RuntimeState.TurnID == "" {
		t.Fatal("detached scope unavailable", err)
	}
	scope := controller.SessionCancelScope{SessionPath: canonical, RuntimeEpoch: view.RuntimeState.RuntimeEpoch, TurnID: view.RuntimeState.TurnID}
	f.server.detachedMu.Lock()
	d.retiring = true
	f.server.detachedMu.Unlock()
	if _, err := client.CancelSessionTurn(context.Background(), scope); !errors.Is(err, controller.ErrTurnCancelChanged) || ctx.Err() != nil {
		t.Fatal("retiring runtime accepted stop", err)
	}
	f.server.detachedMu.Lock()
	d.retiring = false
	f.server.detachedMu.Unlock()
	var foreign atomic.Bool
	foreign.Store(true)
	withForeignWriterLease(t, path, &foreign)
	if _, err := client.CancelSessionTurn(context.Background(), scope); !errors.Is(err, controller.ErrTurnCancelChanged) || ctx.Err() != nil {
		t.Fatal("foreign writer accepted stop", err)
	}
	foreign.Store(false)
	if _, err := client.CancelSessionTurn(context.Background(), scope); err != nil {
		t.Fatal("owned detached stop failed", err)
	}
	select {
	case e := <-done:
		if e.TurnID != scope.TurnID || e.Status != event.TurnInterrupted {
			t.Fatal("wrong detached terminal")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("detached provider did not stop")
	}
	if f.server.ctl().SessionPath() != f.active || f.server.sessionMirrored(path) {
		t.Fatal("detached stop switched foreground or changed ownership")
	}
}
