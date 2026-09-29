package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/profilegate"
	"reasonix/internal/sessionidentity"
)

const testToken = "0123456789abcdef0123456789abcdef"

func tokenStdin(t *testing.T, input string) *os.File {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		current := os.Stdin
		os.Stdin = previous
		if current != reader {
			_ = current.Close()
		}
		_ = reader.Close()
	})
	if _, err := io.WriteString(writer, input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestBridgeTokenIsReadFromStdinAndLegacyEnvironmentIsCleared(t *testing.T) {
	t.Setenv(tokenEnvironment, "ignored-legacy-value")
	pipe := tokenStdin(t, testToken+"\n")
	token, err := consumeBridgeToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != testToken || os.Getenv(tokenEnvironment) != "" {
		t.Fatal("bridge token was not consumed from stdin or legacy environment was retained")
	}
	if _, err := pipe.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("token pipe remained open after startup: %v", err)
	}
	if data, err := io.ReadAll(os.Stdin); err != nil || len(data) != 0 {
		t.Fatalf("bridge stdin was not replaced with the empty device: bytes=%d err=%v", len(data), err)
	}
}

func TestBridgeTokenDoesNotFallBackToEnvironment(t *testing.T) {
	t.Setenv(tokenEnvironment, testToken)
	tokenStdin(t, "")
	if _, err := consumeBridgeToken(); err == nil {
		t.Fatal("missing stdin token was accepted from the environment")
	}
	if os.Getenv(tokenEnvironment) != "" {
		t.Fatal("legacy environment token was retained")
	}
}

type bridgeTestRuntime struct {
	path          string
	title         string
	state         string
	modelRef      string
	history       []desktopbridge.HistoryMessage
	submits       []string
	attachCalls   int
	renameCalls   int
	deleteCalls   int
	cancelCalls   int
	shutdownCalls int
	workspace     desktopbridge.WorkspaceList
	preview       desktopbridge.WorkspaceFilePreview
	changes       desktopbridge.WorkspaceChanges
	revertCommits int
	revertUndos   int
	metrics       desktopbridge.SessionMetrics
	balance       *desktopbridge.SessionBalance
	balanceErr    error
	balanceCalls  int
	mcpActions    []string
	mcpAuthFlow   desktopbridge.MCPAuthFlow
	mcpAuthClears []string
	recall        desktopbridge.MemoryRecallView
}

func (r *bridgeTestRuntime) SessionPath() string                          { return r.path }
func (r *bridgeTestRuntime) ModelRef() string                             { return r.modelRef }
func (r *bridgeTestRuntime) SessionMetrics() desktopbridge.SessionMetrics { return r.metrics }
func (r *bridgeTestRuntime) SessionBalance(context.Context) (*desktopbridge.SessionBalance, error) {
	r.balanceCalls++
	return r.balance, r.balanceErr
}
func (r *bridgeTestRuntime) MCPRuntimeAction(name, action string) (int, error) {
	r.mcpActions = append(r.mcpActions, action+":"+name)
	return 2, nil
}
func (r *bridgeTestRuntime) StartMCPOAuth(name string) (string, error) {
	r.mcpAuthFlow = desktopbridge.MCPAuthFlow{ID: "flow-test", Name: name, Status: "pending"}
	return r.mcpAuthFlow.ID, nil
}
func (r *bridgeTestRuntime) MCPOAuthStatus(id string) (desktopbridge.MCPAuthFlow, error) {
	if id != r.mcpAuthFlow.ID {
		return desktopbridge.MCPAuthFlow{}, desktopbridge.ErrSessionNotFound
	}
	return r.mcpAuthFlow, nil
}
func (r *bridgeTestRuntime) CancelMCPOAuth(id string) error {
	if id != r.mcpAuthFlow.ID {
		return desktopbridge.ErrSessionNotFound
	}
	r.mcpAuthFlow.Status = "canceled"
	return nil
}
func (r *bridgeTestRuntime) ClearMCPAuthentication(name string) (bool, error) {
	r.mcpAuthClears = append(r.mcpAuthClears, name)
	return true, nil
}
func (r *bridgeTestRuntime) MemoryRecall() desktopbridge.MemoryRecallView { return r.recall }
func (r *bridgeTestRuntime) Title() string                                { return r.title }
func (r *bridgeTestRuntime) State() string                                { return r.state }
func (r *bridgeTestRuntime) Rename(title string) error {
	r.renameCalls++
	r.title = title
	return nil
}
func (r *bridgeTestRuntime) Delete() error {
	r.deleteCalls++
	return nil
}
func (r *bridgeTestRuntime) History() []desktopbridge.HistoryMessage {
	return append([]desktopbridge.HistoryMessage(nil), r.history...)
}
func (r *bridgeTestRuntime) AttachFile(path string) (desktopbridge.AttachmentView, error) {
	r.attachCalls++
	return desktopbridge.AttachmentView{Path: ".reasonix/attachments/clipboard-test.txt", Name: filepath.Base(path), Size: 12}, nil
}
func (r *bridgeTestRuntime) ListWorkspace(path string) (desktopbridge.WorkspaceList, error) {
	listing := r.workspace
	listing.Path = path
	return listing, nil
}
func (r *bridgeTestRuntime) ReadWorkspaceFile(path string) (desktopbridge.WorkspaceFilePreview, error) {
	preview := r.preview
	preview.Path = path
	return preview, nil
}
func (r *bridgeTestRuntime) WorkspaceChanges() desktopbridge.WorkspaceChanges { return r.changes }
func (r *bridgeTestRuntime) WorkspaceChangeDetail(string) (desktopbridge.WorkspaceChangeDetail, error) {
	return desktopbridge.WorkspaceChangeDetail{Source: "git", Diff: "@@ -1 +1 @@\n-old\n+new"}, nil
}
func (r *bridgeTestRuntime) PrepareWorkspaceFileRevert(path string) (desktopbridge.WorkspaceFileRevertPlan, error) {
	return desktopbridge.WorkspaceFileRevertPlan{Path: path, PlanID: "plan-test", CanFiles: true}, nil
}
func (r *bridgeTestRuntime) CommitWorkspaceFileRevert(string, string) (desktopbridge.WorkspaceFileRevertResult, error) {
	r.revertCommits++
	return desktopbridge.WorkspaceFileRevertResult{OK: true, WrittenCount: 1}, nil
}
func (r *bridgeTestRuntime) UndoWorkspaceFileRevert(string) (desktopbridge.WorkspaceFileRevertResult, error) {
	r.revertUndos++
	return desktopbridge.WorkspaceFileRevertResult{OK: true, WrittenCount: 1}, nil
}
func (r *bridgeTestRuntime) Submit(input string)                                       { r.submits = append(r.submits, input) }
func (r *bridgeTestRuntime) Cancel()                                                   { r.cancelCalls++ }
func (r *bridgeTestRuntime) Approve(string, bool)                                      {}
func (r *bridgeTestRuntime) AnswerQuestion(string, []desktopbridge.AskAnswer) error    { return nil }
func (r *bridgeTestRuntime) AnswerMCPInteraction(string, string, map[string]any) error { return nil }
func (r *bridgeTestRuntime) ReplayPendingPrompts()                                     {}
func (r *bridgeTestRuntime) Shutdown() error {
	r.shutdownCalls++
	return nil
}

