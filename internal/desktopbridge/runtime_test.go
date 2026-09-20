package desktopbridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type fakeRuntime struct {
	path          string
	state         string
	submits       []string
	cancelCalls   atomic.Int32
	shutdownCalls atomic.Int32
	shutdownErr   error
}

func (r *fakeRuntime) SessionPath() string { return r.path }
func (r *fakeRuntime) State() string       { return r.state }
func (r *fakeRuntime) Submit(input string) { r.submits = append(r.submits, input) }
func (r *fakeRuntime) Cancel()             { r.cancelCalls.Add(1) }
func (r *fakeRuntime) Shutdown() error {
	r.shutdownCalls.Add(1)
	return r.shutdownErr
}

func TestRuntimeManagerSubmitsAndCancelsOwnedSession(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit("a", "hello"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(runtime.submits) != 1 || runtime.submits[0] != "hello" {
		t.Fatalf("submits = %#v", runtime.submits)
	}
	if _, err := manager.Cancel("a"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if runtime.cancelCalls.Load() != 1 {
		t.Fatalf("cancel calls = %d", runtime.cancelCalls.Load())
	}
	if _, err := manager.Submit("missing", "hello"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
	if _, err := manager.Submit("a", "   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank input error = %v", err)
	}
}

func TestRuntimeManagerOwnsOneSessionAndShutsDownOnce(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	first, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/workspace"})
	if err != nil || second != first {
		t.Fatalf("second open = %#v, %v; want %#v, nil", second, err, first)
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "b", WorkspaceRoot: "/workspace"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("different session error = %v, want conflict", err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if got := runtime.shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}

func TestRuntimeManagerDoesNotPublishFailedOrNilRuntime(t *testing.T) {
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return nil, errors.New("build failed")
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err == nil {
		t.Fatal("open succeeded")
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("failed runtime was published")
	}
}

func TestRuntimeManagerClosesRuntimeBuiltDuringConcurrentShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		close(started)
		<-release
		return runtime, nil
	}))
	errCh := make(chan error, 1)
	go func() {
		_, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"})
		errCh <- err
	}()
	<-started
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; !errors.Is(err, ErrClosed) {
		t.Fatalf("open error = %v, want closed", err)
	}
	if got := runtime.shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}
