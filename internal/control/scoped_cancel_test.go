package control

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
)

func scopedCancelController(t *testing.T, sink event.Sink) *Controller {
	t.Helper()
	dir := t.TempDir()
	c := New(Options{SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: sink})
	t.Cleanup(func() {
		defer c.Close()
		c.mu.Lock()
		cancel := c.cancel
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		// Close seals admission but intentionally does not join a live turn.
		// Join this fixture before TempDir removes its ledger artifacts.
		waitIdleAdmission(t, c)
	})
	// Ordinary Serve controllers do not have desktop routing metadata. Their
	// committed Controller epoch must still fence a valid cancellation.
	return c
}

func scopedCancelIdentity(c *Controller) TurnCancelScope {
	return TurnCancelScope{SessionPath: c.SessionPath(), RuntimeEpoch: c.RuntimeStateSnapshot().RuntimeEpoch, TurnID: c.turnEventLedger().ActiveTurnID()}
}

func TestCancelScopedRejectsStaleIdentityWithoutTouchingLiveTurn(t *testing.T) {
	done := make(chan event.Event, 4)
	c := scopedCancelController(t, event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	}))
	c.runGuarded(func(context.Context) error { return nil })
	first := waitTurnDoneEvent(t, done)
	c.SetTurnEventRoutingMetadata("cancel-epoch", "replacement-submission")
	started := make(chan context.Context, 1)
	c.runGuarded(func(ctx context.Context) error { started <- ctx; <-ctx.Done(); return ctx.Err() })
	ctx := <-started
	scope := scopedCancelIdentity(c)
	if scope.TurnID == "" || scope.TurnID == first.TurnID {
		t.Fatal("replacement turn was not admitted")
	}
	wrongPath, wrongEpoch, wrongTurn := scope, scope, scope
	wrongPath.SessionPath += ".other"
	wrongEpoch.RuntimeEpoch += "-other"
	wrongTurn.TurnID = first.TurnID
	revoked, revoke := context.WithCancel(context.Background())
	revoke()
	if err := c.CancelScopedContext(revoked, scope); !errors.Is(err, ErrTurnCancelScope) || ctx.Err() != nil {
		t.Fatal("revoked operation cancelled the live turn")
	}
	for _, stale := range []TurnCancelScope{{}, wrongPath, wrongEpoch, wrongTurn} {
		if err := c.CancelScoped(stale); !errors.Is(err, ErrTurnCancelScope) {
			t.Fatalf("stale cancellation returned %v", err)
		}
		if ctx.Err() != nil || c.RuntimeStatus().CancelRequested {
			t.Fatal("stale cancellation changed the live turn")
		}
	}
	c.turnEvents.mu.Lock()
	c.turnEvents.err = errors.New("fixture ledger unavailable")
	c.turnEvents.mu.Unlock()
	err := c.CancelScoped(scope)
	c.turnEvents.mu.Lock()
	c.turnEvents.err = nil
	c.turnEvents.mu.Unlock()
	if !errors.Is(err, ErrTurnCancelScope) || ctx.Err() != nil {
		t.Fatal("failed ledger allowed cancellation")
	}
	if err := c.CancelScoped(scope); err != nil {
		t.Fatal(err)
	}
	if terminal := waitTurnDoneEvent(t, done); terminal.TurnID != scope.TurnID || terminal.Status != event.TurnInterrupted {
		t.Fatalf("wrong terminal: %+v", terminal)
	}
	if err := c.CancelScoped(scope); !errors.Is(err, ErrTurnCancelScope) {
		t.Fatalf("terminal scope accepted: %v", err)
	}
}

