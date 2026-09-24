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
