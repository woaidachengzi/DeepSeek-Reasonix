package pinnedcontext

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/store"
)

func TestChangePersistsAndRemovesUnavailableFile(t *testing.T) {
	root := t.TempDir()
	session := filepath.Join(root, "session.jsonl")
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("中文 context"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		infos, err := Change(root, session, "./notes.txt", true)
		if err != nil || len(infos) != 1 || infos[0].Path != "notes.txt" || infos[0].Error != "" {
			t.Fatalf("pin: %+v %v", infos, err)
		}
	}
	state, err := LoadState(session)
	if err != nil || !reflect.DeepEqual(state.Files, []string{"notes.txt"}) {
		t.Fatalf("restore: %+v %v", state, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	infos, err := Change(root, session, "notes.txt", true)
	if err != nil || len(infos) != 1 || infos[0].Error == "" {
		t.Fatalf("existing unavailable: %+v %v", infos, err)
	}
	for i := 0; i < 2; i++ {
		infos, err = Change(root, session, "notes.txt", false)
		if err != nil || len(infos) != 0 {
			t.Fatalf("unpin: %+v %v", infos, err)
		}
	}
	state, err = LoadState(session)
	if err != nil || len(state.Files) != 0 {
		t.Fatalf("unpin restore: %+v %v", state, err)
	}
}

func TestChangeRejectsNewUnavailableAndCorruptManifest(t *testing.T) {
	root := t.TempDir()
	session := filepath.Join(root, "session.jsonl")
	for _, path := range []string{"../outside", "missing.txt", "."} {
		if _, err := Change(root, session, path, true); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	if _, err := os.Stat(store.SessionPinnedContext(session)); !os.IsNotExist(err) {
		t.Fatalf("invalid pin saved a manifest: %v", err)
	}
	if err := os.WriteFile(store.SessionPinnedContext(session), []byte("broken manifest"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Change(root, session, "missing.txt", false); err == nil {
		t.Fatal("overwrote corrupt manifest")
	}
	raw, err := os.ReadFile(store.SessionPinnedContext(session))
	if err != nil || string(raw) != "broken manifest" {
		t.Fatalf("manifest changed: %q %v", raw, err)
	}
}

func TestSaveStateRebindsIdentityAndRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	parent, child := filepath.Join(root, "parent.jsonl"), filepath.Join(root, "child.jsonl")
	if err := SaveState(parent, []string{"z.txt", "./a.txt", "a.txt"}); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveState(child, state.Files); err != nil {
		t.Fatal(err)
	}
	copy, err := LoadState(child)
	if err != nil || copy.SessionID != "child" || !reflect.DeepEqual(copy.Files, []string{"a.txt", "z.txt"}) {
		t.Fatalf("copy: %+v %v", copy, err)
	}
	if err := SaveState(parent, []string{"../outside"}); err == nil {
		t.Fatal("accepted escape")
	}
	unchanged, err := LoadState(parent)
	if err != nil || !reflect.DeepEqual(unchanged, state) {
		t.Fatalf("failed write changed state: %+v %v", unchanged, err)
	}
	if err := SaveState("", nil); err == nil {
		t.Fatal("accepted missing session")
	}
}
