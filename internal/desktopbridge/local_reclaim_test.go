package desktopbridge

import (
	"context"
	"testing"
)

func TestLocalReclaimOnlyActualLocalInputAndNoGrant(t *testing.T) {
	m, _, view := commandManager(t)
	o, err := m.ObserveLocalReclaim(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := m.SubmitOwned(context.Background(), view, "remote literal"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-o.Done():
		t.Fatal("remote scoped input reported local reclaim")
	default:
	}
	if _, err := m.Submit(context.Background(), view.Scope.SessionID, "local private body"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-o.Done():
	default:
		t.Fatal("actual local fence did not signal")
	}
	if !o.Reclaimed() {
		t.Fatal("local fence not confirmed")
	}
	select {
	case <-o.Retired():
		t.Fatal("signal erased source lifetime")
	default:
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-o.Retired():
	default:
		t.Fatal("source retirement did not fence IO")
	}
}

func TestLocalReclaimRejectsStaleAndRetirementIsNotInput(t *testing.T) {
	for _, action := range []string{"cancel", "close"} {
		t.Run(action, func(t *testing.T) {
			m, _, view := commandManager(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			o, err := m.ObserveLocalReclaim(ctx, view)
			if err != nil {
				t.Fatal(err)
			}
			if action == "cancel" {
				cancel()
			} else {
				_ = m.Shutdown()
			}
			<-o.Done()
			<-o.Retired()
			if o.Reclaimed() {
				t.Fatal("retirement misreported as local input")
			}
		})
	}
	m, _, view := commandManager(t)
	var observations []*OwnedLocalReclaimObservation
	for range 8 {
		o, err := m.ObserveLocalReclaim(context.Background(), view)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(o.Close)
		observations = append(observations, o)
	}
	if _, err := m.ObserveLocalReclaim(context.Background(), view); err == nil {
		t.Fatal("unbounded observations")
	}
	if _, err := m.Submit(context.Background(), view.Scope.SessionID, "local"); err != nil {
		t.Fatal(err)
	}
	// Free the observer budget so rejection proves the input-version fence,
	// rather than accidentally passing because all eight slots remain occupied.
	for _, o := range observations {
		o.Close()
	}
	if _, err := m.ObserveLocalReclaim(context.Background(), view); err == nil {
		t.Fatal("stale input fence adopted")
	}
}