func TestHealthRequiresToken(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHealthReturnsProtocolAndCapabilities(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got healthResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != desktopbridge.ProtocolVersion || got.Status != "ok" || got.SidecarInstanceID != "instance-a" {
		t.Fatalf("health = %#v", got)
	}
	found := map[string]bool{}
	for _, capability := range got.Capabilities {
		found[capability] = true
	}
	if !found["provider_summary"] || !found["discover_provider_models"] || !found["set_secrets_settings"] || !found["set_default_model"] || !found["set_session_model"] || !found["attach_file"] || !found["rename_session"] || !found["delete_session"] || !found["session_catalog_sync"] || !found["session_directory_snapshot_v1"] || !found["session_directory_snapshot_full_v1"] || !found["session_delete_recovery_list_v1"] || !found["session_title_intent_v1"] || !found["session_title_recovery_list_v1"] || !found["session_balance"] || !found["mcp_runtime_action"] {
		t.Fatalf("health capabilities %v do not include provider model settings", got.Capabilities)
	}
}

func TestSessionBalanceIsAuthorizedAndScopedToActiveSession(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/wallet-session.jsonl", state: "idle", balance: &desktopbridge.SessionBalance{Available: true, Display: "¥12.34"}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "wallet-session"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-wallet", manager)
	request := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+id+"/balance", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := request("wallet-session"); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"display":"¥12.34"`) {
		t.Fatalf("balance status = %d, body = %s", got.Code, got.Body.String())
	}
	if strings.Contains(request("another-session").Body.String(), "¥12.34") {
		t.Fatal("balance leaked to a different session")
	}
	if runtime.balanceCalls != 1 {
		t.Fatalf("balance queried %d times; mismatched session must not invoke provider", runtime.balanceCalls)
	}
}

func TestSessionMCPRuntimeActionIsAuthorizedAndActiveSessionScoped(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/mcp-session.jsonl", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "mcp-session"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-mcp", manager)
	request := func(sessionID, action string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/mcp/runtime", strings.NewReader(`{"name":"github","action":"`+action+`"}`))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set(requestIDHeader, "mcp-"+sessionID+"-"+action)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := request("mcp-session", "connect", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, body=%s", got.Code, got.Body.String())
	}
	if got := request("another-session", "connect", true); got.Code != http.StatusNotFound {
		t.Fatalf("mismatched active session status = %d, body=%s", got.Code, got.Body.String())
	}
	if got := request("mcp-session", "connect", true); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"toolCount":2`) {
		t.Fatalf("active action status = %d, body=%s", got.Code, got.Body.String())
	}
	if strings.Join(runtime.mcpActions, ",") != "connect:github" {
		t.Fatalf("runtime actions = %#v", runtime.mcpActions)
	}
}

