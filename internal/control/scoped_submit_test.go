package control

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"reasonix/internal/event"
)

type scopedSubmitCheckedContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *scopedSubmitCheckedContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func scopedSubmitIdentity(c *Controller) TurnSubmitScope {
	state := c.RuntimeStateSnapshot()
	return TurnSubmitScope{SessionPath: c.SessionPath(), RuntimeEpoch: state.RuntimeEpoch, Revision: state.Revision}
}

func TestSubmitScopedReservesExactlyOneTurnAndRejectsOldIdleRevision(t *testing.T) {
	done := make(chan event.Event, 2)
	states := make(chan event.RuntimeStateSnapshot, 64)
	c := scopedCancelController(t, &runtimeStateTestSink{states: states, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	})})
	scope := scopedSubmitIdentity(c)
	started := make(chan context.Context, 1)
	request, revoke := context.WithCancel(context.Background())
	if err := c.admitScopedTurn(request, scope, func(ctx context.Context) error { started <- ctx; <-ctx.Done(); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	turn := <-started
	revoke()
	if turn.Err() != nil {
		t.Fatal("transport disconnect cancelled accepted turn")
	}
	if err := c.admitScopedTurn(context.Background(), scope, func(context.Context) error { t.Error("duplicate body ran"); return nil }); !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := c.CancelScoped(scopedCancelIdentity(c)); err != nil {
		t.Fatal(err)
	}
	waitTurnDoneEvent(t, done)
	waitIdleAdmission(t, c)
	runtimeStateAwait(t, states, func(s event.RuntimeStateSnapshot) bool { return s.Phase == "idle" && s.Revision > scope.Revision })
	if err := c.admitScopedTurn(context.Background(), scope, func(context.Context) error { t.Error("stale idle body ran"); return nil }); !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("old idle revision: %v", err)
	}
	if err := c.admitScopedTurn(context.Background(), scopedSubmitIdentity(c), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	waitTurnDoneEvent(t, done)
}

func TestSubmitScopedRejectsChangedOwnerAndRevocationWhileWaiting(t *testing.T) {
	c := scopedCancelController(t, nil)
	scope := scopedSubmitIdentity(c)
	path, epoch, revision := scope, scope, scope
	path.SessionPath += ".other"
	epoch.RuntimeEpoch += "-other"
	revision.Revision++
	for _, wrong := range []TurnSubmitScope{{}, path, epoch, revision} {
		if err := c.admitScopedTurn(context.Background(), wrong, func(context.Context) error { t.Error("wrong owner body ran"); return nil }); !errors.Is(err, ErrTurnSubmitScope) {
			t.Fatalf("wrong scope: %v", err)
		}
	}
	request, revoke := context.WithCancel(context.Background())
	checked := &scopedSubmitCheckedContext{Context: request, checked: make(chan struct{})}
	returned := make(chan error, 1)
	c.runtimeState.mu.Lock()
	go func() {
		returned <- c.admitScopedTurn(checked, scope, func(context.Context) error { t.Error("revoked body ran"); return nil })
	}()
	// The first context check returned nil while the runtime lock was held.
	// Revocation must therefore be checked again inside admission, not only
	// rejected by the initial fast path.
	<-checked.checked
	revoke()
	c.runtimeState.mu.Unlock()
	if err := <-returned; !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("revoked admission: %v", err)
	}
	if c.RuntimeStateSnapshot().Running {
		t.Fatal("rejected operation reserved a turn")
	}
	if err := c.SubmitScopedContext(context.Background(), scope, " \n "); !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("empty input: %v", err)
	}
}

func TestSubmitScopedDoesNotParkDuringFinishingOrAcceptInvalidLedger(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	c := scopedCancelController(t, holdFinishingWindow(release, entered, nil))
	c.runGuarded(func(context.Context) error { return nil })
	<-entered
	err := c.admitScopedTurn(context.Background(), scopedSubmitIdentity(c), func(context.Context) error { t.Error("finishing body ran"); return nil })
	c.mu.Lock()
	parked := len(c.parkedTurns)
	c.mu.Unlock()
	close(release)
	waitIdleAdmission(t, c)
	if !errors.Is(err, ErrTurnSubmitScope) || parked != 0 {
		t.Fatalf("finishing admission = %v, parked = %d", err, parked)
	}
	scope := scopedSubmitIdentity(c)
	c.turnEvents.mu.Lock()
	c.turnEvents.err = errors.New("fixture ledger unavailable")
	c.turnEvents.mu.Unlock()
	err = c.admitScopedTurn(context.Background(), scope, func(context.Context) error { t.Error("failed ledger body ran"); return nil })
	c.turnEvents.mu.Lock()
	c.turnEvents.err = nil
	c.turnEvents.mu.Unlock()
	if !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("failed ledger admission: %v", err)
	}
	c.mu.Lock()
	c.rotating = true
	c.mu.Unlock()
	err = c.admitScopedTurn(context.Background(), scope, func(context.Context) error { t.Error("rotation body ran"); return nil })
	c.mu.Lock()
	c.rotating = false
	c.mu.Unlock()
	if !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("rotation admission: %v", err)
	}
	// A finishing/running observation must not become writable merely because
	// admission has opened before the next runtime-state commit arrives.
	c.runtimeState.mu.Lock()
	prior := c.runtimeState.snapshot
	c.runtimeState.snapshot.Phase = "finishing"
	c.runtimeState.snapshot.Running = true
	c.runtimeState.mu.Unlock()
	err = c.admitScopedTurn(context.Background(), scope, func(context.Context) error { t.Error("busy observation body ran"); return nil })
	c.runtimeState.mu.Lock()
	c.runtimeState.snapshot = prior
	c.runtimeState.mu.Unlock()
	if !errors.Is(err, ErrTurnSubmitScope) {
		t.Fatalf("busy observation admission: %v", err)
	}
}

type scopedSubmitTextRunner struct{ input chan string }

func (r *scopedSubmitTextRunner) Run(_ context.Context, input string) error {
	r.input <- input
	return nil
}

func TestSubmitScopedTreatsCommandsAsUserText(t *testing.T) {
	for _, text := range []string{"/new", "/clear", "!echo must-not-execute"} {
		t.Run(text, func(t *testing.T) {
			dir := t.TempDir()
			runner := &scopedSubmitTextRunner{input: make(chan string, 1)}
			done := make(chan event.Event, 1)
			c := New(Options{Runner: runner, SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.TurnDone {
					done <- e
				}
			})})
			t.Cleanup(func() { waitIdleAdmission(t, c); c.Close() })
			scope := scopedSubmitIdentity(c)
			if err := c.SubmitScopedContext(context.Background(), scope, text); err != nil {
				t.Fatal(err)
			}
			waitTurnDoneEvent(t, done)
			if got := <-runner.input; got != text {
				t.Fatalf("runner input = %q", got)
			}
			if c.SessionPath() != scope.SessionPath {
				t.Fatal("user text switched session")
			}
		})
	}
}
