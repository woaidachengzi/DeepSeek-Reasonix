package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reasonix/internal/desktopbridge"
	"strings"
	"testing"
)

func TestSteeringActualControllersRoutesOnceAtToolBoundary(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	for key, value := range map[string]string{"REASONIX_HOME": home, "REASONIX_STATE_HOME": home, "REASONIX_CACHE_HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file"} {
		t.Setenv(key, value)
	}
	fixture, endpoint := newOwnedCommandFixtureProvider(t)
	fixture.boundaryOnce = true
	cfg := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\"]\ndefault=\"alpha\"\n", endpoint)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("private read-only fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	events := desktopbridge.NewEventStream(1024)
	pool := desktopbridge.NewRuntimePool(newControllerFactory(events))
	defer pool.Shutdown()
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Open(ctx, desktopbridge.OpenRequest{SessionID: id, WorkspaceRoot: workspace}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Submit(ctx, id, "inspect this workspace"); err != nil {
			t.Fatal(err)
		}
		awaitOwnedCommand(t, fixture.calls)
	}
	beforeB, _ := pool.CommandSessionSnapshot("b")
	bridge := newBridgeServerWithEvents(testToken, "steer-fixture", pool, events)
	call := func(id, input, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/sessions/"+id+":steer", strings.NewReader(input))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set(requestIDHeader, key)
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	const guidance = "only inspect README; explain the result"
	for i := 0; i < 2; i++ {
		response := call("a", `{"input":"`+guidance+`"}`, "steer-fixture-key-1234567890123456")
		if response.Code != 202 {
			t.Fatal("steer rejected", response.Code, response.Body.String())
		}
	}
	afterB, _ := pool.CommandSessionSnapshot("b")
	if beforeB.Scope != afterB.Scope || afterB.Session.State != "running" {
		t.Fatal("steer affected background owner")
	}
	close(fixture.finish)
	applied := awaitOwnedCommand(t, fixture.calls)
	found := false
	for _, message := range applied.request.Messages {
		if strings.Contains(message.Content, guidance) {
			found = true
		}
	}
	if !found {
		t.Fatal("model request lost the accepted guide")
	}
	awaitParallelState(t, pool, "a", false)
	awaitParallelState(t, pool, "b", false)
	frames, _, _, off := events.Subscribe(0)
	defer off()
	count := 0
	for _, frame := range frames {
		if frame.EventKind == "steer" {
			if frame.SessionID != "a" {
				t.Fatal("guide applied to wrong owner")
			}
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate steering application", count)
	}
	response := call("a", `{"input":"late guide"}`, "late-steer-key-12345678901234567890")
	if response.Code != 409 || !strings.Contains(response.Body.String(), "steer_unavailable") {
		t.Fatal("ended turn admitted late guide", response.Code, response.Body.String())
	}
	if len(fixture.calls) != 0 {
		t.Fatal("rejected guide started a new turn")
	}
}

type closedSteeringRuntime struct {
	*parallelHTTPRuntime
	attempts int
}

func (r *closedSteeringRuntime) TrySteer(string) bool { r.attempts++; return false }
func TestSteeringClosedIntakeNeverFallsBackToSubmit(t *testing.T) {
	runtime := &closedSteeringRuntime{parallelHTTPRuntime: &parallelHTTPRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/sessions/a.jsonl", state: "idle"}}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	defer manager.Shutdown()
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	runtime.state = "running"
	if _, err := manager.Steer("a", "late guide"); !errors.Is(err, desktopbridge.ErrSteerUnavailable) {
		t.Fatal("closed intake not rejected", err)
	}
	if runtime.attempts != 1 || len(runtime.submits) != 0 {
		t.Fatal("rejected guidance started another turn")
	}
	if _, err := manager.Steer("a", strings.Repeat("x", 16*1024+1)); !errors.Is(err, desktopbridge.ErrInvalidInput) || runtime.attempts != 1 {
		t.Fatal("overlong guidance reached intake", err)
	}
}