func TestCancelScopedSignalsContextBeforeStatusBarrier(t *testing.T) {
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	c := scopedCancelController(t, event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStatusChanged && e.Status == event.TurnCancelling {
			close(entered)
			<-release
		}
	}))
	started, holdTurn := make(chan context.Context, 1), make(chan struct{})
	c.runGuarded(func(ctx context.Context) error { started <- ctx; <-ctx.Done(); <-holdTurn; return ctx.Err() })
	ctx := <-started
	defer close(holdTurn)
	defer func() {
		close(release)
		select {
		case err := <-returned:
			if err != nil {
				t.Errorf("scoped cancellation: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("cancellation did not finish after releasing status barrier")
		}
	}()
	scope := scopedCancelIdentity(c)
	go func() { returned <- c.CancelScoped(scope) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling status did not reach barrier")
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("context was not cancelled before status barrier")
	}
	select {
	case err := <-returned:
		t.Fatalf("cancellation returned before barrier: %v", err)
	default:
	}
}

func TestPendingPromptOwnerScopedCancellationPreservesReplacement(t *testing.T) {
	var owner PendingPromptOwner
	oldCalls, otherCalls := 0, 0
	if err := owner.RegisterPrompt(PendingPrompt{Identity: PromptIdentity{PromptID: "old", TurnID: "old-turn"}, Cancel: func() error { oldCalls++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := owner.RegisterPrompt(PendingPrompt{Identity: PromptIdentity{PromptID: "other", TurnID: "other-turn"}, Cancel: func() error { otherCalls++; return nil }}); err != nil {
		t.Fatal(err)
	}
	cancels := owner.takeTurnCancels("old-turn")
	if err := owner.Register(PromptIdentity{PromptID: "replacement", TurnID: "new-turn"}); err != nil {
		t.Fatal(err)
	}
	for _, cancel := range cancels {
		_ = cancel()
	}
	if oldCalls != 1 || otherCalls != 0 {
		t.Fatalf("cancel calls old=%d other=%d", oldCalls, otherCalls)
	}
	if _, ok := owner.Identity("old"); ok {
		t.Fatal("old prompt remained pending")
	}
	for _, id := range []string{"other", "replacement"} {
		if _, ok := owner.Identity(id); !ok {
			t.Fatalf("unrelated prompt %s was removed", id)
		}
	}
	if len(owner.takeTurnCancels("")) != 0 || len(owner.Identities()) != 2 {
		t.Fatal("empty turn wildcard cancelled unrelated prompts")
	}
}

func TestCancelScopedCleanupCannotCancelReplacementContextOrPrompts(t *testing.T) {
	done := make(chan event.Event, 4)
	c := scopedCancelController(t, event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	}))
	started := make(chan context.Context, 1)
	c.runGuarded(func(ctx context.Context) error { started <- ctx; <-ctx.Done(); return ctx.Err() })
	<-started
	scope := scopedCancelIdentity(c)
	cleanupEntered, releaseCleanup := make(chan struct{}), make(chan struct{})
	returned := make(chan error, 1)
	released, joined := false, false
	if err := c.promptOwner.RegisterPrompt(PendingPrompt{
		Identity: PromptIdentity{PromptID: "old-prompt", TurnID: scope.TurnID},
		Cancel:   func() error { close(cleanupEntered); <-releaseCleanup; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !released {
			close(releaseCleanup)
		}
		if joined {
			return
		}
		select {
		case err := <-returned:
			if err != nil {
				t.Errorf("old cancellation: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("old cancellation did not finish")
		}
	}()
	go func() { returned <- c.CancelScoped(scope) }()
	select {
	case <-cleanupEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("old cleanup did not run")
	}
	waitTurnDoneEvent(t, done)
	// Reserve routing directly: this test deliberately admits a replacement
	// while the old cancellation owns promptResolveMu and waits in cleanup.
	c.turnEventLedger().SetRoutingMetadata("replacement-epoch", "replacement")
	if result := c.runGuarded(func(ctx context.Context) error {
		started <- ctx
		<-ctx.Done()
		return ctx.Err()
	}); result != turnStarted && result != turnParked {
		t.Fatalf("replacement admission = %v", result)
	}
	var ctx context.Context
	select {
	case ctx = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not start during old cleanup")
	}
	newID := c.turnEventLedger().ActiveTurnID()
	if err := c.promptOwner.Register(PromptIdentity{PromptID: "new-prompt", TurnID: newID}); err != nil {
		t.Fatal(err)
	}
	// Complete old cleanup while the replacement is alive. The old cancelling
	// status must also be discarded by the ledger, not stick to the new turn.
	close(releaseCleanup)
	released = true
	select {
	case err := <-returned:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old cancellation did not complete")
	}
	if ctx.Err() != nil || c.RuntimeStatus().CancelRequested {
		t.Fatal("old cleanup cancelled replacement context or status")
	}
	if _, ok := c.promptOwner.Identity("new-prompt"); !ok {
		t.Fatal("old cleanup removed replacement prompt")
	}
	c.Cancel()
	waitTurnDoneEvent(t, done)
}

type observedCancelContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *observedCancelContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestCancelScopedRevocationWhileWaitingAdmission(t *testing.T) {
	c := scopedCancelController(t, event.Discard)
	started := make(chan context.Context, 1)
	c.runGuarded(func(ctx context.Context) error { started <- ctx; <-ctx.Done(); return ctx.Err() })
	turnCtx := <-started
	scope := scopedCancelIdentity(c)
	operation, revoke := context.WithCancel(context.Background())
	defer revoke()
	observed := &observedCancelContext{Context: operation, checked: make(chan struct{})}
	result := make(chan error, 1)
	c.mu.Lock()
	go func() { result <- c.CancelScopedContext(observed, scope) }()
	select {
	case <-observed.checked:
	case <-time.After(5 * time.Second):
		c.mu.Unlock()
		t.Fatal("operation did not reach initial context check")
	}
	revoke()
	c.mu.Unlock()
	select {
	case err := <-result:
		if !errors.Is(err, ErrTurnCancelScope) || turnCtx.Err() != nil {
			t.Fatal("revocation during lock wait stopped active turn", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked operation did not settle")
	}
}
