package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func TestTakeoverOpensOriginalProjectTranscriptWithoutRelocation(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	t.Setenv("REASONIX_TAURI_PROFILE_MODE", "takeover")
	path := filepath.Join(root, "projects", "original", "sessions", "20261012-model.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"role\":\"system\",\"content\":\"base\"}\n{\"role\":\"user\",\"content\":\"original question\"}\n{\"role\":\"assistant\",\"content\":\"original answer\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := sessionpath.LegacyID(root, path)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), root, []sessionidentity.Candidate{{ID: id, Path: path, WorkspaceRoot: workspace, Title: "original"}}); err != nil {
		t.Fatal(err)
	}
	identities.Close()
	resolved, err := bridgeSessionPath(appconfig.SessionDir(), id)
	if err != nil || resolved != path {
		t.Fatalf("path: %s %v", resolved, err)
	}
	runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: id, WorkspaceRoot: workspace})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	if runtime.SessionPath() != path {
		t.Fatalf("relocated session: %s", runtime.SessionPath())
	}
	history := runtime.History()
	if len(history) != 2 || history[1].Content != "original answer" {
		t.Fatalf("original history: %+v", history)
	}
	flat, err := sessionpath.TranscriptPath(appconfig.SessionDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(flat); !os.IsNotExist(err) {
		t.Fatalf("created a duplicate flat transcript: %v", err)
	}
	t.Setenv("REASONIX_TAURI_PROFILE_MODE", "")
	if _, err := bridgeSessionPath(appconfig.SessionDir(), id); err == nil {
		t.Fatal("Preview accepted a takeover-only ID")
	}
	t.Setenv("REASONIX_TAURI_PROFILE_MODE", "takeover")
	if _, err := bridgeSessionPath(appconfig.SessionDir(), sessionpath.LegacyPrefix+strings.Repeat("0", 64)); err == nil {
		t.Fatal("unregistered legacy ID became a fresh session")
	}
}
