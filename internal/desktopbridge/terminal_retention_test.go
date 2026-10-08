package desktopbridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type retentionRuntimeFixture struct {
	*fakeModelRuntime
	resource *atomic.Int32
	reject   bool
	stale    bool
	retains  int
	releases int
	commits  int
}

func (r *retentionRuntimeFixture) CommitReplacement() { r.commits++ }

func (r *retentionRuntimeFixture) RetainTerminals() (TerminalRetention, error) {
	r.retains++
	lease := &retentionLeaseFixture{resource: r.resource}
	r.resource = nil
	return lease, nil
}
func (r *retentionRuntimeFixture) Shutdown() error {
	if r.resource != nil {
		r.resource.Add(1)
		r.resource = nil
	}
	return r.fakeRuntime.Shutdown()
}
func (r *retentionRuntimeFixture) ReleaseForReplacement() error {
	r.releases++
	if r.resource != nil {
		r.resource.Add(1)
		r.resource = nil
	}
	return nil
}
func (r *retentionRuntimeFixture) ModelSettingsState() (string, string, error) {
	if r.stale {
		return "old", "new", nil
	}
	return "new", "new", nil
}

type retentionLeaseFixture struct {
	resource *atomic.Int32
	consumed bool
}

func (l *retentionLeaseFixture) Attach(runtime Runtime) error {
	r, ok := runtime.(*retentionRuntimeFixture)
	if !ok || r.reject || l.consumed {
		return ErrTerminalUnavailable
	}
	r.resource = l.resource
	l.resource, l.consumed = nil, true
	return nil
}
func (l *retentionLeaseFixture) Close() error {
	if !l.consumed && l.resource != nil {
		l.resource.Add(1)
	}
	l.consumed, l.resource = true, nil
	return nil
}
func retainedFixture(model string) *retentionRuntimeFixture {
	return &retentionRuntimeFixture{fakeModelRuntime: &fakeModelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/owned.jsonl", state: "idle"}, model: model}}
}

func TestModelReplacementRetainsTerminalsOnSuccessAndRecovery(t *testing.T) {
	for _, outcome := range []string{"success", "target fails", "attachment fails", "recovery fails"} {
		t.Run(outcome, func(t *testing.T) {
			var closed atomic.Int32
			initial := retainedFixture("provider/old")
			initial.resource = &closed
			opens := 0
			m := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
				opens++
				if opens == 1 {
					return initial, nil
				}
				if outcome == "recovery fails" || (outcome == "target fails" && opens == 2) {
					return nil, errors.New("owned fixture build failure")
				}
				next := retainedFixture(req.ModelRef)
				next.reject = outcome == "attachment fails" && opens == 2
				return next, nil
			}))
			if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", ModelRef: "provider/old"}); err != nil {
				t.Fatal(err)
			}
			_, err := m.SetSessionModel(context.Background(), "owned", "provider/new")
			if outcome == "success" && err != nil {
				t.Fatal(err)
			}
			if outcome != "success" && err == nil {
				t.Fatal("injected failure was ignored")
			}
			if outcome == "recovery fails" {
				if !errors.Is(err, ErrSessionModelRecover) || closed.Load() != 1 {
					t.Fatal("unattached resources survived failed recovery")
				}
			} else {
				if closed.Load() != 0 || initial.shutdownCalls.Load() != 1 {
					t.Fatal("replacement closed retained terminals")
				}
				if outcome != "success" && !errors.Is(err, ErrSessionModelSwitch) {
					t.Fatal("missing restored-session failure")
				}
			}
			if err := m.Shutdown(); err != nil || closed.Load() != 1 {
				t.Fatal("terminal lease was leaked or closed twice")
			}
		})
	}
}

func TestShutdownDuringModelBuildClosesRetainedTerminals(t *testing.T) {
	var closed atomic.Int32
	initial := retainedFixture("provider/old")
	initial.resource = &closed
	entered, release := make(chan struct{}), make(chan struct{})
	opens := 0
	m := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		opens++
		if opens == 1 {
			return initial, nil
		}
		close(entered)
		<-release
		return retainedFixture(req.ModelRef), nil
	}))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", ModelRef: "provider/old"}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := m.SetSessionModel(context.Background(), "owned", "provider/new"); finished <- err }()
	<-entered
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 1 {
		t.Fatal("shutdown left the terminal lease alive while the factory was blocked")
	}
	close(release)
	if err := <-finished; !errors.Is(err, ErrClosed) || closed.Load() != 1 {
		t.Fatal("shutdown raced retained resource publication")
	}
}

type retentionFactoryFixture struct {
	initial, next *retentionRuntimeFixture
	fail          bool
}

func (f *retentionFactoryFixture) Open(context.Context, OpenRequest) (Runtime, error) {
	return f.initial, nil
}
func (f *retentionFactoryFixture) Rebuild(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
	if f.fail {
		return nil, errors.New("fixture settings failure")
	}
	return f.next, nil
}

