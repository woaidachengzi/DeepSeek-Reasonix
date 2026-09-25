package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func titleIntentFixture(t *testing.T) (context.Context, string, string, string, *Store) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-title.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "desktop", "identity.sqlite")
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, filepath.Dir(path), []Candidate{{ID: "title", Path: path}}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return ctx, root, dbPath, path, store
}

func TestManualTitleIntentSurvivesReopenAndCommitsAtomically(t *testing.T) {
	ctx, root, dbPath, path, store := titleIntentFixture(t)
	if err := store.BeginManualTitleRename(ctx, "title", path, "", "Chosen title", 0); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.PendingManualTitleRenameIDs(ctx); err != nil || len(ids) != 1 || ids[0] != "title" {
		t.Fatalf("pending title IDs = %#v, %v", ids, err)
	}
	if err := store.BeginManualTitleRename(ctx, "title", path, "", "Other title", 0); !errors.Is(err, ErrTitleConflict) {
		t.Fatalf("second manual title intent = %v, want conflict", err)
	}
	if err := store.SetTitle(ctx, "title", 0, "Competing title", TitleManualRename); !errors.Is(err, ErrTitleConflict) {
		t.Fatalf("ordinary title writer during pending intent = %v, want conflict", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	intent, exists, err := store.PendingManualTitleRename(ctx, "title")
	if err != nil || !exists || intent.Path != path || intent.NewTitle != "Chosen title" ||
		intent.PreviousSidecarTitle != "" || intent.ExpectedRevision != 0 {
		t.Fatalf("reopened intent = %#v exists=%v err=%v", intent, exists, err)
	}
	if err := store.CommitManualTitleRename(ctx, "title", path); err != nil {
		t.Fatal(err)
	}
	record, exists, err := store.Get(ctx, "title")
	if err != nil || !exists || record.Title != "Chosen title" || record.TitleSource != TitleUser || record.TitleRevision != 1 {
		t.Fatalf("committed title = %#v exists=%v err=%v", record, exists, err)
	}
	if _, exists, err := store.PendingManualTitleRename(ctx, "title"); err != nil || exists {
		t.Fatalf("committed intent remains: exists=%v err=%v", exists, err)
	}
	if ids, err := store.PendingManualTitleRenameIDs(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("committed title still listed as pending: %#v, %v", ids, err)
	}
}

func TestManualTitleIntentRetainsIntentAfterRejectedCommit(t *testing.T) {
	ctx, _, dbPath, path, store := titleIntentFixture(t)
	defer store.Close()
	if err := store.BeginManualTitleRename(ctx, "title", path, "", "Chosen title", 0); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_title_intent BEFORE UPDATE OF title ON sessions
		BEGIN SELECT RAISE(ABORT, 'injected title write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitManualTitleRename(ctx, "title", path); err == nil {
		t.Fatal("injected title failure was ignored")
	}
	if _, exists, err := store.PendingManualTitleRename(ctx, "title"); err != nil || !exists {
		t.Fatalf("rejected commit lost intent: exists=%v err=%v", exists, err)
	}
	record, exists, err := store.Get(ctx, "title")
	if err != nil || !exists || record.Title != "" || record.TitleRevision != 0 {
		t.Fatalf("rejected commit changed title: %#v exists=%v err=%v", record, exists, err)
	}
	if _, err := db.Exec("DROP TRIGGER reject_title_intent"); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitManualTitleRename(ctx, "title", path); err != nil {
		t.Fatalf("retry durable title intent: %v", err)
	}
}

func TestManualTitleIntentCanBeAbandonedWhenSidecarDidNotChange(t *testing.T) {
	ctx, _, _, path, store := titleIntentFixture(t)
	defer store.Close()
	if err := store.BeginManualTitleRename(ctx, "title", path, "", "Unused title", 0); err != nil {
		t.Fatal(err)
	}
	if err := store.AbandonManualTitleRename(ctx, "title", 0); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.PendingManualTitleRename(ctx, "title"); err != nil || exists {
		t.Fatalf("abandoned intent remains: exists=%v err=%v", exists, err)
	}
	record, exists, err := store.Get(ctx, "title")
	if err != nil || !exists || record.Title != "" || record.TitleRevision != 0 {
		t.Fatalf("abandon changed title: %#v exists=%v err=%v", record, exists, err)
	}
}

func TestOpenMigratesV4ToTitleIntentSchemaWithoutChangingIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "identity.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, relative_path TEXT NOT NULL UNIQUE, workspace_root TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '', title_source TEXT NOT NULL DEFAULT 'fallback',
		title_revision INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0,
		state TEXT NOT NULL DEFAULT 'ready', created_at_ms INTEGER NOT NULL, updated_at_ms INTEGER NOT NULL);
		CREATE INDEX sessions_state_position_id ON sessions(state, position, id);
		INSERT INTO sessions VALUES ('older', 'sessions/tauri-older.jsonl', '', 'Preserved title', 'user', 3, 0, 'ready', 1, 2);
		PRAGMA user_version=4`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("migrated schema version = %d, %v", version, err)
	}
	record, exists, err := store.Get(ctx, "older")
	if err != nil || !exists || record.Title != "Preserved title" || record.TitleSource != TitleUser || record.TitleRevision != 3 ||
		record.Path != filepath.Join(root, "sessions", "tauri-older.jsonl") {
		t.Fatalf("v4 identity changed during migration: %#v exists=%v err=%v", record, exists, err)
	}
	if _, exists, err := store.PendingManualTitleRename(ctx, "older"); err != nil || exists {
		t.Fatalf("fresh v5 intent table is not empty: exists=%v err=%v", exists, err)
	}
}
