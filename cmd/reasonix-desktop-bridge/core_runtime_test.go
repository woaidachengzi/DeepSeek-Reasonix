package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

func TestBridgeSessionPathIsDeterministicAndContained(t *testing.T) {
	dir := t.TempDir()
	path, err := bridgeSessionPath(dir, "preview_42-a")
	if err != nil {
		t.Fatalf("bridge session path: %v", err)
	}
	want := filepath.Join(dir, "tauri-preview_42-a.jsonl")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := bridgeSessionPath(dir, "../outside"); err == nil {
		t.Fatal("unsafe bridge session ID produced a path")
	}
}

func TestTruncateBridgeHistoryContentPreservesUnicodeAndMarksTruncation(t *testing.T) {
	content := strings.Repeat("界", bridgeHistoryMaxContentRunes+1)
	got, truncated := truncateBridgeHistoryContent(content)
	if !truncated || !strings.HasPrefix(got, strings.Repeat("界", bridgeHistoryMaxContentRunes)) || !strings.HasSuffix(got, "[Preview truncated this message]") {
		t.Fatalf("history truncation = %q, %v", got, truncated)
	}
}

func TestControllerRuntimeRenamesSessionMetadataWithoutTouchingTranscript(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("REASONIX_HOME", t.TempDir())
	open := func() desktopbridge.Runtime {
		factory := newControllerFactory(nil)
		factory.base.WorkspaceRoot = workspace
		runtime, err := factory.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "tab-title"})
		if err != nil {
			t.Fatalf("open session: %v", err)
		}
		t.Cleanup(func() { _ = runtime.Shutdown() })
		return runtime
	}

	runtime := open()
	if got := runtime.Title(); got != "" {
		t.Fatalf("untitled session title = %q", got)
	}
	sessionPath := runtime.SessionPath()
	transcriptBefore, err := os.ReadFile(sessionPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := runtime.Rename("Release notes"); err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if got := runtime.Title(); got != "Release notes" {
		t.Fatalf("renamed session title = %q", got)
	}
	if _, ok, err := agent.LoadBranchMeta(sessionPath); err != nil || !ok {
		t.Fatalf("load branch meta = ok %v, err %v", ok, err)
	}
	transcriptAfter, err := os.ReadFile(sessionPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if string(transcriptAfter) != string(transcriptBefore) {
		t.Fatal("rename rewrote the session transcript")
	}

	restored := open()
	if restored.SessionPath() != sessionPath || restored.Title() != "Release notes" {
		t.Fatalf("restored session = %q, %q", restored.SessionPath(), restored.Title())
	}
}

func TestControllerRuntimeAttachFileCopiesIntoSessionWorkspace(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "research notes.txt")
	if err := os.WriteFile(source, []byte("selected context"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: workspace, Sink: event.Discard})
	if err != nil {
		t.Fatalf("build controller: %v", err)
	}
	t.Cleanup(controller.Close)

	runtime := &controllerRuntime{controller: controller}
	attachment, err := runtime.AttachFile(source)
	if err != nil {
		t.Fatalf("attach file: %v", err)
	}
	if attachment.Name != "research notes.txt" || attachment.Path == source || !strings.HasPrefix(attachment.Path, ".reasonix/attachments/") || attachment.Size != int64(len("selected context")) || attachment.IsImage {
		t.Fatalf("attachment metadata = %#v", attachment)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(attachment.Path)))
	if err != nil || string(data) != "selected context" {
		t.Fatalf("copied attachment content = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".reasonix", "attachments")); err != nil {
		t.Fatalf("workspace attachment directory: %v", err)
	}
}
