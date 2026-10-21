package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
)

func TestCompactPreferenceUpdatesResidentSessionsWithoutRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("COMPACT_TEST_KEY", "fixture")
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	manager := desktopbridge.NewRuntimePool(newControllerFactory(nil))
	defer func() {
		releaseOnce.Do(func() { close(release) })
		_ = manager.Shutdown()
		mock.Close()
	}()
	cfg := fmt.Sprintf(`default_model = "compact/chat"
[agent]
compact_ratio = 0.8
[[providers]]
name = "compact"
kind = "openai"
base_url = "%s/v1"
api_key_env = "COMPACT_TEST_KEY"
models = ["chat", "chat-v2"]
default = "chat"
no_proxy = true
`, mock.URL)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	rootA, rootB := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(rootB, "reasonix.toml"), []byte("[agent]\ncompact_ratio = 0.7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for id, root := range map[string]string{"a": rootA, "b": rootB} {
		if _, err := manager.Open(t.Context(), desktopbridge.OpenRequest{SessionID: id, WorkspaceRoot: root, ModelRef: "compact/chat"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.Open(t.Context(), desktopbridge.OpenRequest{SessionID: "b", WorkspaceRoot: rootB, ModelRef: "compact/chat"}); err != nil {
		t.Fatal(err)
	}
	ownerBefore, ok := manager.CommandSessionSnapshot("a")
	if !ok {
		t.Fatal("missing root owner")
	}
	if _, err := manager.Submit(t.Context(), "a", "answer this question"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("model request did not start")
	}
	bridge := newBridgeServer(testToken, "live-compact")
	bridge.runtimes = manager
	requestID := 0
	save := func(body string, wantStatus int) {
		t.Helper()
		requestID++
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/agent-preferences", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set(requestIDHeader, fmt.Sprintf("live-compact-%d", requestID))
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, req)
		if w.Code != wantStatus {
			t.Fatalf("save = %d: %s", w.Code, w.Body.String())
		}
	}
	assertRatios := func(a, b int) {
		t.Helper()
		for id, want := range map[string]int{"a": a, "b": b} {
			got, ok := manager.SessionMetrics(id)
			if !ok || got.CompactThresholdPercent != want {
				t.Fatalf("session %s threshold=%d want=%d", id, got.CompactThresholdPercent, want)
			}
		}
	}
	assertRatios(80, 70)
	save(`{"compactRatioPercent":75}`, http.StatusOK)
	assertRatios(75, 70)
	save(fmt.Sprintf(`{"compactRatioPercent":65,"scope":"project","workspaceRoot":%q}`, rootB), http.StatusOK)
	assertRatios(75, 65)
	save(`{"compactRatioPercent":99}`, http.StatusBadRequest)
	assertRatios(75, 65)
	ownerAfter, ok := manager.CommandSessionSnapshot("a")
	if !ok || ownerAfter.Scope != ownerBefore.Scope || !ownerAfter.State.Running {
		t.Fatal("settings replaced or stopped the running owner")
	}
	if selected, ok := manager.Snapshot(); !ok || selected.ID != "b" {
		t.Fatal("settings selected a background session")
	}
	releaseOnce.Do(func() { close(release) })
	deadline := time.Now().Add(10 * time.Second)
	for {
		view, ok := manager.SessionSnapshot("a")
		if ok && view.State == "idle" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background session did not finish after the setting changed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := manager.SetSessionModel(context.Background(), "a", "compact/chat-v2"); err != nil {
		t.Fatal(err)
	}
	assertRatios(75, 65)
	save(`{"compactRatioPercent":70}`, http.StatusOK)
	assertRatios(70, 65)
}

// Hold a fully constructed runtime before its manager publishes ownership. A
// preference write cannot enumerate it yet and must still reach it on admission.
func TestCompactPreferenceReachesOpeningSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("COMPACT_TEST_KEY", "fixture")
	cfg := `default_model = "compact/chat"
[agent]
compact_ratio = 0.8
[[providers]]
name = "compact"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
api_key_env = "COMPACT_TEST_KEY"
models = ["chat"]
default = "chat"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	factory := newControllerFactory(nil)
	manager := desktopbridge.NewRuntimePool(desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, req desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		runtime, err := factory.Open(ctx, req)
		if err != nil {
			return nil, err
		}
		close(entered)
		select {
		case <-release:
			return runtime, nil
		case <-ctx.Done():
			_ = runtime.Shutdown()
			return nil, ctx.Err()
		}
	}))
	defer manager.Shutdown()
	done := make(chan error, 1)
	go func() {
		_, err := manager.Open(t.Context(), desktopbridge.OpenRequest{SessionID: "opening", WorkspaceRoot: home, ModelRef: "compact/chat"})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("runtime did not reach the admission barrier")
	}
	bridge := newBridgeServer(testToken, "opening-compact")
	bridge.runtimes = manager
	request := httptest.NewRequest(http.MethodPost, "/v1/settings/agent-preferences", strings.NewReader(`{"compactRatioPercent":75}`))
	response := httptest.NewRecorder()
	bridge.setAgentPreferences(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", response.Code, response.Body.String())
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("opening session did not finish")
	}
	if metrics, ok := manager.SessionMetrics("opening"); !ok || metrics.CompactThresholdPercent != 75 {
		t.Fatalf("opening session missed committed preferences: %+v (ok=%v)", metrics, ok)
	}
}
