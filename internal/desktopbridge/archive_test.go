package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

type archiveRuntime struct {
	*fakeRuntime
	prepareErr error
	releases   int
}

func (r *archiveRuntime) PrepareArchive() error { return r.prepareErr }
func (r *archiveRuntime) ReleaseArchived()      { r.releases++ }

func TestArchiveCommitAndPreparationFailuresPreserveOwnedSession(t *testing.T) {
	r := &archiveRuntime{fakeRuntime: &fakeRuntime{state: "idle", path: "/session"}}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "first"}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("disk refused")
	if err := m.ChangeArchive("first", true, func(bool) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, ok := m.Snapshot(); !ok || r.releases != 0 {
		t.Fatal("commit refusal released owner")
	}
	r.prepareErr = failure
	called := false
	if err := m.ChangeArchive("first", true, func(bool) error { called = true; return nil }); !errors.Is(err, failure) || called {
		t.Fatal("preparation refusal committed")
	}
	r.prepareErr = nil
	r.state = "paused"
	if err := m.ChangeArchive("other", true, func(bool) error { called = true; return nil }); !errors.Is(err, ErrSessionConflict) || called {
		t.Fatal("paused owner admitted archive")
	}
	r.state = "idle"
	if err := m.ChangeArchive("other", true, func(owned bool) error {
		if owned {
			t.Fatal("inactive target marked owned")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if r.releases != 0 {
		t.Fatal("archiving another target released current session")
	}
	if err := m.ChangeArchive("first", true, func(owned bool) error {
		if !owned {
			t.Fatal("missing owner")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Snapshot(); ok || r.releases != 1 {
		t.Fatal("archive did not detach committed owner")
	}
}

type archiveAdmissionFactory struct{ runtime Runtime }

func (f archiveAdmissionFactory) Open(context.Context, OpenRequest) (Runtime, error) {
	return f.runtime, nil
}
func (f archiveAdmissionFactory) ValidateOpen(request OpenRequest) error {
	if request.SessionID == "archived" {
		return ErrSessionConflict
	}
	return nil
}

func TestArchivedSwitchRefusalPreservesCurrentOwner(t *testing.T) {
	r := &fakeRuntime{state: "idle", path: "/session"}
	m := NewRuntimeManager(archiveAdmissionFactory{r})
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Switch(context.Background(), OpenRequest{SessionID: "archived"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatal(err)
	}
	if view, ok := m.Snapshot(); !ok || view.ID != "first" || r.shutdownCalls.Load() != 0 {
		t.Fatal("refused archived switch destroyed current session")
	}
}

func TestArchiveCommitFencesConcurrentSubmission(t *testing.T) {
	r := &archiveRuntime{fakeRuntime: &fakeRuntime{state: "idle", path: "/session"}}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "first"}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	archived := make(chan error, 1)
	go func() {
		archived <- m.ChangeArchive("first", true, func(bool) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	submitted := make(chan error, 1)
	go func() {
		_, err := m.Submit(context.Background(), "first", "must not reach archived session")
		submitted <- err
	}()
	close(release)
	if err := <-archived; err != nil {
		t.Fatal(err)
	}
	if err := <-submitted; !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("submission after archive: %v", err)
	}
	if len(r.submits) != 0 || r.releases != 1 {
		t.Fatal("archived runtime admitted submission or retained owner")
	}
}
