package control

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/event"
)

func desktopDrivingTestKey(n int) string { return fmt.Sprintf("%032x", n) }

func desktopDrivingCapture(t *testing.T, c *Controller) DesktopDrivingScope {
	t.Helper()
	scope, err := c.CaptureDesktopDriving(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func waitDesktopDrivingIdleCommit(t *testing.T, c *Controller) {
	t.Helper()
	waitIdleAdmission(t, c)
	deadline := time.Now().Add(5 * time.Second)
	for {
		state := c.RuntimeStateSnapshot()
		if state.Phase == "idle" && !state.Running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("controller did not publish its completed idle boundary")
		}
		time.Sleep(time.Millisecond)
	}
}

func desktopDrivingController(t *testing.T) (*Controller, chan string, chan event.Event) {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	dir := t.TempDir()
	inputs, done := make(chan string, 32), make(chan event.Event, 32)
	c := New(Options{Runner: &scopedSubmitTextRunner{input: inputs}, SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- e
		}
	})})
	t.Cleanup(func() {
		c.mu.Lock()
		cancel := c.cancel
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		waitIdleAdmission(t, c)
		c.Close()
	})
	return c, inputs, done
}

func TestDesktopDrivingCaptureIsBoundedIdempotentAndNeverRevivesSpentKey(t *testing.T) {
	c, inputs, _ := desktopDrivingController(t)
	scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
	before := c.RuntimeStateSnapshot()
	wrong := scope
	wrong.RuntimeEpoch += "-replacement"
	for _, invalid := range []struct {
		scope DesktopDrivingScope
		key   string
	}{{wrong, key}, {DesktopDrivingScope{}, key}, {scope, "raw-id"}} {
		if err := c.AcquireDesktopDriving(context.Background(), invalid.scope, invalid.key); !errors.Is(err, ErrDesktopDrivingScope) {
			t.Fatal("invalid capture admitted", err)
		}
	}
	if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	expiry := c.desktopDriving.active.expires
	c.mu.Unlock()
	if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
		t.Fatal("unknown-acquire observation was not idempotent", err)
	}
	c.mu.Lock()
	unchanged := c.desktopDriving.active.expires == expiry
	c.mu.Unlock()
	if !unchanged || c.RuntimeStateSnapshot() != before || len(inputs) != 0 {
		t.Fatal("capture renewed, started a turn or changed runtime state")
	}
	if err := c.AcquireDesktopDriving(context.Background(), scope, desktopDrivingTestKey(2)); err == nil {
		t.Fatal("another holder silently stole control")
	}
	c.mu.Lock()
	c.desktopDriving.active.expires = time.Now().Add(-time.Second)
	c.mu.Unlock()
	if c.DesktopDrivingActive(context.Background(), scope, key) || c.AcquireDesktopDriving(context.Background(), scope, key) == nil {
		t.Fatal("expiry rearmed the spent key")
	}
	other := desktopDrivingTestKey(2)
	scope = desktopDrivingCapture(t, c)
	if c.AcquireDesktopDriving(context.Background(), scope, key) == nil {
		t.Fatal("fresh control generation rearmed spent key")
	}
	if err := c.AcquireDesktopDriving(context.Background(), scope, other); err != nil {
		t.Fatal(err)
	}
	if err := c.ReleaseDesktopDriving(context.Background(), scope, key); err != nil || !c.DesktopDrivingActive(context.Background(), scope, other) {
		t.Fatal("old release affected replacement holder", err)
	}
	if err := c.ReleaseDesktopDriving(context.Background(), scope, other); err != nil || c.DesktopDrivingActive(context.Background(), scope, other) {
		t.Fatal("release did not revoke", err)
	}
	if c.AcquireDesktopDriving(context.Background(), scope, other) == nil {
		t.Fatal("released holder revived")
	}
}

func TestDesktopDrivingActualLiteralTurnsUseCurrentRevisionButSameHolder(t *testing.T) {
	c, inputs, done := desktopDrivingController(t)
	capture, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
	if err := c.AcquireDesktopDriving(context.Background(), capture, key); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"/new", "!echo must-not-execute"} {
		scope := scopedSubmitIdentity(c)
		if err := c.SubmitDesktopDriving(context.Background(), scope, key, text); err != nil {
			t.Fatal(err)
		}
		waitTurnDoneEvent(t, done)
		waitDesktopDrivingIdleCommit(t, c)
		if got := <-inputs; got != text || c.SessionPath() != capture.SessionPath || !c.DesktopDrivingActive(context.Background(), capture, key) {
			t.Fatal("literal input switched owner or reclaimed its own grant", got)
		}
		if c.SubmitDesktopDriving(context.Background(), scope, key, "stale repeat") == nil {
			t.Fatal("old idle revision dispatched a duplicate")
		}
	}
	if err := c.AcquireDesktopDriving(context.Background(), capture, key); err != nil {
		t.Fatal("original active capture did not remain idempotent", err)
	}
	if err := c.SubmitScopedContext(context.Background(), scopedSubmitIdentity(c), "ordinary remote UI input"); err != nil {
		t.Fatal(err)
	}
	waitTurnDoneEvent(t, done)
	waitDesktopDrivingIdleCommit(t, c)
	<-inputs
	if c.DesktopDrivingActive(context.Background(), capture, key) || c.SubmitDesktopDriving(context.Background(), scopedSubmitIdentity(c), key, "old holder") == nil || len(inputs) != 0 {
		t.Fatal("ordinary scoped user input did not reclaim")
	}
}

