package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestOfflineSnapshotVerifiesAndStagesCompleteProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "preview-profile")
	sessionDir := filepath.Join(profile, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-tauri-first.jsonl")
	writeTranscript(t, transcript)
	sidecar := transcript + ".meta"
	if err := os.WriteFile(sidecar, []byte("metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(profile, "empty-memory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "MEMORY.md"), []byte("long-term memory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "run-helper.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(profile, "desktop", "session-state-v1.sqlite")
	writer, err := Open(ctx, identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Import(ctx, sessionDir, []Candidate{{ID: "tauri-first", Path: transcript}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-first", Title: "First"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyOfflineSnapshot(ctx, snapshot)
	if err != nil || manifest.Version != offlineSnapshotVersion || len(manifest.Files) < 4 {
		t.Fatalf("verified snapshot = %#v, %v", manifest, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(snapshot)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("snapshot permissions = %#v, %v", info, err)
		}
	}
	recoveryParent := filepath.Join(root, "recovery")
	if err := os.Mkdir(recoveryParent, 0o700); err != nil {
		t.Fatal(err)
	}
	staged, err := StageOfflineSnapshot(ctx, snapshot, recoveryParent)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		executable, err := os.Stat(filepath.Join(staged, "profile", "run-helper.sh"))
		if err != nil || executable.Mode().Perm() != 0o700 {
			t.Fatalf("staged executable permissions = %#v, %v", executable, err)
		}
	}
	if info, err := os.Stat(filepath.Join(staged, "profile", "empty-memory")); err != nil || !info.IsDir() {
		t.Fatalf("empty profile directory was not restored: %#v, %v", info, err)
	}
	for _, rel := range []string{"sessions/tauri-tauri-first.jsonl", "sessions/tauri-tauri-first.jsonl.meta", "MEMORY.md"} {
		original, err := os.ReadFile(filepath.Join(profile, rel))
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := os.ReadFile(filepath.Join(staged, "profile", rel))
		if err != nil || !reflect.DeepEqual(original, recovered) {
			t.Fatalf("staged recovery differs at %s: %v", rel, err)
		}
	}
	originalCatalog, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	recoveredCatalog, err := os.ReadFile(filepath.Join(staged, "catalog", "workbench-sessions.json"))
	if err != nil || !reflect.DeepEqual(originalCatalog, recoveredCatalog) {
		t.Fatalf("catalog recovery differs: %v", err)
	}
	restoredDB, err := OpenReadOnly(ctx, filepath.Join(staged, "profile", "desktop", "session-state-v1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	records, err := restoredDB.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "tauri-first" {
		t.Fatalf("restored identity database = %#v, %v", records, err)
	}
}

func TestOfflineSnapshotRejectsTamperingAndNestedDestination(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	if _, err := CreateOfflineSnapshot(ctx, profile, catalog, filepath.Join(profile, "sessions")); err == nil {
		t.Fatal("snapshot accepted a destination inside its source profile")
	}
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(snapshot, "profile", "sessions", "tauri-tauri-one.jsonl")
	if err := os.WriteFile(member, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil {
		t.Fatal("tampered snapshot verified")
	}
	if _, err := StageOfflineSnapshot(ctx, snapshot, root); err == nil {
		t.Fatal("tampered snapshot was staged for recovery")
	}
	original, err := os.ReadFile(filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	if err != nil || string(original) != "{}\n" {
		t.Fatalf("source profile changed: %q, %v", original, err)
	}
}

func TestOfflineSnapshotRejectsSymlinkAndLeavesIncompleteMarker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	if err := os.Symlink(catalog, filepath.Join(profile, "sessions", "linked-catalog")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err == nil || snapshot == "" {
		t.Fatalf("symlink source snapshot = %q, %v", snapshot, err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete snapshot has a valid manifest: %v", err)
	}
}