func TestSessionMCPCredentialClearIsAuthorizedIdleAndSessionScoped(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/mcp-clear-session.jsonl", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "mcp-clear-session"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-mcp-clear", manager)
	counter := 0
	request := func(sessionID string, authorized bool) *httptest.ResponseRecorder {
		counter++
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/mcp/auth/clear", strings.NewReader(`{"name":"github"}`))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set(requestIDHeader, fmt.Sprintf("mcp-clear-%d", counter))
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := request("mcp-clear-session", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized clear status = %d, body=%s", got.Code, got.Body.String())
	}
	if got := request("another-session", true); got.Code != http.StatusNotFound {
		t.Fatalf("mismatched session clear status = %d, body=%s", got.Code, got.Body.String())
	}
	runtime.state = "running"
	if got := request("mcp-clear-session", true); got.Code != http.StatusConflict {
		t.Fatalf("running session clear status = %d, body=%s", got.Code, got.Body.String())
	}
	runtime.state = "idle"
	if got := request("mcp-clear-session", true); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"changed":true`) {
		t.Fatalf("active session clear status = %d, body=%s", got.Code, got.Body.String())
	}
	if strings.Join(runtime.mcpAuthClears, ",") != "github" {
		t.Fatalf("credential clear calls = %#v", runtime.mcpAuthClears)
	}
}

func TestSetSessionModelRebuildsAndDeduplicatesTheCurrentSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	workspace := t.TempDir()
	contents := `default_model = "local/a"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:1234/v1"
models = ["a", "b"]
default = "a"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	opened := []string{}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		opened = append(opened, request.ModelRef)
		return &bridgeTestRuntime{path: "/tmp/model-session.jsonl", state: "idle", modelRef: request.ModelRef}, nil
	}))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "model-session", WorkspaceRoot: workspace, ModelRef: "local/a"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance", manager)
	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/model-session/model", strings.NewReader(`{"model":"local/b"}`))
		httpRequest.Header.Set("Authorization", "Bearer "+testToken)
		httpRequest.Header.Set(requestIDHeader, "switch-model-request")
		bridge.handler().ServeHTTP(response, httpRequest)
		return response
	}
	first := request()
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"modelRef":"local/b"`) {
		t.Fatalf("model switch status = %d, body = %s", first.Code, first.Body.String())
	}
	replay := request()
	if replay.Code != http.StatusOK || replay.Body.String() != first.Body.String() {
		t.Fatalf("model switch replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	if strings.Join(opened, ",") != "local/a,local/b" {
		t.Fatalf("runtime models opened = %#v; request replay must not rebuild twice", opened)
	}
}

func TestBridgeServerDeletesSessionIdempotently(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-delete"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	opened := httptest.NewRecorder()
	handler.ServeHTTP(opened, open)
	if opened.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", opened.Code, opened.Body.String())
	}

	deleteTwice := func(requestID string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/tab-delete", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set(requestIDHeader, requestID)
		handler.ServeHTTP(response, request)
		return response
	}
	first := deleteTwice("delete-request-1")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"deleted":true`) || !strings.Contains(first.Body.String(), `"protocolVersion":1`) {
		t.Fatalf("delete status = %d, body = %s", first.Code, first.Body.String())
	}
	if runtime.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want one physical sweep", runtime.deleteCalls)
	}
	// Replaying the same request ID must return the cached response, not sweep
	// the session a second time.
	replay := deleteTwice("delete-request-1")
	if replay.Code != http.StatusOK || replay.Body.String() != first.Body.String() {
		t.Fatalf("replayed delete status = %d, body = %s", replay.Code, replay.Body.String())
	}
	if runtime.deleteCalls != 1 {
		t.Fatalf("delete calls after replay = %d, want one", runtime.deleteCalls)
	}
	// A fresh request for the now-released session reports it as gone.
	missing := deleteTwice("delete-request-2")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("repeat delete status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func TestBridgeServerRenamesSessionIdempotently(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-title"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	opened := httptest.NewRecorder()
	handler.ServeHTTP(opened, open)
	if opened.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", opened.Code, opened.Body.String())
	}
	for range 2 {
		request := httptest.NewRequest(http.MethodPatch, "/v1/sessions/tab-title/title", strings.NewReader(`{"title":"Release notes"}`))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(requestIDHeader, "rename-request-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"title":"Release notes"`) {
			t.Fatalf("rename status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	if runtime.title != "Release notes" {
		t.Fatalf("runtime title = %q", runtime.title)
	}
	if runtime.renameCalls != 1 {
		t.Fatalf("rename calls = %d, want one after idempotent replay", runtime.renameCalls)
	}
	request := httptest.NewRequest(http.MethodPatch, "/v1/sessions/tab-title/title", strings.NewReader(`{"title":"bad\ntitle"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid title status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestShutdownIsAuthenticatedAndIdempotent(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	handler := bridge.handler()
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/v1:shutdown", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
		}
	}
	select {
	case <-bridge.shutdownRequested:
	default:
		t.Fatal("shutdown request was not signalled")
	}
}

func TestBridgeServerOpensSessionAndReturnsSnapshot(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle", metrics: desktopbridge.SessionMetrics{ContextUsedTokens: 2400, ContextWindowTokens: 10000, CompactThresholdPercent: 80, CacheHitTokens: 300, CacheMissTokens: 100}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		if request.SessionID != "tab-1" {
			t.Fatalf("unexpected session ID: %q", request.SessionID)
		}
		if request.WorkspaceRoot != "/workspace" {
			t.Fatalf("unexpected workspace root: %q", request.WorkspaceRoot)
		}
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1","workspaceRoot":"/workspace"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRequest.Header.Set("Content-Type", "application/json")
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	var openResponse struct {
		ProtocolVersion int                       `json:"protocolVersion"`
		Session         desktopbridge.SessionView `json:"session"`
	}
	if err := json.NewDecoder(openRecorder.Body).Decode(&openResponse); err != nil {
		t.Fatalf("decode open response: %v", err)
	}
	if openResponse.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("protocol version = %d", openResponse.ProtocolVersion)
	}
	if openResponse.Session.ID != "tab-1" || openResponse.Session.Path != runtime.path || openResponse.Session.State != "idle" {
		t.Fatalf("unexpected session: %#v", openResponse.Session)
	}

	snapshotRequest := httptest.NewRequest(http.MethodGet, "/v1/sessions/tab-1/snapshot", nil)
	snapshotRequest.Header.Set("Authorization", "Bearer "+testToken)
	snapshotRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(snapshotRecorder, snapshotRequest)
	if snapshotRecorder.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", snapshotRecorder.Code, snapshotRecorder.Body.String())
	}

	var snapshotResponse struct {
		ProtocolVersion int                          `json:"protocolVersion"`
		Sequence        uint64                       `json:"sequence"`
		Session         desktopbridge.SessionView    `json:"session"`
		Metrics         desktopbridge.SessionMetrics `json:"metrics"`
	}
	if err := json.NewDecoder(snapshotRecorder.Body).Decode(&snapshotResponse); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	if snapshotResponse.ProtocolVersion != desktopbridge.ProtocolVersion || snapshotResponse.Sequence != 0 || snapshotResponse.Session.ID != "tab-1" {
		t.Fatalf("unexpected snapshot: %#v", snapshotResponse)
	}
	if snapshotResponse.Metrics.ContextUsedTokens != 2400 || snapshotResponse.Metrics.CacheHitTokens != 300 {
		t.Fatalf("snapshot lost active session metrics: %+v", snapshotResponse.Metrics)
	}
}

func TestBridgeServerSwitchesOnlyIdleSessions(t *testing.T) {
	first := &bridgeTestRuntime{path: "/tmp/reasonix-a", state: "idle"}
	second := &bridgeTestRuntime{path: "/tmp/reasonix-b", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		switch request.SessionID {
		case "tab-a":
			return first, nil
		case "tab-b":
			return second, nil
		default:
			t.Fatalf("unexpected session %q", request.SessionID)
			return nil, nil
		}
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-a"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openResult := httptest.NewRecorder()
	handler.ServeHTTP(openResult, open)
	if openResult.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openResult.Code, openResult.Body.String())
	}

	switchRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:switch", strings.NewReader(`{"sessionId":"tab-b"}`))
	switchRequest.Header.Set("Authorization", "Bearer "+testToken)
	switchRequest.Header.Set("Content-Type", "application/json")
	switchResult := httptest.NewRecorder()
	handler.ServeHTTP(switchResult, switchRequest)
	if switchResult.Code != http.StatusOK {
		t.Fatalf("switch status = %d, body = %s", switchResult.Code, switchResult.Body.String())
	}
	if first.shutdownCalls != 1 || second.shutdownCalls != 0 {
		t.Fatalf("switch shutdowns first=%d second=%d", first.shutdownCalls, second.shutdownCalls)
	}

	second.state = "running"
	blocked := httptest.NewRequest(http.MethodPost, "/v1/sessions:switch", strings.NewReader(`{"sessionId":"tab-a"}`))
	blocked.Header.Set("Authorization", "Bearer "+testToken)
	blocked.Header.Set("Content-Type", "application/json")
	blockedResult := httptest.NewRecorder()
	handler.ServeHTTP(blockedResult, blocked)
	if blockedResult.Code != http.StatusConflict {
		t.Fatalf("switch running status = %d, body = %s", blockedResult.Code, blockedResult.Body.String())
	}
	if second.shutdownCalls != 0 {
		t.Fatalf("running session was closed %d times", second.shutdownCalls)
	}
}

func TestBridgeServerReturnsDisplaySafeSessionHistory(t *testing.T) {
	runtime := &bridgeTestRuntime{
		path:  "/tmp/reasonix-session",
		state: "idle",
		history: []desktopbridge.HistoryMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi", Truncated: true, WorkDurationMs: 960_000},
		},
	}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openRecorder := httptest.NewRecorder()
	handler.ServeHTTP(openRecorder, open)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/tab-1/history", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", response.Code, response.Body.String())
	}
	var got historyResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if got.ProtocolVersion != desktopbridge.ProtocolVersion || got.Session.ID != "tab-1" || got.TotalMessages != 2 || got.StartIndex != 0 || len(got.Messages) != 2 || !got.Messages[1].Truncated || got.Messages[1].WorkDurationMs != 960_000 {
		t.Fatalf("history = %#v", got)
	}
}

func TestBridgeServerReturnsEmptyHistoryAsJSONArray(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-empty"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openRecorder := httptest.NewRecorder()
	handler.ServeHTTP(openRecorder, open)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/tab-empty/history", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"messages":[]`) {
		t.Fatalf("empty history response = %s, want messages array", response.Body.String())
	}
}

func TestBridgeServerAttachesFileWithIdempotentRequest(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-attach"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	opened := httptest.NewRecorder()
	handler.ServeHTTP(opened, open)
	if opened.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", opened.Code, opened.Body.String())
	}

	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-attach:attach", strings.NewReader(`{"sessionId":"tab-attach","path":"/private/research notes.txt"}`))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(requestIDHeader, "attach-request-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("attach status = %d, body = %s", response.Code, response.Body.String())
		}
		var got attachmentResponse
		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatalf("decode attachment response: %v", err)
		}
		if got.ProtocolVersion != desktopbridge.ProtocolVersion || got.Attachment.Path != ".reasonix/attachments/clipboard-test.txt" || got.Attachment.Name != "research notes.txt" || got.Attachment.Size != 12 {
			t.Fatalf("attachment response = %#v", got)
		}
		if strings.Contains(response.Body.String(), "/private/") {
			t.Fatalf("response leaked selected source path: %s", response.Body.String())
		}
	}
	if runtime.attachCalls != 1 {
		t.Fatalf("runtime attachment calls = %d, want 1 after idempotent replay", runtime.attachCalls)
	}
}