func TestDesktopDrivingOrdinaryInputEntrypointsReclaimBeforeDispatch(t *testing.T) {
	const unknown = "/definitely_unknown_desktop_drive_test_command"
	for _, tc := range []struct {
		name  string
		input func(*Controller)
	}{
		{"submit", func(c *Controller) { c.Submit(unknown) }},
		{"http", func(c *Controller) { c.SubmitHTTP(unknown) }},
		{"http-format", func(c *Controller) { c.SubmitHTTPFormat(unknown, "json_object") }},
		{"display", func(c *Controller) { c.SubmitDisplay("display", unknown) }},
		{"edited", func(c *Controller) { c.SubmitEditedDisplay("display", unknown, "original") }},
		{"invocation", func(c *Controller) { c.SubmitInvocationDisplay("display", unknown, nil) }},
		{"user-turn", func(c *Controller) { c.SubmitUserTurn("ordinary user text", "display") }},
		{"steer", func(c *Controller) { c.TrySteer("local guidance") }},
		{"guarded-inbox", func(c *Controller) { c.runGuardedInbox(func(context.Context) error { return nil }, nil) }},
		{"synchronous", func(c *Controller) {
			_ = c.runSynchronousTurn(context.Background(), nil, func(context.Context) error { return nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := desktopDrivingController(t)
			scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
			if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
				t.Fatal(err)
			}
			tc.input(c)
			if c.DesktopDrivingActive(context.Background(), scope, key) {
				t.Fatal("ordinary user input left driving grant active")
			}
		})
	}
}

func TestDesktopDrivingPausedQueueReclaimsBeforeDispatch(t *testing.T) {
	c, inputs, _ := desktopDrivingController(t)
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	for n, source := range []string{"http", "plan_revision"} {
		scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(n+1)
		if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
			t.Fatal(err)
		}
		if _, err := c.EnqueueInbox(InboxRequest{Submit: "queued local input", Source: source}); err != nil {
			t.Fatal(err)
		}
		if c.DesktopDrivingActive(context.Background(), scope, key) || len(inputs) != 0 || c.Running() {
			t.Fatal("paused input did not reclaim or source metadata bypassed it")
		}
	}
}

func TestDesktopDrivingRevocationWhileAdmissionWaitsAndOwnerRetirement(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
	if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(context.Background())
	checked := &scopedSubmitCheckedContext{Context: request, checked: make(chan struct{})}
	done := make(chan error, 1)
	c.runtimeState.mu.Lock()
	go func() { done <- c.SubmitDesktopDriving(checked, scope.TurnSubmitScope, key, "must not run") }()
	<-checked.checked
	cancel()
	c.runtimeState.mu.Unlock()
	if err := <-done; !errors.Is(err, ErrDesktopDrivingScope) {
		t.Fatal("cancel while waiting admitted", err)
	}
	// A local input can revoke while a remote submit waits for the runtime lock.
	c.runtimeState.mu.Lock()
	go func() {
		done <- c.SubmitDesktopDriving(context.Background(), scope.TurnSubmitScope, key, "revoked holder")
	}()
	c.revokeDesktopDriving()
	c.runtimeState.mu.Unlock()
	if err := <-done; !errors.Is(err, ErrDesktopDrivingScope) {
		t.Fatal("non-atomic holder check admitted", err)
	}
	if c.AcquireDesktopDriving(context.Background(), scope, key) == nil {
		t.Fatal("local revoke revived spent key")
	}
	other := desktopDrivingTestKey(2)
	scope = desktopDrivingCapture(t, c)
	if err := c.AcquireDesktopDriving(context.Background(), scope, other); err != nil {
		t.Fatal(err)
	}
	c.SetSessionPath(scope.SessionPath)
	if c.DesktopDrivingActive(context.Background(), scope, other) {
		t.Fatal("same-path rebind retained control")
	}
	c.Close()
	if c.AcquireDesktopDriving(context.Background(), scope, desktopDrivingTestKey(3)) == nil {
		t.Fatal("closed owner acquired control")
	}
}

func TestDesktopDrivingUsedKeyCapacityFailsClosed(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	scope := desktopDrivingCapture(t, c)
	for n := 1; n <= desktopDrivingKeyLimit; n++ {
		scope = desktopDrivingCapture(t, c)
		key := desktopDrivingTestKey(n)
		if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
			t.Fatal(n, err)
		}
		if err := c.ReleaseDesktopDriving(context.Background(), scope, key); err != nil {
			t.Fatal(err)
		}
	}
	scope = desktopDrivingCapture(t, c)
	if c.AcquireDesktopDriving(context.Background(), scope, desktopDrivingTestKey(1)) == nil || c.AcquireDesktopDriving(context.Background(), scope, desktopDrivingTestKey(desktopDrivingKeyLimit+1)) == nil {
		t.Fatal("capacity evicted spent keys or allowed unbounded grants")
	}
}

