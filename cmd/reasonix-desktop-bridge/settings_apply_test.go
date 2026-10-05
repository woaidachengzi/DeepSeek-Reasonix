package main

// A provider API key saved after the controller was built must reach the
// provider boundary. The controller freezes credentials at build time, so this
// only holds when Submit rebuilds a runtime whose settings fingerprint moved.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

const settingsApplyProviderKind = "bridge-settings-apply-test"

type settingsApplyProvider struct {
	recorder *settingsApplyRecorder
	key      string
}

func (p *settingsApplyProvider) Name() string { return settingsApplyProviderKind }
func (p *settingsApplyProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.recorder.observeKey(p.key)
	return p.recorder.Stream(ctx, req)
}

type settingsApplyRecorder struct {
	mu   sync.Mutex
	keys []string
}

func (r *settingsApplyRecorder) Name() string { return settingsApplyProviderKind }

func (r *settingsApplyRecorder) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	chunks := []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}
	out := make(chan provider.Chunk, len(chunks))
	for _, c := range chunks {
		out <- c
	}
	close(out)
	return out, nil
}

func (r *settingsApplyRecorder) observeKey(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = append(r.keys, key)
}

func (r *settingsApplyRecorder) observed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.keys...)
}

// waitForBridgeIdle lets an admitted turn finish. Submit returns after
// admission rather than after Agent work, so a submit that follows too closely
// sees the session still running and the rebuild gate refuses to swap the
// controller underneath a live turn.
func waitForBridgeIdle(t *testing.T, manager *desktopbridge.RuntimeManager, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		view, ok := manager.Snapshot()
		if ok && view.ID == sessionID && view.State == "idle" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %q never returned to idle; state=%q", sessionID, view.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSubmitAppliesSavedAPIKeyToProvider is the effect test for this fix: the
// key that reaches the provider is the one on disk at submit time, not the one
// frozen when the session was opened.
func TestSubmitAppliesSavedAPIKeyToProvider(t *testing.T) {
	const keyEnv = "REASONIX_SETTINGS_APPLY_TEST_KEY"
	recorder := &settingsApplyRecorder{}
	provider.Register(settingsApplyProviderKind, func(cfg provider.Config) (provider.Provider, error) {
		if cfg.APIKey == "unbuildable-key" {
			return nil, errors.New("fixture provider refused replacement")
		}
		return &settingsApplyProvider{recorder: recorder, key: cfg.APIKey}, nil
	})

	t.Setenv(keyEnv, "")
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	config := `default_model = "settings-apply/chat"

[[providers]]
name = "settings-apply"
kind = "` + settingsApplyProviderKind + `"
base_url = "http://127.0.0.1:9/v1"
api_key_env = "` + keyEnv + `"
models = ["chat"]
default = "chat"

[desktop]
provider_access = ["settings-apply"]
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(keyEnv+"=first-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	factory := &settingsControllerFactory{controllerFactory: newControllerFactory(nil)}
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	request := desktopbridge.OpenRequest{SessionID: "settings-apply", WorkspaceRoot: t.TempDir()}
	if _, err := manager.Open(ctx, request); err != nil {
		t.Fatalf("open session: %v", err)
	}
	if _, err := manager.Submit(ctx, request.SessionID, "hello"); err != nil {
		t.Fatalf("submit before saving: %v", err)
	}
	waitForBridgeIdle(t, manager, request.SessionID)
	observed := recorder.observed()
	if len(observed) == 0 || observed[0] != "first-key" {
		t.Fatalf("first submit reached the provider with %q, want %q", observed, "first-key")
	}

	old := factory.latest
	old.controller.RestoreSessionAuthorizations(control.SessionAuthorizations{Grants: []string{"bash|go test ./..."}, PlanModeReadOnlyCommands: []string{"git status"}})
	old.controller.SetToolApprovalMode(control.ToolApprovalAuto)
	temp := old.controller.SessionTemp()
	// A deterministic provider-construction failure must preserve the actual
	// controller, conversation, and session grants, rather than retrying the
	// same broken disk configuration after closing the only usable controller.
	if err := persistProviderAPIKey(setProviderKeyRequest{ProviderName: "settings-apply", APIKey: "unbuildable-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(ctx, request.SessionID, "must not send"); !errors.Is(err, desktopbridge.ErrSessionSettingsApply) {
		t.Fatalf("failed rebuild: %v", err)
	}
	if factory.latest != old || old.controller.SessionTemp().Sealed() {
		t.Fatal("failed rebuild destroyed old controller")
	}
	if history, err := manager.History(request.SessionID); err != nil || len(history.Messages) == 0 {
		t.Fatalf("failed build lost visible history: %+v %v", history, err)
	}
	if len(recorder.observed()) != len(observed) {
		t.Fatal("failed build submitted a provider request")
	}
	// Reverting the failed save can still use the original accepted runtime.
	if err := persistProviderAPIKey(setProviderKeyRequest{ProviderName: "settings-apply", APIKey: "first-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(ctx, request.SessionID, "retry original"); err != nil {
		t.Fatal(err)
	}
	waitForBridgeIdle(t, manager, request.SessionID)
	if factory.latest != old {
		t.Fatal("failure replaced original controller")
	}
	if keys := recorder.observed(); len(keys) != len(observed)+1 || keys[len(keys)-1] != "first-key" {
		t.Fatalf("old controller no longer usable: %v", keys)
	}
	// Exercise the actual save helper after the first accepted run froze its key.
	if err := persistProviderAPIKey(setProviderKeyRequest{ProviderName: "settings-apply", APIKey: "second-key"}); err != nil {
		t.Fatal(err)
	}
	waitForBridgeIdle(t, manager, request.SessionID)
	if _, err := manager.Submit(ctx, request.SessionID, "hello again"); err != nil {
		t.Fatalf("submit after saving: %v", err)
	}
	waitForBridgeIdle(t, manager, request.SessionID)

	next := factory.latest
	if next == old {
		t.Fatal("saved key did not replace controller")
	}
	auth := next.controller.SessionAuthorizations()
	if len(auth.Grants) != 1 || auth.Grants[0] != "bash|go test ./..." || len(auth.PlanModeReadOnlyCommands) != 1 {
		t.Fatalf("lost session authorizations: %+v", auth)
	}
	if next.controller.ToolApprovalMode() != control.ToolApprovalAuto {
		t.Fatal("lost approval posture")
	}
	if next.controller.SessionTemp() != temp || temp.Sealed() {
		t.Fatal("lost logical-session temporary storage")
	}
	if next.SessionPath() != old.SessionPath() {
		t.Fatal("session path changed")
	}
	found := false
	for _, message := range next.controller.History() {
		if message.Role == provider.RoleUser && message.Content == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost previous conversation")
	}

	observed = recorder.observed()
	last := observed[len(observed)-1]
	if last != "second-key" {
		t.Fatalf("submit after saving reached the provider with %q, want %q; the saved API key never left disk (observed %v)", last, "second-key", observed)
	}
}

// TestSubmitWithoutSettingsChangeKeepsRuntime pins the no-op path: when the
// settings fingerprint is unchanged Submit must not tear down and rebuild the
// session, which would reset its in-memory state on every message.
func TestSubmitWithoutSettingsChangeKeepsRuntime(t *testing.T) {
	opened := 0
	runtime := &bridgeTestRuntime{path: "/sessions/a.jsonl", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(
		desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
			opened++
			return runtime, nil
		}),
	)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "a"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := manager.Submit(context.Background(), "a", "hello"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if opened != 1 {
		t.Fatalf("factory opened %d runtimes for 3 submits, want 1", opened)
	}
	if len(runtime.submits) != 3 {
		t.Fatalf("runtime received %d submits, want 3", len(runtime.submits))
	}
}

// Capture the actual boot controllers without replacing the production rebuild.
type settingsControllerFactory struct {
	*controllerFactory
	latest *controllerRuntime
}

func (f *settingsControllerFactory) Open(ctx context.Context, req desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
	r, err := f.controllerFactory.Open(ctx, req)
	if err == nil {
		f.latest = r.(*controllerRuntime)
	}
	return r, err
}
func (f *settingsControllerFactory) Rebuild(ctx context.Context, old desktopbridge.Runtime, req desktopbridge.OpenRequest) (desktopbridge.SettingsRuntime, error) {
	r, err := f.controllerFactory.Rebuild(ctx, old, req)
	if err == nil {
		f.latest = r.(*controllerRuntime)
	}
	return r, err
}

type settingsReadErrorRuntime struct{ *bridgeTestRuntime }

func (r *settingsReadErrorRuntime) ModelSettingsState() (string, string, error) {
	return "old", "", errors.New("sensitive fixture detail")
}
func TestSettingsReadFailureReturnsSafeConflict(t *testing.T) {
	runtime := &settingsReadErrorRuntime{&bridgeTestRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance", manager)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/a:submit", strings.NewReader(`{"input":"hello"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	res := httptest.NewRecorder()
	bridge.handler().ServeHTTP(res, req)
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "settings_apply_failed") || strings.Contains(res.Body.String(), "sensitive fixture detail") {
		t.Fatalf("unsafe settings error: %d %s", res.Code, res.Body.String())
	}
	if len(runtime.submits) != 0 {
		t.Fatal("read failure admitted turn")
	}
}