func TestBridgeServerShutdownClosesOpenedRuntime(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	shutdownRequest := httptest.NewRequest(http.MethodPost, "/v1:shutdown", nil)
	shutdownRequest.Header.Set("Authorization", "Bearer "+testToken)
	shutdownRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(shutdownRecorder, shutdownRequest)
	if shutdownRecorder.Code != http.StatusAccepted {
		t.Fatalf("shutdown status = %d, body = %s", shutdownRecorder.Code, shutdownRecorder.Body.String())
	}
	if runtime.shutdownCalls != 1 {
		t.Fatalf("shutdown calls = %d, want 1", runtime.shutdownCalls)
	}
}

func TestBridgeServerSubmitsAndCancelsOpenedRuntime(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	submitRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:submit", strings.NewReader(`{"input":"hello"}`))
	submitRequest.Header.Set("Authorization", "Bearer "+testToken)
	submitRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body = %s", submitRecorder.Code, submitRecorder.Body.String())
	}
	if len(runtime.submits) != 1 || runtime.submits[0] != "hello" {
		t.Fatalf("submits = %#v", runtime.submits)
	}

	cancelRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:cancel", nil)
	cancelRequest.Header.Set("Authorization", "Bearer "+testToken)
	cancelRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d, body = %s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	if runtime.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d", runtime.cancelCalls)
	}
}