func TestDesktopDrivingIndependentControlVersionRejectsOldCaptureWithFreshKey(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	old := desktopDrivingCapture(t, c)
	// Reclaim before any holder exists, without changing runtime/turn state.
	c.revokeDesktopDriving()
	fresh := desktopDrivingCapture(t, c)
	if fresh.TurnSubmitScope != old.TurnSubmitScope || fresh.ControlVersion <= old.ControlVersion {
		t.Fatal("fixture did not isolate the independent control generation")
	}
	if c.AcquireDesktopDriving(context.Background(), old, desktopDrivingTestKey(1)) == nil {
		t.Fatal("old capture acquired using a fresh never-used key")
	}
	if err := c.AcquireDesktopDriving(context.Background(), fresh, desktopDrivingTestKey(1)); err != nil {
		t.Fatal("new explicit capture could not acquire", err)
	}
	// Counter exhaustion cannot wrap an old capture back into authority.
	c.mu.Lock()
	c.desktopDriving.version = desktopDrivingVersionLimit
	c.revokeDesktopDrivingLocked()
	c.mu.Unlock()
	if _, err := c.CaptureDesktopDriving(context.Background()); err == nil {
		t.Fatal("control generation overflow admitted a capture")
	}
}

func TestDesktopDrivingConcurrentCaptureHasOneWinnerAndCanceledCaptureCreatesNothing(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	scope := desktopDrivingCapture(t, c)
	request, cancel := context.WithCancel(context.Background())
	checked := &scopedSubmitCheckedContext{Context: request, checked: make(chan struct{})}
	aborted := make(chan error, 1)
	c.runtimeState.mu.Lock()
	go func() { aborted <- c.AcquireDesktopDriving(checked, scope, desktopDrivingTestKey(99)) }()
	<-checked.checked
	cancel()
	c.runtimeState.mu.Unlock()
	if err := <-aborted; !errors.Is(err, ErrDesktopDrivingScope) {
		t.Fatal("canceled waiting capture acquired", err)
	}
	c.mu.Lock()
	empty := c.desktopDriving.active == nil && len(c.desktopDriving.used) == 0
	c.mu.Unlock()
	if !empty {
		t.Fatal("canceled capture allocated a grant or spent key")
	}
	type result struct {
		key string
		err error
	}
	results := make(chan result, 16)
	start := make(chan struct{})
	for n := 1; n <= 16; n++ {
		go func(key string) {
			<-start
			results <- result{key, c.AcquireDesktopDriving(context.Background(), scope, key)}
		}(desktopDrivingTestKey(n))
	}
	close(start)
	winners := 0
	for range 16 {
		got := <-results
		if got.err == nil {
			winners++
			if !c.DesktopDrivingActive(context.Background(), scope, got.key) {
				t.Fatal("winning capture lost authority without explicit reclaim")
			}
		}
	}
	if winners != 1 {
		t.Fatal("concurrent captures did not have exactly one winner", winners)
	}
}

type desktopDrivingBlockingRunner struct{ started chan context.Context }

func (r *desktopDrivingBlockingRunner) Run(ctx context.Context, _ string) error {
	r.started <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestDesktopDrivingReleaseOrLocalReclaimDoesNotCancelAcceptedTurn(t *testing.T) {
	for _, action := range []string{"release", "local-input"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("REASONIX_HOME", t.TempDir())
			t.Setenv("REASONIX_STATE_HOME", t.TempDir())
			t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
			t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
			dir := t.TempDir()
			runner := &desktopDrivingBlockingRunner{started: make(chan context.Context, 1)}
			done := make(chan event.Event, 1)
			c := New(Options{Runner: runner, SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.TurnDone {
					done <- e
				}
			})})
			t.Cleanup(func() { c.Cancel(); waitIdleAdmission(t, c); c.Close() })
			scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
			if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
				t.Fatal(err)
			}
			request, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := c.SubmitDesktopDriving(request, scope.TurnSubmitScope, key, "accepted task"); err != nil {
				t.Fatal(err)
			}
			var turn context.Context
			select {
			case turn = <-runner.started:
			case <-time.After(5 * time.Second):
				t.Fatal("actual runner not started")
			}
			cancel()
			if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
				t.Fatal("original active capture not idempotent while running", err)
			}
			if action == "release" {
				if err := c.ReleaseDesktopDriving(context.Background(), scope, key); err != nil {
					t.Fatal(err)
				}
			} else {
				c.Submit("/definitely_unknown_desktop_drive_test_command")
			}
			if c.DesktopDrivingActive(context.Background(), scope, key) || turn.Err() != nil || c.RuntimeStatus().CancelRequested {
				t.Fatal("revoke did not retire holder or cancelled accepted work")
			}
			c.Cancel()
			waitTurnDoneEvent(t, done)
			waitIdleAdmission(t, c)
		})
	}
}
