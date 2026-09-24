package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteLifecyclePersistsFenceAndTombstone(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-doomed.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "desktop", "state.sqlite")
	identities, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []Candidate{{ID: "doomed", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.FinishDelete(ctx, "doomed", path); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("finish without fence = %v", err)
	}
	if err := identities.BeginDelete(ctx, "doomed", path); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginDelete(ctx, "doomed", path); err != nil {
		t.Fatalf("retry begin deletion: %v", err)
	}
	if err := identities.FinishDelete(ctx, "doomed", path); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("finish while transcript exists = %v", err)
	}
	page, err := identities.ListVisible(ctx, 10, nil, "")
	if err != nil || page.Total != 0 || len(page.Records) != 0 {
		t.Fatalf("deleting identity appeared in directory: %#v, %v", page, err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	identities, err = Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "doomed")
	if err != nil || !exists || record.State != StateDeleting {
		t.Fatalf("interrupted deletion state = %#v, %v, %v", record, exists, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := identities.FinishDelete(ctx, "doomed", path); err != nil {
		t.Fatal(err)
	}
	if err := identities.FinishDelete(ctx, "doomed", path); err != nil {
		t.Fatalf("retry finish deletion: %v", err)
	}
	record, exists, err = identities.Get(ctx, "doomed")
	if err != nil || !exists || record.State != StateDeleted {
		t.Fatalf("deletion tombstone = %#v, %v, %v", record, exists, err)
	}
	if err := identities.Reserve(ctx, sessionDir, Candidate{ID: "doomed", Path: path}); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("tombstoned ID was reusable: %v", err)
	}
	if err := identities.BeginDelete(ctx, "doomed", path); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("finished tombstone began deletion again: %v", err)
	}
}

func TestBeginDeleteRejectsPathMismatchWithoutChangingState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-owned.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	if err := identities.Import(ctx, sessionDir, []Candidate{{ID: "owned", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginDelete(ctx, "owned", filepath.Join(sessionDir, "other.jsonl")); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("wrong path deletion = %v", err)
	}
	record, _, err := identities.Get(ctx, "owned")
	if err != nil || record.State != StateReady {
		t.Fatalf("wrong path changed identity: %#v, %v", record, err)
	}
}

func TestBeginDeleteAcceptsReservedAndMissingIdentities(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	reservedPath := filepath.Join(sessionDir, "tauri-reserved.jsonl")
	if err := identities.Reserve(ctx, sessionDir, Candidate{ID: "reserved", Path: reservedPath}); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(sessionDir, "tauri-missing.jsonl")
	if err := os.WriteFile(missingPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []Candidate{{ID: "missing", Path: missingPath}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}
	if err := identities.MarkMissing(ctx, "missing", missingPath); err != nil {
		t.Fatal(err)
	}
	for id, path := range map[string]string{"reserved": reservedPath, "missing": missingPath} {
		if err := identities.BeginDelete(ctx, id, path); err != nil {
			t.Fatalf("begin deletion of %s: %v", id, err)
		}
		if err := identities.FinishDelete(ctx, id, path); err != nil {
			t.Fatalf("finish deletion of %s: %v", id, err)
		}
	}
}
