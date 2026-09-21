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
	history       []HistoryMessage
	submits       []string
	cancelCalls   atomic.Int32
	shutdownCalls atomic.Int32
	shutdownErr   error
}

func (r *fakeRuntime) SessionPath() string { return r.path }
func (r *fakeRuntime) State() string       { return r.state }
func (r *fakeRuntime) History() []HistoryMessage {
	return append([]HistoryMessage(nil), r.history...)
}
func (r *fakeRuntime) AttachFile(path string) (AttachmentView, error) {
	return AttachmentView{Path: path, Name: "selected.txt", Size: 1}, nil
}
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

func TestRuntimeManagerHistoryReturnsBoundedNewestPage(t *testing.T) {
	messages := make([]HistoryMessage, 202)
	for i := range messages {
		messages[i] = HistoryMessage{Role: "user", Content: "message"}
	}
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle", history: messages}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	history, err := manager.History("a")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.Session.ID != "a" || history.TotalMessages != 202 || history.StartIndex != 2 || len(history.Messages) != maxHistoryMessages {
		t.Fatalf("history = %#v", history)
	}
	if _, err := manager.History("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
}

func TestRuntimeManagerHistoryReturnsEmptyArrayForEmptyTranscript(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}

	history, err := manager.History("a")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.Messages == nil || len(history.Messages) != 0 {
		t.Fatalf("empty history messages = %#v, want non-nil empty slice", history.Messages)
	}
}

func TestRuntimeManagerAttachesFileOnlyToOwnedSession(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}

	attachment, err := manager.AttachFile("a", "/tmp/selected.txt")
	if err != nil {
		t.Fatalf("attach file: %v", err)
	}
	if attachment.Path != "/tmp/selected.txt" || attachment.Name != "selected.txt" {
		t.Fatalf("attachment = %#v", attachment)
	}
	if _, err := manager.AttachFile("missing", "/tmp/selected.txt"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
	if _, err := manager.AttachFile("a", " "); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("blank path error = %v", err)
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

func TestRuntimeManagerSwitchesOnlyAfterClosingAnIdleSession(t *testing.T) {
	first := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	second := &fakeRuntime{path: "/sessions/b.jsonl", state: "idle"}
	opened := make([]string, 0, 2)
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		opened = append(opened, request.SessionID)
		switch request.SessionID {
		case "a":
			return first, nil
		case "b":
			return second, nil
		default:
			t.Fatalf("unexpected session %q", request.SessionID)
			return nil, nil
		}
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	view, err := manager.Switch(context.Background(), OpenRequest{SessionID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != "b" || first.shutdownCalls.Load() != 1 || second.shutdownCalls.Load() != 0 {
		t.Fatalf("switch result = %#v; shutdowns first=%d second=%d", view, first.shutdownCalls.Load(), second.shutdownCalls.Load())
	}
	if got, ok := manager.Snapshot(); !ok || got.ID != "b" {
		t.Fatalf("active snapshot = %#v, %t", got, ok)
	}
	if len(opened) != 2 || opened[0] != "a" || opened[1] != "b" {
		t.Fatalf("opened = %#v", opened)
	}
}

func TestRuntimeManagerRefusesToSwitchAnActiveSession(t *testing.T) {
	first := &fakeRuntime{path: "/sessions/a.jsonl", state: "running"}
	var opened atomic.Int32
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		opened.Add(1)
		return first, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Switch(context.Background(), OpenRequest{SessionID: "b"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("switch running session error = %v, want conflict", err)
	}
	if first.shutdownCalls.Load() != 0 || opened.Load() != 1 {
		t.Fatalf("running session was altered: shutdowns=%d opens=%d", first.shutdownCalls.Load(), opened.Load())
	}
	if got, ok := manager.Snapshot(); !ok || got.ID != "a" {
		t.Fatalf("active snapshot = %#v, %t", got, ok)
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

func TestRuntimeManagerRejectsUnsafeSessionIDBeforeOpeningCore(t *testing.T) {
	var opened atomic.Int32
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		opened.Add(1)
		return &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "../outside"}); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("unsafe session ID error = %v, want invalid session ID", err)
	}
	if got := opened.Load(); got != 0 {
		t.Fatalf("factory opens = %d, want 0", got)
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