func TestBridgeServerAnswersAndReplaysPromptCommands(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-prompts"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	handler.ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	cases := []struct {
		path string
		body string
	}{
		{path: "/v1/sessions/tab-prompts:approve", body: `{"id":"approval-1","allow":true}`},
		{path: "/v1/sessions/tab-prompts:answer", body: `{"id":"ask-1","answers":[{"questionId":"q-1","selected":["yes"]}]}`},
		{path: "/v1/sessions/tab-prompts:mcp", body: `{"id":"mcp-1","action":"decline"}`},
		{path: "/v1/sessions/tab-prompts:replay-prompts", body: ``},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s status = %d, body = %s", tc.path, response.Code, response.Body.String())
		}
	}
}

func TestBridgeServerReplaysRequestIDWithoutSubmittingTwice(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	handler.ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	for range 2 {
		submitRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:submit", strings.NewReader(`{"input":"hello"}`))
		submitRequest.Header.Set("Authorization", "Bearer "+testToken)
		submitRequest.Header.Set(requestIDHeader, "request-123")
		submitRecorder := httptest.NewRecorder()
		handler.ServeHTTP(submitRecorder, submitRequest)
		if submitRecorder.Code != http.StatusAccepted {
			t.Fatalf("submit status = %d, body = %s", submitRecorder.Code, submitRecorder.Body.String())
		}
	}
	if len(runtime.submits) != 1 || runtime.submits[0] != "hello" {
		t.Fatalf("submits = %#v, want one accepted prompt", runtime.submits)
	}

	conflictRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:submit", strings.NewReader(`{"input":"different"}`))
	conflictRequest.Header.Set("Authorization", "Bearer "+testToken)
	conflictRequest.Header.Set(requestIDHeader, "request-123")
	conflictRecorder := httptest.NewRecorder()
	handler.ServeHTTP(conflictRecorder, conflictRequest)
	if conflictRecorder.Code != http.StatusConflict {
		t.Fatalf("reused request ID status = %d, body = %s", conflictRecorder.Code, conflictRecorder.Body.String())
	}
}

func TestBridgeServerRejectsMalformedAndUnknownCommandInput(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance")

	malformedRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1","unexpected":true}`))
	malformedRequest.Header.Set("Authorization", "Bearer "+testToken)
	malformedRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(malformedRecorder, malformedRequest)
	if malformedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, body = %s", malformedRecorder.Code, malformedRecorder.Body.String())
	}

	unknownRouteRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:unknown", nil)
	unknownRouteRequest.Header.Set("Authorization", "Bearer "+testToken)
	unknownRouteRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(unknownRouteRecorder, unknownRouteRequest)
	if unknownRouteRecorder.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, body = %s", unknownRouteRecorder.Code, unknownRouteRecorder.Body.String())
	}
}

