package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge"
	"reflect"
	"sync"
	"testing"
)

func TestSessionEffortReachesProviderPersistsAndRejectsInvalidSelection(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		body["fixtureAuth"] = r.Header.Get("Authorization")
		calls = append(calls, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"reply\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer mock.Close()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("REASONIX_EFFORT_FIXTURE_KEY", "fixture-only")
	text := fmt.Sprintf(`default_model = "depth/deepseek-v4-flash"
[[providers]]
name = "depth"
kind = "openai"
base_url = "%s/v1"
api_key_env = "REASONIX_EFFORT_FIXTURE_KEY"
models = ["deepseek-v4-flash", "ordinary"]
default = "deepseek-v4-flash"
no_proxy = true
[[providers]]
name = "mimo"
kind = "openai"
base_url = "https://api.xiaomimimo.com/v1"
request_url = "%s/v1/chat/completions"
api_key_env = "REASONIX_EFFORT_FIXTURE_KEY"
models = ["mimo-v2.6-flash"]
no_proxy = true
`, mock.URL, mock.URL)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("REASONIX_EFFORT_FIXTURE_KEY=fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	factory := newControllerFactory(nil)
	manager := desktopbridge.NewRuntimeManager(factory)
	defer func() { _ = manager.Shutdown() }()
	level := "low"
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "effort-fixture", WorkspaceRoot: t.TempDir(), ModelRef: "depth/deepseek-v4-flash", Effort: &level})
	if err != nil {
		t.Fatal(err)
	}
	if view.Effort != "low" {
		t.Fatal(view)
	}
	send := func() {
		t.Helper()
		if _, err := manager.Submit(ctx, "effort-fixture", "reply once"); err != nil {
			t.Fatal(err)
		}
		waitForBridgeIdle(t, manager, "effort-fixture")
	}
	body := func() map[string]any { mu.Lock(); defer mu.Unlock(); return calls[len(calls)-1] }
	send()
	if body()["reasoning_effort"] != "low" {
		t.Fatal(body())
	}
	before, _ := manager.History("effort-fixture")
	view, err = manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, "max")
	if err != nil || view.Effort != "max" {
		t.Fatalf("%+v %v", view, err)
	}
	after, _ := manager.History("effort-fixture")
	if !reflect.DeepEqual(before.Messages, after.Messages) {
		t.Fatal("effort switch changed history")
	}
	if _, err = manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, "turbo"); err == nil {
		t.Fatal("invalid effort accepted")
	}
	snapshot, _ := manager.Snapshot()
	if snapshot.Effort != "max" {
		t.Fatal("failure lost selection")
	}
	send()
	if body()["reasoning_effort"] != "max" {
		t.Fatal(body())
	}
	// Credential refresh must inherit the exact session-local effort.
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("REASONIX_EFFORT_FIXTURE_KEY=second-fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	send()
	refreshed, _ := manager.Snapshot()
	if refreshed.Effort != "max" || body()["reasoning_effort"] != "max" || body()["fixtureAuth"] != "Bearer second-fixture-only" {
		t.Fatal("settings refresh lost reasoning", refreshed, body())
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	factory = newControllerFactory(nil)
	manager = desktopbridge.NewRuntimeManager(factory)
	view, err = manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "effort-fixture"})
	if err != nil || view.Effort != "max" || view.ModelRef != "depth/deepseek-v4-flash" {
		t.Fatalf("restart %+v %v", view, err)
	}
	send()
	if body()["reasoning_effort"] != "max" {
		t.Fatal(body())
	}
	view, err = manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, "disabled")
	if err != nil {
		t.Fatal(err)
	}
	send()
	if body()["thinking"].(map[string]any)["type"] != "disabled" || body()["reasoning_effort"] != nil {
		t.Fatal(body())
	}
	view, err = manager.SetSessionModel(ctx, "effort-fixture", "mimo/mimo-v2.6-flash")
	if err != nil || view.Effort != "auto" {
		t.Fatalf("model change %+v %v", view, err)
	}
	for _, level := range []string{"disabled", "enabled"} {
		view, err = manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, level)
		if err != nil {
			t.Fatal(err)
		}
		send()
		if body()["thinking"].(map[string]any)["type"] != level || body()["reasoning_effort"] != nil {
			t.Fatal(body())
		}
	}
	if _, err = manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, "high"); err == nil {
		t.Fatal("MiMo exposed nonexistent depth")
	}
	// A failed replacement build rolls back the metadata before serving the old runtime.
	factory.base.RequireKey = true
	if err := os.Remove(filepath.Join(home, ".env")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionEffort(ctx, "effort-fixture", view.ModelRef, "disabled"); err == nil {
		t.Fatal("keyless replacement unexpectedly built")
	}
	factory.base.RequireKey = false
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("REASONIX_EFFORT_FIXTURE_KEY=second-fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	intact, _ := manager.Snapshot()
	if intact.Effort != "enabled" {
		t.Fatal("failed build replaced active runtime", intact)
	}
	meta, ok, err := agent.LoadBranchMeta(view.Path)
	if err != nil || !ok || meta.ReasoningEffort == nil || *meta.ReasoningEffort != "enabled" {
		t.Fatalf("snapshot lost effort %+v %v", meta, err)
	}
	summary, err := loadProviderSummary()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range summary.Providers {
		if p.Name == "mimo" && !reflect.DeepEqual(p.Reasoning[0].Levels, []string{"auto", "enabled", "disabled"}) {
			t.Fatal(p.Reasoning)
		}
	}
}
