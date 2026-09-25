package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenReadOnlyNeverCreatesMissingDatabase(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "not-created", "identity.sqlite")
	if _, err := OpenReadOnly(context.Background(), path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing database error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only open created directory: %v", err)
	}
}

func TestIdentityOpenRejectsDatabaseSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "outside.sqlite")
	store, err := Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "profile", "identity.sqlite")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, link); err == nil {
		t.Fatal("write open followed a symlinked identity database")
	}
	if _, err := OpenReadOnly(ctx, link, root); err == nil {
		t.Fatal("read-only open followed a symlinked identity database")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected symlink open changed the target database")
	}
}

func TestIdentityOpenRejectsSymlinkedSQLiteJournal(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "identity.sqlite")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside-wal")
	if err := os.WriteFile(target, []byte("must not be opened as a journal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dbPath+"-wal"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, dbPath); err == nil {
		t.Fatal("write open followed a symlinked SQLite journal")
	}
	if _, err := OpenReadOnly(ctx, dbPath, root); err == nil {
		t.Fatal("read-only open accepted a symlinked SQLite journal")
	}
}

func TestIdentityOpenRejectsDatabaseDirectorySymlinkEscape(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "identity.sqlite")
	store, err := Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "desktop")); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "desktop", "identity.sqlite")
	if _, err := Open(ctx, linkPath, root); err == nil {
		t.Fatal("write open accepted an identity directory symlink escape")
	}
	if _, err := OpenReadOnly(ctx, linkPath, root); err == nil {
		t.Fatal("read-only open accepted an identity directory symlink escape")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected directory symlink open changed the target database")
	}
}

func TestOpenReadOnlyListsButCannotImportOrRename(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-existing.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("{}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "identity.sqlite")
	writer, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Import(ctx, root, []Candidate{{ID: "existing", Path: path, Title: "Title"}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	records, err := reader.List(ctx)
	if err != nil || len(records) != 1 || records[0].Title != "Title" {
		t.Fatalf("read-only records = %#v, %v", records, err)
	}
	if known, err := reader.HasRegisteredID(ctx, "existing"); err != nil || !known {
		t.Fatalf("read-only registered ID = %v, %v", known, err)
	}
	if known, err := reader.HasRegisteredID(ctx, "new"); err != nil || known {
		t.Fatalf("read-only unregistered ID = %v, %v", known, err)
	}
	if err := reader.SetTitle(ctx, "existing", 0, "Changed", TitleManualRename); err == nil {
		t.Fatal("read-only title update succeeded")
	}
	if err := reader.Import(ctx, root, []Candidate{{ID: "existing", Path: path, Position: 2}}); err == nil {
		t.Fatal("read-only import succeeded")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only access changed database bytes: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, content) {
		t.Fatalf("read-only access changed transcript: %q, %v", got, err)
	}
}
