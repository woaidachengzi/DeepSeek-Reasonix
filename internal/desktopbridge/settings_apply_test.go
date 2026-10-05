package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

type settingsTestRuntime struct {
	*fakeRuntime
	applied, desired string
	readErr          error
	releases         int
}

func (r *settingsTestRuntime) ModelSettingsState() (string, string, error) {
	return r.applied, r.desired, r.readErr
}
func (r *settingsTestRuntime) ReleaseForReplacement() error { r.releases++; return nil }

type settingsTestFactory struct {
	initial *settingsTestRuntime
	rebuild func(context.Context, Runtime, OpenRequest) (SettingsRuntime, error)
	calls   int
}

func (f *settingsTestFactory) Open(context.Context, OpenRequest) (Runtime, error) {
	return f.initial, nil
}
func (f *settingsTestFactory) Rebuild(ctx context.Context, old Runtime, req OpenRequest) (SettingsRuntime, error) {
	f.calls++
	return f.rebuild(ctx, old, req)
}
func settingsManager(t *testing.T, rebuild func(context.Context, Runtime, OpenRequest) (SettingsRuntime, error)) (*RuntimeManager, *settingsTestFactory) {
	t.Helper()
	factory := &settingsTestFactory{initial: &settingsTestRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, applied: "old", desired: "new"}, rebuild: rebuild}
	manager := NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/workspace", ModelRef: "provider/chat"}); err != nil {
		t.Fatal(err)
	}
	return manager, factory
}
func TestSettingsReadFailureRejectsSubmit(t *testing.T) {
	manager, factory := settingsManager(t, nil)
	factory.initial.readErr = errors.New("cannot read settings")
	if _, err := manager.Submit(context.Background(), "a", "hello"); !errors.Is(err, ErrSessionSettingsApply) {
		t.Fatal(err)
	}
	if len(factory.initial.submits) != 0 || factory.calls != 0 || factory.initial.releases != 0 {
		t.Fatal("failed read admitted work or changed runtime")
	}
	if view, ok := manager.Snapshot(); !ok || view.ID != "a" {
		t.Fatal("lost session")
	}
}
func TestSettingsBuildFailurePreservesRuntimeAndAllowsRetry(t *testing.T) {
	next := &settingsTestRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, applied: "new", desired: "new"}
	fail := true
	manager, factory := settingsManager(t, func(_ context.Context, old Runtime, request OpenRequest) (SettingsRuntime, error) {
		if request.SessionID != "a" || request.ModelRef != "provider/chat" || request.WorkspaceRoot != "/workspace" {
			t.Fatalf("changed session identity: %+v", request)
		}
		if fail {
			return nil, errors.New("provider cannot build")
		}
		return next, nil
	})
	if _, err := manager.Submit(context.Background(), "a", "hello"); !errors.Is(err, ErrSessionSettingsApply) {
		t.Fatal(err)
	}
	if _, ok := manager.Snapshot(); !ok || factory.initial.releases != 0 || factory.initial.shutdownCalls.Load() != 0 {
		t.Fatal("failed build destroyed previous runtime")
	}
	fail = false
	if _, err := manager.Submit(context.Background(), "a", "retry"); err != nil {
		t.Fatal(err)
	}
	if len(next.submits) != 1 || next.submits[0] != "retry" || factory.initial.releases != 1 || factory.initial.shutdownCalls.Load() != 0 {
		t.Fatal("retry did not migrate and admit exactly once")
	}
}
func TestSettingsReplacementValidationPreservesPrevious(t *testing.T) {
	for _, test := range []struct {
		name string
		next *settingsTestRuntime
	}{
		{"nil", nil},
		{"missing path", &settingsTestRuntime{fakeRuntime: &fakeRuntime{state: "idle"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, factory := settingsManager(t, func(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
				if test.next == nil {
					return nil, nil
				}
				return test.next, nil
			})
			if _, err := manager.Submit(context.Background(), "a", "hello"); !errors.Is(err, ErrSessionSettingsApply) {
				t.Fatal(err)
			}
			if _, ok := manager.Snapshot(); !ok || factory.initial.releases != 0 {
				t.Fatal("invalid replacement lost old runtime")
			}
			if test.next != nil && test.next.releases != 1 {
				t.Fatal("invalid candidate leaked")
			}
		})
	}
}
func TestSettingsCancellationAndActiveWorkPreserveRuntime(t *testing.T) {
	manager, factory := settingsManager(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Submit(ctx, "a", "hello"); !errors.Is(err, ErrSessionSettingsApply) {
		t.Fatal(err)
	}
	for _, state := range []string{"running", "paused", "deleting"} {
		factory.initial.state = state
		if _, err := manager.Submit(context.Background(), "a", "hello"); !errors.Is(err, ErrSessionConflict) {
			t.Fatalf("%s: %v", state, err)
		}
	}
	if factory.calls != 0 || factory.initial.releases != 0 || len(factory.initial.submits) != 0 {
		t.Fatal("active/canceled work changed runtime")
	}
}
func TestSettingsSubmitSerializesConcurrentShutdown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	next := &settingsTestRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, applied: "new", desired: "new"}
	manager, factory := settingsManager(t, func(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
		close(entered)
		<-release
		return next, nil
	})
	submitted := make(chan error, 1)
	go func() { _, err := manager.Submit(context.Background(), "a", "hello"); submitted <- err }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- manager.Shutdown() }()
	close(release)
	if err := <-submitted; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if len(next.submits) != 1 || next.shutdownCalls.Load() != 1 || factory.initial.releases != 1 {
		t.Fatal("shutdown interrupted swap/admission or leaked runtime")
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("runtime remained after shutdown")
	}
}
