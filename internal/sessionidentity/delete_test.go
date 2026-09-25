package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestBeginDeleteRefusesLegacyPhysicalPathConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(sessionDir, "shared.jsonl")
	content := []byte("keep the shared transcript\n")
	if err := os.WriteFile(transcript, content, 0o600); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for position, identity := range []struct{ id, path string }{
		{id: "delete-first", path: transcript},
		{id: "delete-second", path: filepath.Join(aliasDir, filepath.Base(transcript))},
	} {
		relative, err := relativeTranscriptPath(root, identity.id, identity.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, identity.id, relative, position); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.BeginManualTitleRename(ctx, "delete-first", transcript, "", "Preserve pending title", 0); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginDelete(ctx, "delete-first", transcript); !errors.Is(err, ErrTranscriptPathConflict) {
		t.Fatalf("begin deletion of aliased identity = %v, want ErrTranscriptPathConflict", err)
	}
	if _, pending, err := store.PendingManualTitleRename(ctx, "delete-first"); err != nil || !pending {
		t.Fatalf("rejected deletion lost title intent: pending=%v err=%v", pending, err)
	}
	for _, id := range []string{"delete-first", "delete-second"} {
		record, exists, err := store.Get(ctx, id)
		if err != nil || !exists || record.State != StateReady {
			t.Fatalf("identity %s after rejected delete = %#v, %v, %v", id, record, exists, err)
		}
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE sessions SET state='deleting' WHERE id='delete-first'"); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginDelete(ctx, "delete-first", transcript); !errors.Is(err, ErrTranscriptPathConflict) {
		t.Fatalf("retry aliased deletion = %v, want ErrTranscriptPathConflict", err)
	}
	if record, exists, err := store.Get(ctx, "delete-first"); err != nil || !exists || record.State != StateDeleting {
		t.Fatalf("conflicted deletion retry changed its fence: %#v, %v, %v", record, exists, err)
	}
	if after, err := os.ReadFile(transcript); err != nil || string(after) != string(content) {
		t.Fatalf("conflicted delete changed transcript: %q, %v", after, err)
	}
}

func TestFinishDeleteRefusesLegacyPhysicalPathConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	firstPath := filepath.Join(sessionDir, "missing.jsonl")
	secondPath := filepath.Join(aliasDir, "missing.jsonl")
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for position, identity := range []struct{ id, path string }{
		{id: "finish-first", path: firstPath},
		{id: "finish-second", path: secondPath},
	} {
		relative, err := relativeTranscriptPath(root, identity.id, identity.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'deleting', 0, 0)`, identity.id, relative, position); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.FinishDelete(ctx, "finish-first", firstPath); !errors.Is(err, ErrTranscriptPathConflict) {
		t.Fatalf("finish aliased deletion = %v, want ErrTranscriptPathConflict", err)
	}
	for _, id := range []string{"finish-first", "finish-second"} {
		record, exists, err := store.Get(ctx, id)
		if err != nil || !exists || record.State != StateDeleting {
			t.Fatalf("identity %s after rejected finish = %#v, %v, %v", id, record, exists, err)
		}
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

func TestConcurrentFirstReadyCannotUndoDeleteFence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "desktop", "state.sqlite")
	readyStore, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer readyStore.Close()
	deleteStore, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteStore.Close()

	for index := range 32 {
		id := fmt.Sprintf("race-%02d", index)
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		if err := readyStore.Reserve(ctx, sessionDir, Candidate{ID: id, Path: path}); err != nil {
			t.Fatalf("reserve %s: %v", id, err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		readyResult := make(chan error, 1)
		deleteResult := make(chan error, 1)
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			readyResult <- readyStore.MarkReady(ctx, id, path)
		}()
		go func() {
			defer workers.Done()
			<-start
			deleteResult <- deleteStore.BeginDelete(ctx, id, path)
		}()
		close(start)
		workers.Wait()

		if err := <-deleteResult; err != nil {
			t.Fatalf("begin delete %s: %v", id, err)
		}
		if err := <-readyResult; err != nil && !errors.Is(err, ErrSessionStateConflict) {
			t.Fatalf("mark ready %s returned unexpected error: %v", id, err)
		}
		record, exists, err := readyStore.Get(ctx, id)
		if err != nil || !exists || record.State != StateDeleting {
			t.Fatalf("concurrent ready/delete state %s = %#v, %v, %v", id, record, exists, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := deleteStore.FinishDelete(ctx, id, path); err != nil {
			t.Fatalf("finish delete %s: %v", id, err)
		}
	}
}
