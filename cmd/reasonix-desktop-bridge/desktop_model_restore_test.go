package main

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/boot"
	"reasonix/internal/desktopbridge"
)

func TestDesktopRestoreSkipsUnknownDefaultsButPreservesExplicitSelection(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	text := `default_model = "retired/old-chat"
[desktop]
provider_access = ["allowed"]
[[providers]]
name = "excluded"
kind = "openai"
base_url = "http://127.0.0.1:9/v1"
models = ["excluded-chat"]
[[providers]]
name = "allowed"
kind = "openai"
base_url = "http://127.0.0.1:9/v1"
models = ["current-chat"]
`
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "reasonix.toml"), []byte("default_model = \"retired-project/old-chat\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	factory := newControllerFactory(nil)
	for _, scope := range []string{"", workspace} {
		options, _, err := factory.options(desktopbridge.OpenRequest{SessionID: "model-recovery", WorkspaceRoot: scope})
		if err != nil || options.Model != "allowed/current-chat" {
			t.Fatalf("desktop fallback = %q, %v", options.Model, err)
		}
	}
	for _, explicit := range []string{"retired/old-chat", "excluded/excluded-chat"} {
		_, _, err := factory.options(desktopbridge.OpenRequest{SessionID: "model-recovery", WorkspaceRoot: workspace, ModelRef: explicit})
		if !errors.Is(err, boot.ErrUnknownModel) {
			t.Fatalf("explicit model silently replaced: %v", err)
		}
	}
	options, _, err := factory.options(desktopbridge.OpenRequest{SessionID: "model-recovery", WorkspaceRoot: workspace, ModelRef: "allowed/current-chat"})
	if err != nil || options.Model != "allowed/current-chat" {
		t.Fatalf("valid selection not retained: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "tauri-model-recovery.jsonl.meta"),
		[]byte(`{"reasoning_model":"retired/old-chat","reasoning_effort":"max"}`), 0600); err != nil {
		t.Fatal(err)
	}
	options, _, err = factory.options(desktopbridge.OpenRequest{SessionID: "model-recovery", WorkspaceRoot: workspace})
	if err != nil || options.Model != "allowed/current-chat" || options.EffortOverride != nil {
		t.Fatalf("removed saved model did not recover cleanly: %q, %v", options.Model, err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != text {
		t.Fatal("opening history rewrote user model preferences")
	}
}

func TestUnavailableDesktopModelIsActionableWithoutPrivateDetails(t *testing.T) {
	response := httptest.NewRecorder()
	(&bridgeServer{}).writeRuntimeError(response, fmt.Errorf("%w: private-provider-secret", boot.ErrUnknownModel), "private transcript path")
	if response.Code != 400 || !strings.Contains(response.Body.String(), "model_unavailable") || strings.Contains(response.Body.String(), "private") {
		t.Fatal("unavailable model misclassified or private details leaked")
	}
}
