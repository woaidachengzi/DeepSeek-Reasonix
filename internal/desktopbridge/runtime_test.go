package desktopbridge

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeRuntime struct {
	path          string
	title         string
	state         string
	deleted       bool
	history       []HistoryMessage
	submits       []string
	cancelCalls   atomic.Int32
	shutdownCalls atomic.Int32
	shutdownErr   error
}

type fakeMCPRuntime struct {
	*fakeRuntime
	servers []MCPRuntimeServer
	actions []string
}

type fakeMCPOAuthRuntime struct {
	*fakeRuntime
	flow MCPAuthFlow
}

type fakeMCPCredentialRuntime struct {
	*fakeRuntime
	cleared []string
}

func (r *fakeMCPCredentialRuntime) ClearMCPAuthentication(name string) (bool, error) {
	r.cleared = append(r.cleared, name)
	return true, nil
}

func (r *fakeMCPOAuthRuntime) StartMCPOAuth(name string) (string, error) {
	r.flow = MCPAuthFlow{ID: "0123456789abcdef", Name: name, Status: "pending"}
	return r.flow.ID, nil
}
func (r *fakeMCPOAuthRuntime) MCPOAuthStatus(id string) (MCPAuthFlow, error) {
	if id != r.flow.ID {
		return MCPAuthFlow{}, ErrSessionNotFound
	}
	return r.flow, nil
}
func (r *fakeMCPOAuthRuntime) CancelMCPOAuth(id string) error {
	if id != r.flow.ID {
		return ErrSessionNotFound
	}
	r.flow.Status = "canceled"
	return nil
}

func (r *fakeMCPRuntime) MCPRuntimeAction(name, action string) (int, error) {
	r.actions = append(r.actions, action+":"+name)
	return 3, nil
}

type fakeMetricsRuntime struct {
	*fakeRuntime
	metrics SessionMetrics
}

type fakeModelRuntime struct {
	*fakeRuntime
	model string
}

func (r *fakeModelRuntime) ModelRef() string { return r.model }

func (r *fakeMetricsRuntime) SessionMetrics() SessionMetrics { return r.metrics }