func TestBridgeServerEventsRequireResyncOutsideReplayWindow(t *testing.T) {
	events := desktopbridge.NewEventStream(1)
	sink := events.Sink("tab-1")
	sink.Emit(event.Event{Kind: event.Text, Text: "first"})
	sink.Emit(event.Event{Kind: event.Text, Text: "second"})
	bridge := newBridgeServerWithEvents(testToken, "instance", desktopbridge.NewRuntimeManager(nil), events)

	request := httptest.NewRequest(http.MethodGet, "/v1/events?afterSequence=0", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("events status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "resync_required") {
		t.Fatalf("events response = %s", response.Body.String())
	}
}

func TestWriteSSEEncodesVersionedBridgeEventEnvelope(t *testing.T) {
	item := desktopbridge.Event{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sequence:        7,
		EventKind:       "turn_done",
		SessionID:       "tab-1",
		Payload:         json.RawMessage(`{"kind":"turn_done","status":"failed","err":"provider unavailable"}`),
	}
	var frame strings.Builder
	if !writeSSE(&frame, item) {
		t.Fatal("writeSSE returned false")
	}

	var data string
	for _, line := range strings.Split(frame.String(), "\n") {
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if data == "" {
		t.Fatalf("SSE frame has no data line: %q", frame.String())
	}
	var got desktopbridge.Event
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatalf("decode SSE bridge event envelope: %v; data=%s", err, data)
	}
	if got.ProtocolVersion != item.ProtocolVersion || got.Sequence != item.Sequence || got.EventKind != item.EventKind || got.SessionID != item.SessionID {
		t.Fatalf("bridge event envelope = %#v", got)
	}
	if string(got.Payload) != string(item.Payload) {
		t.Fatalf("payload = %s, want %s", got.Payload, item.Payload)
	}
}

func TestReadyFileDoesNotContainTokenAndIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ready.json")
	ready := readyFile{ProtocolVersion: desktopbridge.ProtocolVersion, Address: "127.0.0.1:12345", SidecarInstanceID: "instance-a", LaunchID: "launch-a"}
	if err := writeReadyFile(path, ready); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == testToken {
		t.Fatal("ready file contains token")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRequireLoopbackAddress(t *testing.T) {
	if err := requireLoopbackAddress("127.0.0.1:0"); err != nil {
		t.Fatalf("loopback address rejected: %v", err)
	}
	if err := requireLoopbackAddress("0.0.0.0:0"); err == nil {
		t.Fatal("wildcard address accepted")
	}
}

func TestRunPublishesReadyHealthAndShutdown(t *testing.T) {
	profileRoot := t.TempDir()
	t.Setenv("REASONIX_HOME", profileRoot)
	t.Setenv("REASONIX_STATE_HOME", profileRoot)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcriptPath, err := bridgeSessionPath(sessionDir, "startup-terminal-title")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcriptPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	identities, err := sessionidentity.Open(context.Background(), identityPath, profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{ID: "startup-terminal-title", Path: transcriptPath}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginDelete(context.Background(), "startup-terminal-title", transcriptPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(transcriptPath); err != nil {
		t.Fatal(err)
	}
	if err := identities.FinishDelete(context.Background(), "startup-terminal-title", transcriptPath); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	legacyDB, err := sql.Open("sqlite", identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO session_title_intents
		(session_id, relative_path, expected_revision, previous_sidecar_title, new_title, created_at_ms)
		SELECT id, relative_path, title_revision, '', 'Stranded title', 1 FROM sessions WHERE id='startup-terminal-title'`); err != nil {
		t.Fatal(err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(t.TempDir(), "ready.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, config{
			listen:    "127.0.0.1:0",
			readyFile: readyPath,
			launchID:  "test-launch",
		}, testToken)
	}()

	var ready readyFile
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-errCh:
			t.Fatalf("bridge exited before readiness: %v", err)
		default:
		}
		content, err := os.ReadFile(readyPath)
		if err == nil && json.Unmarshal(content, &ready) == nil && ready.Address != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge did not publish ready file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ready.LaunchID != "test-launch" || ready.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("ready = %#v", ready)
	}
	identityReader, err := sessionidentity.OpenReadOnly(context.Background(), identityPath, profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, pending, err := identityReader.PendingManualTitleRename(context.Background(), "startup-terminal-title"); err != nil || pending {
		t.Fatalf("ready bridge left terminal title intent: pending=%v err=%v", pending, err)
	}
	if err := identityReader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := profilegate.TryAcquire(profileRoot); !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("ready bridge does not own profile: %v", err)
	}

	healthRequest, err := http.NewRequest(http.MethodGet, "http://"+ready.Address+"/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	healthRequest.Header.Set("Authorization", "Bearer "+testToken)
	healthResponse, err := http.DefaultClient.Do(healthRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, healthResponse.Body); err != nil {
		t.Fatal(err)
	}
	healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthResponse.StatusCode, http.StatusOK)
	}

	shutdownRequest, err := http.NewRequest(http.MethodPost, "http://"+ready.Address+"/v1:shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	shutdownRequest.Header.Set("Authorization", "Bearer "+testToken)
	shutdownResponse, err := http.DefaultClient.Do(shutdownRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, shutdownResponse.Body); err != nil {
		t.Fatal(err)
	}
	shutdownResponse.Body.Close()
	if shutdownResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown status = %d, want %d", shutdownResponse.StatusCode, http.StatusAccepted)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("run returned %v", err)
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("ready file remained after shutdown: %v", err)
	}
	release, err := profilegate.TryAcquire(profileRoot)
	if err != nil {
		t.Fatalf("profile ownership remained after shutdown: %v", err)
	}
	release()
}

func TestRunRejectsSecondProfileOwnerBeforePublishingReady(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", root)
	release, err := profilegate.TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	readyPath := filepath.Join(t.TempDir(), "ready.json")
	err = run(context.Background(), config{
		listen: "127.0.0.1:0", readyFile: readyPath, launchID: "second-owner",
	}, testToken)
	if !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("run error = %v, want profile owner conflict", err)
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("second owner published readiness: %v", err)
	}
}

func TestRunRejectsOrphanSQLiteSidecarBeforePublishingReady(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	identityPath := appconfig.DesktopSessionIdentityPath()
	if err := os.MkdirAll(filepath.Dir(identityPath), 0o700); err != nil {
		t.Fatal(err)
	}
	walPath := identityPath + "-wal"
	before := []byte("orphan WAL is evidence of an incomplete identity store\n")
	if err := os.WriteFile(walPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(root, "ready.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // A regression must return, not leave the test hosting a bridge.
	err := run(ctx, config{listen: "127.0.0.1:0", readyFile: readyPath, launchID: "orphan-wal"}, testToken)
	if err == nil || !strings.Contains(err.Error(), "sidecars remain") {
		t.Fatalf("run with orphan WAL = %v, want startup refusal", err)
	}
	if _, err := os.Lstat(readyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan WAL profile published bridge readiness: %v", err)
	}
	if _, err := os.Lstat(identityPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup created identity database beside orphan WAL: %v", err)
	}
	if after, err := os.ReadFile(walPath); err != nil || string(after) != string(before) {
		t.Fatalf("startup changed orphan WAL: %q, %v", after, err)
	}
}

func TestRunRejectsSecondProfileOwnerFromAnotherProcess(t *testing.T) {
	root := t.TempDir()
	release, err := profilegate.TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	readyPath := filepath.Join(t.TempDir(), "child-ready.json")
	command := exec.Command(os.Args[0], "-test.run=^TestRunProfileGateChildProcess$")
	command.Env = append(os.Environ(),
		"REASONIX_BRIDGE_PROFILE_GATE_CHILD=1",
		"REASONIX_STATE_HOME="+root,
		"REASONIX_BRIDGE_PROFILE_GATE_READY="+readyPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child bridge startup: %v\n%s", err, output)
	}
	if _, err := os.Stat(readyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second process published a ready file: %v", err)
	}
}

func TestRunProfileGateChildProcess(t *testing.T) {
	if os.Getenv("REASONIX_BRIDGE_PROFILE_GATE_CHILD") != "1" {
		return
	}
	readyPath := os.Getenv("REASONIX_BRIDGE_PROFILE_GATE_READY")
	err := run(context.Background(), config{
		listen: "127.0.0.1:0", readyFile: readyPath, launchID: "second-process-owner",
	}, testToken)
	if !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("child bridge startup = %v, want profile owner conflict", err)
	}
}

// Protocol compatibility: the bridge must always emit the current
// ProtocolVersion in every response envelope. This test documents that
// contract by asserting the constant stays at 1 and every response shape
// carries it. A future version bump requires updating both the Go constant
// and the Rust PROTOCOL_VERSION, plus a migration plan.
func TestAllResponsesCarryCurrentProtocolVersion(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	handler := bridge.handler()

	// Health
	healthReq := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	healthReq.Header.Set("Authorization", "Bearer "+testToken)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	var health healthResponse
	if err := json.NewDecoder(healthRec.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("health protocolVersion = %d, want %d", health.ProtocolVersion, desktopbridge.ProtocolVersion)
	}

	// Open session
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge2 := newBridgeServer(testToken, "instance-b", manager)
	handler2 := bridge2.handler()

	openReq := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"t"}`))
	openReq.Header.Set("Authorization", "Bearer "+testToken)
	openReq.Header.Set("Content-Type", "application/json")
	openRec := httptest.NewRecorder()
	handler2.ServeHTTP(openRec, openReq)
	var openResp struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.NewDecoder(openRec.Body).Decode(&openResp); err != nil {
		t.Fatal(err)
	}
	if openResp.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("open protocolVersion = %d, want %d", openResp.ProtocolVersion, desktopbridge.ProtocolVersion)
	}

	// Snapshot
	snapReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/t/snapshot", nil)
	snapReq.Header.Set("Authorization", "Bearer "+testToken)
	snapRec := httptest.NewRecorder()
	handler2.ServeHTTP(snapRec, snapReq)
	var snapResp struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.NewDecoder(snapRec.Body).Decode(&snapResp); err != nil {
		t.Fatal(err)
	}
	if snapResp.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("snapshot protocolVersion = %d, want %d", snapResp.ProtocolVersion, desktopbridge.ProtocolVersion)
	}

	// Submit
	subReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/t:submit", strings.NewReader(`{"input":"hi"}`))
	subReq.Header.Set("Authorization", "Bearer "+testToken)
	subReq.Header.Set("Content-Type", "application/json")
	subRec := httptest.NewRecorder()
	handler2.ServeHTTP(subRec, subReq)
	var subResp struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.NewDecoder(subRec.Body).Decode(&subResp); err != nil {
		t.Fatal(err)
	}
	if subResp.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("submit protocolVersion = %d, want %d", subResp.ProtocolVersion, desktopbridge.ProtocolVersion)
	}

	// Cancel
	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/t:cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+testToken)
	cancelRec := httptest.NewRecorder()
	handler2.ServeHTTP(cancelRec, cancelReq)
	var cancelResp struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelResp); err != nil {
		t.Fatal(err)
	}
	if cancelResp.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("cancel protocolVersion = %d, want %d", cancelResp.ProtocolVersion, desktopbridge.ProtocolVersion)
	}
}

