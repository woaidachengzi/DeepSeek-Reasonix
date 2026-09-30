package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

type workspaceRuntime struct {
	*fakeRuntime
	root string
	err  error
}

func (r *workspaceRuntime) LocalWorkspace() (string, error) { return r.root, r.err }

func TestWorkspaceTargetUsesLiveProviderAndFencesUnownedClosedOrUnsupportedSessions(t *testing.T) {
	runtime := &workspaceRuntime{fakeRuntime: &fakeRuntime{path: "/session", state: "idle"}, root: "/actual/global"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "global"}); err != nil {
		t.Fatal(err)
	}
	target, err := manager.WorkspaceTarget("global")
	if err != nil || target.SessionID != "global" || target.WorkspaceRoot != runtime.root {
		t.Fatalf("target %+v %v", target, err)
	}
	if _, err := manager.WorkspaceTarget("other"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unowned error %v", err)
	}
	runtime.root = ""
	if _, err := manager.WorkspaceTarget("global"); !errors.Is(err, ErrInvalidWorkspacePath) {
		t.Fatalf("empty target error %v", err)
	}
	runtime.err = errors.New("unavailable")
	if _, err := manager.WorkspaceTarget("global"); !errors.Is(err, runtime.err) {
		t.Fatalf("provider error %v", err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.WorkspaceTarget("global"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed error %v", err)
	}
	unsupported := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return &fakeRuntime{path: "/session", state: "idle"}, nil
	}))
	if _, err := unsupported.Open(context.Background(), OpenRequest{SessionID: "unsupported", WorkspaceRoot: "/client/root"}); err != nil {
		t.Fatal(err)
	}
	defer unsupported.Shutdown()
	if _, err := unsupported.WorkspaceTarget("unsupported"); !errors.Is(err, ErrInvalidWorkspacePath) {
		t.Fatal("unsupported provider fell back to client path")
	}
}
