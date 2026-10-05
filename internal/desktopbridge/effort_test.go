package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

func TestEffortFailureBusyAndWrongModelPreserveOwnership(t *testing.T) {
	manager, factory := settingsManager(t, func(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
		return nil, errors.New("refused")
	})
	if _, err := manager.SetSessionEffort(context.Background(), "a", "provider/chat", "high"); err == nil {
		t.Fatal("failure ignored")
	}
	if factory.initial.releases != 0 {
		t.Fatal("failure released old runtime")
	}
	for _, state := range []string{"running", "paused"} {
		factory.initial.state = state
		if _, err := manager.SetSessionEffort(context.Background(), "a", "provider/chat", "high"); !errors.Is(err, ErrSessionConflict) {
			t.Fatal(err)
		}
	}
	factory.initial.state = "idle"
	if _, err := manager.SetSessionEffort(context.Background(), "a", "other/model", "high"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if factory.calls != 1 {
		t.Fatal("invalid/busy selection reached builder")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.SetSessionEffort(ctx, "a", "provider/chat", "high"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
