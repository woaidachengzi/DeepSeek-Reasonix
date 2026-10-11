package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

type fakePinnedRuntime struct {
	*fakeRuntime
	calls int
}

func (r *fakePinnedRuntime) PinnedFiles() ([]PinnedFile, error) {
	r.calls++
	return []PinnedFile{}, nil
}

func (r *fakePinnedRuntime) ChangePinnedFile(path string, pin bool) ([]PinnedFile, error) {
	r.calls++
	return []PinnedFile{{Path: path}}, nil
}

func TestPinnedFilesRequireOwnershipAndIdleMutation(t *testing.T) {
	runtime := &fakePinnedRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.PinnedFiles("a"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ChangePinnedFile("b", "notes.txt", true); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
	for _, state := range []string{"running", "paused", "deleted", "deleting"} {
		runtime.state = state
		if _, err := manager.ChangePinnedFile("a", "notes.txt", true); !errors.Is(err, ErrSessionConflict) {
			t.Fatalf("%s: %v", state, err)
		}
	}
	if runtime.calls != 0 {
		t.Fatal("rejected mutations reached runtime")
	}
	runtime.state = "idle"
	if files, err := manager.PinnedFiles("a"); err != nil || files == nil {
		t.Fatalf("empty list: %+v %v", files, err)
	}
	if files, err := manager.ChangePinnedFile("a", "notes.txt", true); err != nil || len(files) != 1 {
		t.Fatalf("pin: %+v %v", files, err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PinnedFiles("a"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