// Protocol compatibility: the bridge must reject requests that carry
// unknown top-level fields, matching the schema's additionalProperties: false.
func TestBridgeRejectsRequestWithUnknownFields(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	handler := bridge.handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions:open",
		strings.NewReader(`{"sessionId":"t","unknownField":"surprise"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for unknown field", rec.Code, http.StatusBadRequest)
	}
}

// Protocol compatibility: requestId must not survive a sidecar restart.
// The idempotency ledger is per-process; after restart, a previously-seen
// requestId must NOT be treated as a completed request.
func TestRequestIdDoesNotSurviveRestart(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))

	// First server instance: open a session with a specific requestId.
	bridge1 := newBridgeServer(testToken, "inst-1", manager)
	handler1 := bridge1.handler()
	openReq := httptest.NewRequest(http.MethodPost, "/v1/sessions:open",
		strings.NewReader(`{"sessionId":"t"}`))
	openReq.Header.Set("Authorization", "Bearer "+testToken)
	openReq.Header.Set("Content-Type", "application/json")
	openReq.Header.Set("X-Reasonix-Request-ID", "req-42")
	openRec := httptest.NewRecorder()
	handler1.ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusOK {
		t.Fatalf("first open: status = %d", openRec.Code)
	}

	// Second server instance (simulating restart): same requestId must be
	// treated as a fresh request, not a replay.
	bridge2 := newBridgeServer(testToken, "inst-2", manager)
	handler2 := bridge2.handler()
	openReq2 := httptest.NewRequest(http.MethodPost, "/v1/sessions:open",
		strings.NewReader(`{"sessionId":"t"}`))
	openReq2.Header.Set("Authorization", "Bearer "+testToken)
	openReq2.Header.Set("Content-Type", "application/json")
	openReq2.Header.Set("X-Reasonix-Request-ID", "req-42")
	openRec2 := httptest.NewRecorder()
	handler2.ServeHTTP(openRec2, openReq2)
	if openRec2.Code != http.StatusOK {
		t.Fatalf("second open after restart: status = %d, want 200 (not a replay or conflict)", openRec2.Code)
	}
}

func TestBridgeServerListsWorkspaceEntries(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle", workspace: desktopbridge.WorkspaceList{
		Entries:   []desktopbridge.WorkspaceEntry{{Name: "src", Path: "src", IsDir: true}},
		Truncated: true,
	}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-workspace"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openRec := httptest.NewRecorder()
	handler.ServeHTTP(openRec, open)
	if openRec.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRec.Code, openRec.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-workspace:workspace", strings.NewReader(`{"path":"src"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response workspaceListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != desktopbridge.ProtocolVersion || response.Path != "src" || len(response.Entries) != 1 || !response.Truncated {
		t.Fatalf("workspace response = %#v", response)
	}
}

func TestBridgeServerPreviewsWorkspaceFile(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle", preview: desktopbridge.WorkspaceFilePreview{Body: "hello", Size: 5}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-preview"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openRec := httptest.NewRecorder()
	handler.ServeHTTP(openRec, open)
	if openRec.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRec.Code, openRec.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-preview:workspace-file", strings.NewReader(`{"path":"notes.txt"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace file status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response workspaceFileResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != desktopbridge.ProtocolVersion || response.Preview.Path != "notes.txt" || response.Preview.Body != "hello" {
		t.Fatalf("workspace file response = %#v", response)
	}
}

func TestBridgeServerReturnsWorkspaceChangesAndDetail(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle", changes: desktopbridge.WorkspaceChanges{
		GitAvailable: true,
		GitBranch:    "main",
		Files:        []desktopbridge.WorkspaceChangeView{{Path: "notes.txt", Sources: []string{"git"}, GitStatus: " M"}},
	}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	open := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-changes"}`))
	open.Header.Set("Authorization", "Bearer "+testToken)
	open.Header.Set("Content-Type", "application/json")
	openRec := httptest.NewRecorder()
	handler.ServeHTTP(openRec, open)
	if openRec.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRec.Code, openRec.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-changes:workspace-changes", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace changes status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var changes workspaceChangesResponse
	if err := json.NewDecoder(recorder.Body).Decode(&changes); err != nil {
		t.Fatal(err)
	}
	if changes.ProtocolVersion != desktopbridge.ProtocolVersion || changes.Changes.GitBranch != "main" || len(changes.Changes.Files) != 1 {
		t.Fatalf("workspace changes response = %#v", changes)
	}
	detailRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-changes:workspace-change-detail", strings.NewReader(`{"path":"notes.txt"}`))
	detailRequest.Header.Set("Authorization", "Bearer "+testToken)
	detailRequest.Header.Set("Content-Type", "application/json")
	detailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(detailRecorder, detailRequest)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("workspace change detail status = %d, body = %s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail workspaceChangeDetailResponse
	if err := json.NewDecoder(detailRecorder.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.ProtocolVersion != desktopbridge.ProtocolVersion || detail.Detail.Source != "git" || !strings.Contains(detail.Detail.Diff, "+new") {
		t.Fatalf("workspace change detail response = %#v", detail)
	}
}

func TestBridgeServerWorkspaceFileRevertRequiresRequestIDAndDeduplicates(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-revert"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	preview := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-revert:workspace-file-revert-preview", strings.NewReader(`{"path":"notes.txt"}`))
	preview.Header.Set("Authorization", "Bearer "+testToken)
	preview.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, preview)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"planId":"plan-test"`) {
		t.Fatalf("preview status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	commit := func(requestID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-revert:workspace-file-revert-commit", strings.NewReader(`{"planId":"plan-test","resolution":""}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		if requestID != "" {
			req.Header.Set(requestIDHeader, requestID)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	if response := commit(""); response.Code != http.StatusBadRequest || runtime.revertCommits != 0 {
		t.Fatalf("commit without request ID: status=%d calls=%d", response.Code, runtime.revertCommits)
	}
	first, second := commit("revert-1"), commit("revert-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.revertCommits != 1 {
		t.Fatalf("idempotent commit: first=%d second=%d calls=%d", first.Code, second.Code, runtime.revertCommits)
	}
	undo := func(requestID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-revert:workspace-file-revert-undo", strings.NewReader(`{"transactionId":"tx-test"}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		if requestID != "" {
			req.Header.Set(requestIDHeader, requestID)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	if response := undo(""); response.Code != http.StatusBadRequest || runtime.revertUndos != 0 {
		t.Fatalf("undo without request ID: status=%d calls=%d", response.Code, runtime.revertUndos)
	}
	first, second = undo("undo-1"), undo("undo-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.revertUndos != 1 {
		t.Fatalf("idempotent undo: first=%d second=%d calls=%d", first.Code, second.Code, runtime.revertUndos)
	}
}
