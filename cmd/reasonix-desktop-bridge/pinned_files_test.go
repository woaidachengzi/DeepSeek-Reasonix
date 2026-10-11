package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/pinnedcontext"
)

func TestRuntimePinnedFilesRestoreAndFeedTurnLoader(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("original content"), 0600); err != nil {
		t.Fatal(err)
	}
	newRuntime := func() *controllerRuntime {
		exec := agent.New(nil, nil, agent.NewSession("base"), agent.Options{}, event.Discard)
		ctrl := control.New(control.Options{Executor: exec, Runner: exec, Sink: event.Discard, SessionPath: path, WorkspaceRoot: root})
		t.Cleanup(ctrl.Close)
		return &controllerRuntime{controller: ctrl}
	}
	runtime := newRuntime()
	if files, err := runtime.PinnedFiles(); err != nil || files == nil || len(files) != 0 {
		t.Fatalf("initial: %+v %v", files, err)
	}
	if files, err := runtime.ChangePinnedFile("notes.txt", true); err != nil || len(files) != 1 {
		t.Fatalf("pin: %+v %v", files, err)
	}
	restored := newRuntime()
	if files, err := restored.PinnedFiles(); err != nil || len(files) != 1 || files[0].Path != "notes.txt" {
		t.Fatalf("restored: %+v %v", files, err)
	}
	load := pinnedcontext.Loader(root, nil)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("changed content"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := load(context.Background(), path)
	if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Content != "changed content" {
		t.Fatalf("fresh context: %+v %v", snapshot, err)
	}
	if err := os.Remove(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if files, err := restored.PinnedFiles(); err != nil || len(files) != 1 || files[0].Error == "" {
		t.Fatalf("missing: %+v %v", files, err)
	}
	if _, err := restored.ChangePinnedFile("notes.txt", false); err != nil {
		t.Fatal(err)
	}
	snapshot, err = load(context.Background(), path)
	if err != nil || len(snapshot.Files) != 0 || len(snapshot.Issues) != 0 {
		t.Fatalf("revoked: %+v %v", snapshot, err)
	}
}
