package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

func drivingReclaimFixture(t *testing.T) (*Controller, DesktopDrivingScope, string, *DesktopDrivingReclaimObservation) {
	t.Helper()
	c, _, _ := desktopDrivingController(t)
	scope, key := desktopDrivingCapture(t, c), desktopDrivingTestKey(1)
	if err := c.AcquireDesktopDriving(context.Background(), scope, key); err != nil {
		t.Fatal(err)
	}
	o, err := c.ObserveDesktopDrivingReclaim(context.Background(), scope, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Close)
	return c, scope, key, o
}

func awaitDrivingReclaimClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("original observation did not terminate")
	}
}

func TestDesktopDrivingReclaimActualInputOnlyAndOriginalKeyRelease(t *testing.T) {
	c, scope, key, o := drivingReclaimFixture(t)
	if err := c.SubmitDesktopDriving(context.Background(), scope.TurnSubmitScope, key, "private literal remote input"); err != nil {
		t.Fatal(err)
	}
	waitDesktopDrivingIdleCommit(t, c)
	select {
	case <-o.Done():
		t.Fatal("scoped driving input reclaimed its own grant")
	default:
	}
	c.SubmitHTTP("/definitely_unknown_desktop_drive_test_command")
	awaitDrivingReclaimClosed(t, o.Done())
	if !o.Reclaimed() || c.DesktopDrivingActive(context.Background(), scope, key) {
		t.Fatal("actual user input did not revoke original holder")
	}
	select {
	case <-o.Retired():
		t.Fatal("local signal erased source lifetime")
	default:
	}
	// Local HTTP input can dispatch a turn after signalling reclaim. A fresh
	// takeover still requires that turn's actual idle commit, not just Done.
	waitDesktopDrivingIdleCommit(t, c)
	newScope, newKey := desktopDrivingCapture(t, c), desktopDrivingTestKey(2)
	if err := c.AcquireDesktopDriving(context.Background(), newScope, newKey); err != nil {
		t.Fatal(err)
	}
	if err := c.ReleaseDesktopDriving(context.Background(), scope, key); err != nil {
		t.Fatal(err)
	}
	awaitDrivingReclaimClosed(t, o.Retired())
	if !c.DesktopDrivingActive(context.Background(), newScope, newKey) {
		t.Fatal("spent original key affected replacement grant")
	}
}

func TestDesktopDrivingReclaimRetirementIsNotLocalInput(t *testing.T) {
	for _, action := range []string{"release", "close", "same_path", "ledger_failure", "expired_local_input", "expiry_timer"} {
		t.Run(action, func(t *testing.T) {
			c, scope, key, o := drivingReclaimFixture(t)
			switch action {
			case "release":
				if err := c.ReleaseDesktopDriving(context.Background(), scope, key); err != nil {
					t.Fatal(err)
				}
			case "close":
				c.Close()
			case "same_path":
				c.SetSessionPath(scope.SessionPath)
			case "ledger_failure":
				c.failTurnEventLedger(errors.New("private fixture failure"))
			case "expired_local_input":
				c.mu.Lock()
				c.desktopDriving.active.expires = time.Now().Add(-time.Second)
				c.mu.Unlock()
				c.SubmitHTTP("/definitely_unknown_desktop_drive_test_command")
			case "expiry_timer":
				o.Close()
				c.mu.Lock()
				c.desktopDriving.active.expires = time.Now().Add(250 * time.Millisecond)
				c.mu.Unlock()
				var err error
				o, err = c.ObserveDesktopDrivingReclaim(context.Background(), scope, key)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(o.Close)
			}
			awaitDrivingReclaimClosed(t, o.Done())
			awaitDrivingReclaimClosed(t, o.Retired())
			if o.Reclaimed() {
				t.Fatal("retirement or expired grant falsely reported local input")
			}
		})
	}
}

func TestDesktopDrivingReclaimRequiresExactActiveGrantAndBoundedObservers(t *testing.T) {
	c, scope, key, o := drivingReclaimFixture(t)
	wrong := scope
	wrong.RuntimeEpoch += "-replacement"
	for _, candidate := range []struct {
		scope DesktopDrivingScope
		key   string
	}{{wrong, key}, {scope, desktopDrivingTestKey(2)}} {
		if _, err := c.ObserveDesktopDrivingReclaim(context.Background(), candidate.scope, candidate.key); err == nil {
			t.Fatal("foreign grant observed")
		}
	}
	var all []*DesktopDrivingReclaimObservation
	all = append(all, o)
	for range 7 {
		observer, err := c.ObserveDesktopDrivingReclaim(context.Background(), scope, key)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, observer)
		t.Cleanup(observer.Close)
	}
	if _, err := c.ObserveDesktopDrivingReclaim(context.Background(), scope, key); err == nil {
		t.Fatal("unbounded observation")
	}
	for _, observer := range all {
		observer.Close()
	}
	if !c.DesktopDrivingActive(context.Background(), scope, key) {
		t.Fatal("closing observation released grant")
	}
	ctx, cancel := context.WithCancel(context.Background())
	observer, err := c.ObserveDesktopDrivingReclaim(ctx, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	cancel()
	awaitDrivingReclaimClosed(t, observer.Retired())
	if observer.Reclaimed() || !c.DesktopDrivingActive(context.Background(), scope, key) {
		t.Fatal("cancelled reader mutated driving")
	}
	c.revokeDesktopDriving()
	if _, err := c.ObserveDesktopDrivingReclaim(context.Background(), scope, key); err == nil {
		t.Fatal("spent capture replayed a reclaim signal")
	}
}