func TestSessionMetricsRequireMatchingActiveID(t *testing.T) {
	runtime := &fakeMetricsRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, metrics: SessionMetrics{ContextUsedTokens: 300, ContextWindowTokens: 1000}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, ok := manager.SessionMetrics("a"); ok {
		t.Fatal("metrics available before opening a session")
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.SessionMetrics("b"); ok {
		t.Fatal("metrics leaked to another session ID")
	}
	if metrics, ok := manager.SessionMetrics("a"); !ok || metrics.ContextUsedTokens != 300 {
		t.Fatalf("matching session metrics = %+v, available=%t", metrics, ok)
	}
}

func (r *fakeMCPRuntime) MCPRuntimeStatus() []MCPRuntimeServer { return r.servers }

func TestMCPStatusOnlyReadsMatchingActiveWorkspace(t *testing.T) {
	runtime := &fakeMCPRuntime{
		fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"},
		servers:     []MCPRuntimeServer{{Name: "time", Status: "connected", ToolCount: 1}},
	}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if servers, active := manager.MCPStatus("/work/a"); active || len(servers) != 0 {
		t.Fatalf("unopened runtime status = %+v, active=%t", servers, active)
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	if servers, active := manager.MCPStatus("/work/b"); active || len(servers) != 0 {
		t.Fatalf("other workspace status = %+v, active=%t", servers, active)
	}
	servers, active := manager.MCPStatus("/work/a")
	if !active || len(servers) != 1 || servers[0].Name != "time" {
		t.Fatalf("matching workspace status = %+v, active=%t", servers, active)
	}
}

func TestMCPRuntimeActionRequiresMatchingIdleSession(t *testing.T) {
	runtime := &fakeMCPRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.MCPRuntimeAction("a", "github", "connect"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("action before open error = %v, want not found", err)
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.MCPRuntimeAction("other", "github", "connect"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("wrong session action error = %v, want not found", err)
	}
	if count, err := manager.MCPRuntimeAction("a", "github", "connect"); err != nil || count != 3 {
		t.Fatalf("matching session action = count %d, error %v", count, err)
	}
	runtime.state = "running"
	if _, err := manager.MCPRuntimeAction("a", "github", "disconnect"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("running session action error = %v, want conflict", err)
	}
	if got := strings.Join(runtime.actions, ","); got != "connect:github" {
		t.Fatalf("runtime actions = %q, want only the accepted action", got)
	}
}

func TestMCPOAuthFlowIsBoundToTheMatchingIdleSession(t *testing.T) {
	runtime := &fakeMCPOAuthRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartMCPOAuth("other", "github"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("wrong session start error = %v, want not found", err)
	}
	runtime.state = "running"
	if _, err := manager.StartMCPOAuth("a", "github"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("running session start error = %v, want conflict", err)
	}
	runtime.state = "idle"
	flowID, err := manager.StartMCPOAuth("a", "github")
	if err != nil || flowID != runtime.flow.ID {
		t.Fatalf("matching session start = %q, error %v", flowID, err)
	}
	if _, err := manager.MCPOAuthStatus("other", flowID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("wrong session poll error = %v, want not found", err)
	}
	if err := manager.CancelMCPOAuth("a", flowID); err != nil {
		t.Fatal(err)
	}
	if flow, err := manager.MCPOAuthStatus("a", flowID); err != nil || flow.Status != "canceled" {
		t.Fatalf("matching session status = %+v, error %v", flow, err)
	}
}

func TestClearMCPAuthenticationRequiresMatchingIdleSession(t *testing.T) {
	runtime := &fakeMCPCredentialRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ClearMCPAuthentication("other", "github"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("wrong session clear error = %v, want not found", err)
	}
	runtime.state = "running"
	if _, err := manager.ClearMCPAuthentication("a", "github"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("running session clear error = %v, want conflict", err)
	}
	runtime.state = "idle"
	if changed, err := manager.ClearMCPAuthentication("a", "github"); err != nil || !changed {
		t.Fatalf("matching session clear = changed %t, error %v", changed, err)
	}
	if got := strings.Join(runtime.cleared, ","); got != "github" {
		t.Fatalf("clear calls = %q, want only the accepted action", got)
	}
}

func (r *fakeRuntime) SessionPath() string       { return r.path }
func (r *fakeRuntime) Title() string             { return r.title }
func (r *fakeRuntime) State() string             { return r.state }
func (r *fakeRuntime) Rename(title string) error { r.title = title; return nil }
func (r *fakeRuntime) Delete() error             { r.deleted = true; return nil }
func (r *fakeRuntime) History() []HistoryMessage {
	return append([]HistoryMessage(nil), r.history...)
}
func (r *fakeRuntime) AttachFile(path string) (AttachmentView, error) {
	return AttachmentView{Path: path, Name: "selected.txt", Size: 1}, nil
}
func (r *fakeRuntime) ListWorkspace(path string) (WorkspaceList, error) {
	return WorkspaceList{Path: path, Entries: []WorkspaceEntry{}}, nil
}
func (r *fakeRuntime) ReadWorkspaceFile(path string) (WorkspaceFilePreview, error) {
	return WorkspaceFilePreview{Path: path, Body: "preview", Size: 7}, nil
}
func (r *fakeRuntime) WorkspaceChanges() WorkspaceChanges {
	return WorkspaceChanges{Files: []WorkspaceChangeView{}, GitAvailable: true}
}
func (r *fakeRuntime) WorkspaceChangeDetail(string) (WorkspaceChangeDetail, error) {
	return WorkspaceChangeDetail{}, nil
}
func (r *fakeRuntime) Submit(input string)                                       { r.submits = append(r.submits, input) }
func (r *fakeRuntime) Cancel()                                                   { r.cancelCalls.Add(1) }
func (r *fakeRuntime) Approve(string, bool)                                      {}
func (r *fakeRuntime) AnswerQuestion(string, []AskAnswer) error                  { return nil }
func (r *fakeRuntime) AnswerMCPInteraction(string, string, map[string]any) error { return nil }
func (r *fakeRuntime) ReplayPendingPrompts()                                     {}
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

func TestRuntimeManagerSetSessionModelReopensSameSession(t *testing.T) {
	opened := []string{}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		opened = append(opened, request.ModelRef)
		return &fakeModelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle", title: "Conversation"}, model: request.ModelRef}, nil
	}))
	initial, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a", ModelRef: "provider/old"})
	if err != nil {
		t.Fatal(err)
	}
	if initial.ModelRef != "provider/old" {
		t.Fatalf("initial model = %q", initial.ModelRef)
	}

	updated, err := manager.SetSessionModel(context.Background(), "a", "provider/new")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "a" || updated.WorkspaceRoot != "/work/a" || updated.Path != initial.Path || updated.ModelRef != "provider/new" {
		t.Fatalf("updated session = %+v", updated)
	}
	if strings.Join(opened, ",") != "provider/old,provider/new" {
		t.Fatalf("factory opened models = %#v", opened)
	}
}

