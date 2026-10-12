package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/takeover"
)

// Optional private real-data acceptance. This never submits a provider request.
func TestPrivateTakeoverHistoryAcceptance(t *testing.T) {
	root := os.Getenv("REASONIX_PRIVATE_TAKEOVER_FIXTURE")
	if root == "" {
		t.Skip("private prepared takeover fixture not requested")
	}
	if filepath.Clean(root) == filepath.Join(os.Getenv("HOME"), ".reasonix") || !filepath.IsAbs(root) || !strings.HasPrefix(filepath.Clean(root), "/private/tmp/") {
		t.Fatal("fixture must be a private temporary profile")
	}
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	t.Setenv("REASONIX_TAURI_PROFILE_MODE", "takeover")
	data, err := os.ReadFile(filepath.Join(root, "tauri-takeover-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt takeover.Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	for index, row := range receipt.Rows {
		runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: row.SessionID, WorkspaceRoot: row.WorkspaceRoot})
		if err != nil {
			t.Fatalf("row %d: %v", index, err)
		}
		t.Logf("row %d history messages %d", index, len(runtime.History()))
		runtime.Shutdown()
	}
}
