package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

func observeDesktopTest(t *testing.T, c *Controller, ctx context.Context) *DesktopEventSubscription {
	t.Helper()
	state := c.RuntimeStateSnapshot()
	sub, err := c.ObserveDesktopEvents(ctx, DesktopEventScope{SessionPath: c.SessionPath(), RuntimeEpoch: state.RuntimeEpoch})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub
}

func TestDesktopEventsActualControllerKindsOnlyNoReplayOrGrant(t *testing.T) {
	c, inputs, done := desktopDrivingController(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := observeDesktopTest(t, c, ctx)
	select {
	case <-sub.frames:
		t.Fatal("observer replayed history")
	default:
	}
	c.SubmitUserTurn("private user/provider body", "private display")
	for _, want := range []string{"turn_started", "turn_done"} {
		frame, err := sub.Read(ctx)
		if err != nil || frame.Kind != want {
			t.Fatal(frame, err)
		}
		raw, _ := json.Marshal(frame)
		if string(raw) != `{"Kind":"`+want+`"}` {
			t.Fatal("observer exposed non-kind data", string(raw))
		}
	}
	waitTurnDoneEvent(t, done)
	waitDesktopDrivingIdleCommit(t, c)
	if <-inputs != "private user/provider body" {
		t.Fatal("observation changed literal input")
	}
	c.mu.Lock()
	granted := c.desktopDriving.active != nil
	c.mu.Unlock()
	if granted {
		t.Fatal("observation acquired driving")
	}
}

func TestDesktopEventsLimitsRetirementRejectQueuedPrefixAndPrivateKinds(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	sub := observeDesktopTest(t, c, context.Background())
	ledger := c.turnEventLedger()
	c.desktopEvents.publish(ledger, event.Notice)
	select {
	case <-sub.frames:
		t.Fatal("unapproved kind published")
	default:
	}
	for range 33 {
		c.desktopEvents.publish(ledger, event.TurnStarted)
	}
	if _, err := sub.Read(context.Background()); !errors.Is(err, ErrDesktopObservation) {
		t.Fatal("overflow returned stale prefix", err)
	}
	for range 8 {
		observeDesktopTest(t, c, context.Background())
	}
	state := c.RuntimeStateSnapshot()
	if _, err := c.ObserveDesktopEvents(context.Background(), DesktopEventScope{c.SessionPath(), state.RuntimeEpoch}); !errors.Is(err, ErrDesktopObservation) {
		t.Fatal("unbounded subscribers", err)
	}
	c.SetSessionPath(c.SessionPath())
	c.desktopEvents.mu.Lock()
	remaining := len(c.desktopEvents.subs)
	c.desktopEvents.mu.Unlock()
	if remaining != 0 {
		t.Fatal("rebind retained old observers", remaining)
	}
}

func TestDesktopEventsOwnerCancellationAndCloseInterruptBlockedReaders(t *testing.T) {
	for _, action := range []string{"cancel", "close", "rebind", "ledger_failure"} {
		t.Run(action, func(t *testing.T) {
			c, _, _ := desktopDrivingController(t)
			owner, cancel := context.WithCancel(context.Background())
			defer cancel()
			sub := observeDesktopTest(t, c, owner)
			read := make(chan error, 1)
			go func() { _, err := sub.Read(context.Background()); read <- err }()
			switch action {
			case "cancel":
				cancel()
			case "close":
				c.Close()
			case "rebind":
				c.SetSessionPath(c.SessionPath())
			case "ledger_failure":
				c.failTurnEventLedger(errors.New("private persistence failure"))
			}
			select {
			case err := <-read:
				if !errors.Is(err, ErrDesktopObservation) {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("retired reader did not cancel")
			}
			select {
			case <-sub.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("observer lifetime did not retire")
			}
		})
	}
}

func TestDesktopEventsWrongInstanceCannotSubscribe(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	state := c.RuntimeStateSnapshot()
	for _, scope := range []DesktopEventScope{{c.SessionPath(), state.RuntimeEpoch + "-other"}, {c.SessionPath() + "-saved", state.RuntimeEpoch}, {}} {
		if _, err := c.ObserveDesktopEvents(context.Background(), scope); !errors.Is(err, ErrDesktopObservation) {
			t.Fatal("observer adopted another owner", err)
		}
	}
}

func TestDesktopEventsApprovedKindsMatchSharedWireNames(t *testing.T) {
	c, _, _ := desktopDrivingController(t)
	sub := observeDesktopTest(t, c, context.Background())
	for _, kind := range []event.Kind{event.TurnStarted, event.TurnDone, event.AskRequest, event.ApprovalRequest, event.MCPInteractionRequest} {
		c.desktopEvents.publish(c.turnEventLedger(), kind)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		frame, err := sub.Read(ctx)
		cancel()
		want, ok := eventwire.KindName(kind)
		if err != nil || !ok || frame.Kind != want {
			t.Fatal("observation kind diverged from shared protocol", frame, want, err)
		}
	}
}