func TestRuntimeManagerDoesNotSwitchModelDuringActiveTurn(t *testing.T) {
	runtime := &fakeModelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "running"}, model: "provider/old"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", ModelRef: "provider/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionModel(context.Background(), "a", "provider/new"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("active-turn model switch error = %v", err)
	}
	if runtime.shutdownCalls.Load() != 0 {
		t.Fatalf("active runtime shut down %d times", runtime.shutdownCalls.Load())
	}
}

func TestRuntimeManagerSetSessionModelRestoresOldModelAfterFailure(t *testing.T) {
	opened := []string{}
	targetErr := errors.New("target build failed")
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		opened = append(opened, request.ModelRef)
		if request.ModelRef == "provider/new" {
			return nil, targetErr
		}
		return &fakeModelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, model: request.ModelRef}, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a", ModelRef: "provider/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionModel(context.Background(), "a", "provider/new"); !errors.Is(err, targetErr) || !errors.Is(err, ErrSessionModelSwitch) || !strings.Contains(err.Error(), "previous model restored") {
		t.Fatalf("model switch error = %v", err)
	}
	view, ok := manager.Snapshot()
	if !ok || view.ID != "a" || view.ModelRef != "provider/old" || view.State != "idle" {
		t.Fatalf("recovered snapshot = %+v, available=%t", view, ok)
	}
	if strings.Join(opened, ",") != "provider/old,provider/new,provider/old" {
		t.Fatalf("factory opened models = %#v", opened)
	}
}