func TestSettingsAndEffortTransferTerminalsOnlyAfterCandidateValidation(t *testing.T) {
	for _, action := range []string{"settings", "effort"} {
		for _, outcome := range []string{"success", "build fails", "attachment fails"} {
			t.Run(action+"/"+outcome, func(t *testing.T) {
				var closed atomic.Int32
				old, next := retainedFixture("provider/old"), retainedFixture("provider/old")
				old.resource, old.stale = &closed, true
				next.reject = outcome == "attachment fails"
				f := &retentionFactoryFixture{initial: old, next: next, fail: outcome == "build fails"}
				m := NewRuntimeManager(f)
				if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", ModelRef: "provider/old"}); err != nil {
					t.Fatal(err)
				}
				epoch := m.ownerEpoch
				var err error
				if action == "settings" {
					_, err = m.RebuildSettings(context.Background(), "owned")
				} else {
					_, err = m.SetSessionEffort(context.Background(), "owned", "provider/old", "high")
				}
				if (err == nil) != (outcome == "success") || closed.Load() != 0 {
					t.Fatal("replacement outcome or resource ownership incorrect")
				}
				if outcome == "success" {
					if m.runtime != next || old.releases != 1 || m.ownerEpoch != epoch+1 || next.resource != &closed || next.commits != 1 {
						t.Fatal("replacement did not transfer and fence the new owner")
					}
				} else if m.runtime != old || old.releases != 0 || m.ownerEpoch != epoch || old.resource != &closed || next.commits != 0 {
					t.Fatal("failed candidate changed the previous owner")
				}
				if err := m.Shutdown(); err != nil || closed.Load() != 1 {
					t.Fatal("retained terminal resource cleanup incorrect")
				}
			})
		}
	}
}

func TestModelReplacementRejectsAnUnacknowledgedTerminalCreate(t *testing.T) {
	r := &terminalRuntimeFixture{fakeRuntime: &fakeRuntime{path: "/owned", state: "idle"}, block: make(chan struct{}), entered: make(chan struct{})}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", ModelRef: "provider/old"}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := m.CreateTerminal(context.Background(), "owned", ".", "sh"); finished <- err }()
	<-r.entered
	if _, err := m.SetSessionModel(context.Background(), "owned", "provider/new"); !errors.Is(err, ErrTerminalBusy) || r.shutdownCalls.Load() != 0 {
		t.Fatal("model replacement crossed pending create")
	}
	close(r.block)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if m.terminalCreates != 0 {
		t.Fatal("creation reservation leaked")
	}
	_ = m.Shutdown()
}

type terminalReplacementRuntime struct{ *terminalRuntimeFixture }

func (r *terminalReplacementRuntime) ModelSettingsState() (string, string, error) {
	return "old", "new", nil
}
func (r *terminalReplacementRuntime) ReleaseForReplacement() error { return nil }

type terminalReplacementFactory struct {
	initial Runtime
	next    SettingsRuntime
	builds  int
}

func (f *terminalReplacementFactory) Open(context.Context, OpenRequest) (Runtime, error) {
	return f.initial, nil
}
func (f *terminalReplacementFactory) Rebuild(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
	f.builds++
	return f.next, nil
}

func TestSettingsAndEffortRejectPendingCreatesAndFenceLateOutput(t *testing.T) {
	for _, action := range []string{"settings", "effort"} {
		for _, operation := range []string{"create", "output"} {
			t.Run(action+"/"+operation, func(t *testing.T) {
				r := &terminalReplacementRuntime{&terminalRuntimeFixture{fakeRuntime: &fakeRuntime{path: "/owned", state: "idle"}, block: make(chan struct{}), entered: make(chan struct{})}}
				f := &terminalReplacementFactory{initial: r, next: &settingsTestRuntime{fakeRuntime: &fakeRuntime{path: "/owned", state: "idle"}, applied: "new", desired: "new"}}
				m := NewRuntimeManager(f)
				if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", ModelRef: "provider/chat"}); err != nil {
					t.Fatal(err)
				}
				finished := make(chan error, 1)
				go func() {
					var err error
					if operation == "create" {
						_, err = m.CreateTerminal(context.Background(), "owned", ".", "sh")
					} else {
						_, err = m.TerminalOutput("owned", "id")
					}
					finished <- err
				}()
				<-r.entered
				var err error
				if action == "settings" {
					_, err = m.RebuildSettings(context.Background(), "owned")
				} else {
					_, err = m.SetSessionEffort(context.Background(), "owned", "provider/chat", "high")
				}
				if operation == "create" {
					if !errors.Is(err, ErrTerminalBusy) || f.builds != 0 {
						t.Fatal("pending terminal creation was crossed by a replacement build")
					}
				} else if err != nil || f.builds != 1 {
					t.Fatal("read-only terminal operation blocked replacement", err)
				}
				close(r.block)
				err = <-finished
				if operation == "create" && err != nil {
					t.Fatal("rejected replacement broke the admitted creation", err)
				}
				if operation == "output" && !errors.Is(err, ErrSessionNotFound) {
					t.Fatal("old-generation output escaped a settings/effort replacement", err)
				}
				_ = m.Shutdown()
			})
		}
	}
}