func TestRuntimeManagerReportsSessionModelRecoveryFailure(t *testing.T) {
	opened := 0
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		opened++
		if opened > 1 {
			return nil, errors.New("injected rebuild failure")
		}
		return &fakeModelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, model: request.ModelRef}, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/work/a", ModelRef: "provider/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionModel(context.Background(), "a", "provider/new"); !errors.Is(err, ErrSessionModelRecover) {
		t.Fatalf("model switch recovery error = %v", err)
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("manager should not expose a runtime after both the target and restoration fail")
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

func TestRuntimeManagerRenamesOnlyIdleOwnedSession(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	view, err := manager.RenameSession("a", "  Release notes  ")
	if err != nil || view.Title != "Release notes" || runtime.title != "Release notes" {
		t.Fatalf("rename = %#v, %v; runtime title = %q", view, err, runtime.title)
	}
	if _, err := manager.RenameSession("missing", "Other"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
	for _, title := range []string{"", "  ", "has\ncontrol", strings.Repeat("x", 121)} {
		if _, err := manager.RenameSession("a", title); !errors.Is(err, ErrInvalidTitle) {
			t.Fatalf("invalid title %q error = %v", title, err)
		}
	}
	runtime.state = "running"
	if _, err := manager.RenameSession("a", "Too soon"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("running session error = %v", err)
	}
}

func TestRuntimeManagerDeletesOnlyTheIdleOwnedSession(t *testing.T) {
	runtime := &fakeRuntime{path: "/sessions/a.jsonl", state: "running"}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return runtime, nil
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteSession("a"); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("running session delete error = %v", err)
	}
	if err := manager.DeleteSession("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unowned session delete error = %v", err)
	}
	if runtime.deleted {
		t.Fatal("an unowned or running session was deleted")
	}

	runtime.state = "idle"
	if err := manager.DeleteSession("a"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if !runtime.deleted {
		t.Fatal("delete did not reach the core runtime")
	}
	// The manager must release the deleted session: a second delete reports the
	// session as gone instead of sweeping files a second time.
	if err := manager.DeleteSession("a"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("repeat delete error = %v", err)
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("the manager still reports a snapshot after the session was deleted")
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

func TestRuntimeManagerRestoresPreviousSessionWhenSwitchTargetFails(t *testing.T) {
	first := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	recovered := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	targetErr := errors.New("target unavailable")
	var opens atomic.Int32
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(ctx context.Context, request OpenRequest) (Runtime, error) {
		switch opens.Add(1) {
		case 1:
			return first, nil
		case 2:
			if request.SessionID != "b" {
				t.Fatalf("target request = %#v", request)
			}
			return nil, targetErr
		case 3:
			if request.SessionID != "a" || request.WorkspaceRoot != "/workspace" || ctx.Err() != nil {
				t.Fatalf("recovery request = %#v, context error = %v", request, ctx.Err())
			}
			return recovered, nil
		default:
			t.Fatal("unexpected extra open")
			return nil, nil
		}
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a", WorkspaceRoot: "/workspace"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Switch(ctx, OpenRequest{SessionID: "b"}); !errors.Is(err, targetErr) {
		t.Fatalf("switch error = %v, want target failure", err)
	}
	if first.shutdownCalls.Load() != 1 || recovered.shutdownCalls.Load() != 0 {
		t.Fatalf("shutdowns first=%d recovered=%d", first.shutdownCalls.Load(), recovered.shutdownCalls.Load())
	}
	if view, ok := manager.Snapshot(); !ok || view.ID != "a" || view.WorkspaceRoot != "/workspace" {
		t.Fatalf("restored snapshot = %#v, %t", view, ok)
	}
}

func TestRuntimeManagerReportsBothFailuresWhenSwitchRecoveryFails(t *testing.T) {
	first := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	targetErr := errors.New("target unavailable")
	recoveryErr := errors.New("previous unavailable")
	var opens atomic.Int32
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, _ OpenRequest) (Runtime, error) {
		switch opens.Add(1) {
		case 1:
			return first, nil
		case 2:
			return nil, targetErr
		default:
			return nil, recoveryErr
		}
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Switch(context.Background(), OpenRequest{SessionID: "b"})
	if !errors.Is(err, targetErr) || !errors.Is(err, recoveryErr) {
		t.Fatalf("switch error = %v, want both failures", err)
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("unrestored session was published")
	}
}

func TestRuntimeManagerDoesNotPublishRecoveredSessionAfterShutdown(t *testing.T) {
	first := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	recovered := &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}
	recoveryStarted := make(chan struct{})
	releaseRecovery := make(chan struct{})
	var opens atomic.Int32
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, _ OpenRequest) (Runtime, error) {
		switch opens.Add(1) {
		case 1:
			return first, nil
		case 2:
			return nil, errors.New("target unavailable")
		default:
			close(recoveryStarted)
			<-releaseRecovery
			return recovered, nil
		}
	}))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := manager.Switch(context.Background(), OpenRequest{SessionID: "b"})
		result <- err
	}()
	<-recoveryStarted
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "c"}); !errors.Is(err, ErrOpenInProgress) {
		t.Fatalf("concurrent open error = %v, want open in progress", err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	close(releaseRecovery)
	if err := <-result; !errors.Is(err, ErrClosed) {
		t.Fatalf("switch error = %v, want closed", err)
	}
	if recovered.shutdownCalls.Load() != 1 {
		t.Fatalf("recovered runtime shutdowns = %d, want 1", recovered.shutdownCalls.Load())
	}
	if _, ok := manager.Snapshot(); ok {
		t.Fatal("recovered session survived manager shutdown")
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
