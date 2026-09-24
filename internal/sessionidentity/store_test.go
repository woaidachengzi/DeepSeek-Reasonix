package sessionidentity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/desktopbridge/sessionpath"
)

func TestImportPreservesTranscriptAndIdentityAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-tauri-first.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("{\"role\":\"user\",\"content\":\"hello\"}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	entry := Candidate{ID: "tauri-first", Path: path, WorkspaceRoot: "/work/first", Title: "Hello", Position: 2}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	first, err := store.List(ctx)
	if err != nil || len(first) != 1 || first[0].ID != entry.ID || first[0].Missing {
		t.Fatalf("first import = %#v, %v", first, err)
	}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	second, err := store.List(ctx)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotent import = %#v, %v; want %#v", second, err, first)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(ctx, dbPath); err == nil {
		_ = reopened.Close()
		t.Fatal("existing relative-path database opened without its profile root")
	}
	store, err = Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	third, err := store.List(ctx)
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatalf("reopened import = %#v, %v; want %#v", third, err, first)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, content) {
		t.Fatalf("transcript changed: %q, %v", got, err)
	}
}

func TestImportMarksKnownMissingWithoutCreatingFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(root, "sessions", "tauri-known.jsonl")
	entry := Candidate{ID: "known", Path: path}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	if records, err := store.List(ctx); err != nil || len(records) != 0 {
		t.Fatalf("never-persisted tab registered: %#v, %v", records, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx)
	if err != nil || len(records) != 1 || !records[0].Missing {
		t.Fatalf("missing transcript = %#v, %v", records, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("import recreated missing transcript: %v", err)
	}
}

func TestReservedIdentitySurvivesSidecarsAndTransitions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-draft.jsonl")
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	candidate := Candidate{ID: "draft", Path: path, WorkspaceRoot: "/work/project"}
	if err := store.Reserve(ctx, sessionDir, candidate); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".meta", []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Reserve(ctx, sessionDir, candidate); err != nil {
		t.Fatalf("idempotent reservation: %v", err)
	}
	record, ok, err := store.Get(ctx, "draft")
	if err != nil || !ok || record.State != StateReserved || record.Missing {
		t.Fatalf("reserved identity = %#v, %v, %v", record, ok, err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkReady(ctx, "draft", path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkMissing(ctx, "draft", path); err != nil {
		t.Fatal(err)
	}
	record, ok, err = store.Get(ctx, "draft")
	if err != nil || !ok || record.State != StateMissing || !record.Missing {
		t.Fatalf("missing identity = %#v, %v, %v", record, ok, err)
	}
	if err := store.Reserve(ctx, sessionDir, candidate); !errors.Is(err, ErrSessionStateConflict) {
		t.Fatalf("missing ID was reused: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, sessionDir, []Candidate{candidate}); err != nil {
		t.Fatal(err)
	}
	record, _, err = store.Get(ctx, "draft")
	if err != nil || record.State != StateMissing {
		t.Fatalf("unreviewed file appearance recovered missing identity: %#v, %v", record, err)
	}
	if err := store.importCandidates(ctx, sessionDir, []Candidate{candidate}, true, false, false); err != nil {
		t.Fatal(err)
	}
	record, _, err = store.Get(ctx, "draft")
	if err != nil || record.State != StateReady {
		t.Fatalf("reviewed file import did not recover identity: %#v, %v", record, err)
	}
}

func TestListVisibleKeysetPaginationDoesNotLoseTiedPositions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	candidates := make([]Candidate, 209)
	for i := range candidates {
		id := fmt.Sprintf("session-%03d", i)
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		workspace := "/work/a"
		if i%2 != 0 {
			workspace = "/work/b"
		}
		candidates[i] = Candidate{ID: id, Path: path, WorkspaceRoot: workspace, Position: i / 4}
	}
	if err := store.Import(ctx, root, candidates); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sessions SET state='missing' WHERE id='session-000'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sessions SET state='deleting' WHERE id='session-001'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sessions SET state='deleted' WHERE id='session-002'`); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool, 209)
	var cursor *Cursor
	total := -1
	for {
		page, err := store.ListVisible(ctx, 73, cursor, "")
		if err != nil {
			t.Fatalf("list visible page: %v", err)
		}
		if total == -1 {
			total = page.Total
		}
		if page.Total != total {
			t.Fatalf("total changed between pages: %d then %d", total, page.Total)
		}
		for _, record := range page.Records {
			if seen[record.ID] {
				t.Fatalf("duplicate session across pages: %s", record.ID)
			}
			seen[record.ID] = true
			if record.ID == "session-001" || record.ID == "session-002" {
				t.Fatalf("hidden lifecycle state leaked into visible page: %#v", record)
			}
			if record.ID == "session-000" && (record.State != StateMissing || !record.Missing) {
				t.Fatalf("missing session projection = %#v", record)
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor
	}
	if total != 207 || len(seen) != 207 {
		t.Fatalf("visible sessions total=%d collected=%d, want 207", total, len(seen))
	}

	filtered, err := store.ListVisible(ctx, 200, nil, "/work/a")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range filtered.Records {
		if record.WorkspaceRoot != "/work/a" {
			t.Fatalf("workspace filter leaked record: %#v", record)
		}
	}
}

func TestImportRejectsPathChangeAndRollsBack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pathA := filepath.Join(root, "sessions", "a.jsonl")
	pathB := filepath.Join(root, "sessions", "b.jsonl")
	if err := os.MkdirAll(filepath.Dir(pathA), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pathA, pathB} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Import(ctx, root, []Candidate{{ID: "stable", Path: pathA}}); err != nil {
		t.Fatal(err)
	}
	change := []Candidate{{ID: "new", Path: pathB}, {ID: "stable", Path: pathB}}
	if err := store.Import(ctx, root, change); err == nil {
		t.Fatal("duplicate path accepted")
	}
	change[1].Path = filepath.Join(root, "sessions", "c.jsonl")
	if err := os.WriteFile(change[1].Path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, root, change); !errors.Is(err, ErrPathChanged) {
		t.Fatalf("path change error = %v", err)
	}
	records, err := store.List(ctx)
	if err != nil || len(records) != 1 || records[0].Path != pathA {
		t.Fatalf("partial import survived rollback: %#v, %v", records, err)
	}
}

func TestImportRejectsOutsideRootAndSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Import(ctx, root, []Candidate{{ID: "outside", Path: outside}}); err == nil {
		t.Fatal("outside transcript accepted")
	}
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := store.Import(ctx, root, []Candidate{{ID: "linked", Path: link}}); err == nil {
		t.Fatal("symlink transcript accepted")
	}
	linkedDir := filepath.Join(root, "linked-dir")
	if err := os.Symlink(filepath.Dir(outside), linkedDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if err := store.Import(ctx, root, []Candidate{{ID: "parent-linked", Path: filepath.Join(linkedDir, "outside.jsonl")}}); err == nil {
		t.Fatal("transcript outside root through a parent symlink accepted")
	}
}

func TestImportWorkbenchCatalogPreservesOrderAndWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	catalog := filepath.Join(t.TempDir(), "workbench-sessions.json")
	workspace := filepath.Join(t.TempDir(), "project")
	// The bridge writes every session flat in its session dir, whatever the
	// workspace. The importer must derive the same paths; a workspace must not
	// move a transcript into projects/<slug>/sessions.
	projectPath := filepath.Join(sessionDir, "tauri-tauri-project.jsonl")
	globalPath := filepath.Join(sessionDir, "tauri-tauri-global.jsonl")
	for _, path := range []string{globalPath, projectPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	content, err := json.Marshal([]catalogEntry{
		{SessionID: "tauri-project", Title: "Project chat", WorkspaceRoot: workspace},
		{SessionID: "tauri-global", Title: "Global chat"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ImportWorkbenchCatalog(ctx, sessionDir, catalog); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx)
	if err != nil || len(records) != 2 {
		t.Fatalf("imported catalog = %#v, %v", records, err)
	}
	// A workspace-bearing session keeps its metadata but lands beside the
	// session dir's other transcripts.
	if records[0].ID != "tauri-project" || records[0].Path != projectPath || records[0].WorkspaceRoot != workspace {
		t.Fatalf("catalog mapping = %#v", records)
	}
	if records[1].ID != "tauri-global" || records[1].Path != globalPath {
		t.Fatalf("catalog mapping = %#v", records)
	}

	// The importer and the bridge must agree for the same session ID. If this
	// fails, one of them changed its layout rule alone.
	fromBridge, err := sessionpath.TranscriptPath(sessionDir, "tauri-project")
	if err != nil {
		t.Fatal(err)
	}
	if fromBridge != records[0].Path {
		t.Fatalf("bridge path %q != imported path %q", fromBridge, records[0].Path)
	}
}

func TestOpenRejectsFutureSchemaWithoutChangingIt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(ctx, path); err == nil {
		_ = store.Close()
		t.Fatal("future schema was opened")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 99 {
		t.Fatalf("future schema version = %d, %v", version, err)
	}
}

func TestImportLegacyCatalogPreservesMissingRowsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-legacy-missing.jsonl")
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	candidates := []Candidate{{ID: "legacy-missing", Path: path, Title: "Old title", Position: 0}}
	if err := store.ImportLegacyCatalog(ctx, sessionDir, candidates); err != nil {
		t.Fatal(err)
	}
	if err := store.ImportLegacyCatalog(ctx, sessionDir, []Candidate{{ID: "legacy-missing", Path: path, Title: "Stale title", Position: 1}}); err != nil {
		t.Fatal(err)
	}
	record, ok, err := store.Get(ctx, "legacy-missing")
	if err != nil || !ok {
		t.Fatalf("legacy record lookup: ok=%v err=%v", ok, err)
	}
	if record.State != StateMissing || !record.Missing || record.Title != "Old title" || record.Position != 0 {
		t.Fatalf("legacy missing row was lost or overwritten: %#v", record)
	}
	if err := store.ImportLegacyCatalog(ctx, sessionDir, make([]Candidate, 51)); err == nil {
		t.Fatal("oversized legacy catalog was accepted")
	}
}

func TestTitleProvenanceAndRepeatImport(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-title.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	entry := Candidate{ID: "title", Path: path, Title: "Catalog title"}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx)
	if err != nil || records[0].TitleSource != TitleLegacyUnknown {
		t.Fatalf("imported title provenance = %#v, %v", records, err)
	}
	if err := store.SetTitle(ctx, "title", 0, "Automatic", TitleAutomaticGeneration); !errors.Is(err, ErrTitleProtected) {
		t.Fatalf("automatic rename of legacy title = %v", err)
	}
	if err := store.SetTitle(ctx, "title", 0, "Manual", TitleManualRename); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTitle(ctx, "title", 0, "Stale", TitleManualRename); !errors.Is(err, ErrTitleConflict) {
		t.Fatalf("stale rename = %v", err)
	}
	if err := store.Import(ctx, root, []Candidate{entry}); err != nil {
		t.Fatal(err)
	}
	records, err = store.List(ctx)
	if err != nil || records[0].Title != "Manual" || records[0].TitleSource != TitleUser || records[0].TitleRevision != 1 {
		t.Fatalf("repeat import overwrote rename = %#v, %v", records, err)
	}
	if err := store.SetTitle(ctx, "title", 1, "Background", TitleAutomaticGeneration); !errors.Is(err, ErrTitleProtected) {
		t.Fatalf("automatic rename of manual title = %v", err)
	}
	if err := store.SetTitle(ctx, "title", 1, "Requested AI title", TitleUserRequestedGeneration); err != nil {
		t.Fatal(err)
	}
	records, err = store.List(ctx)
	if err != nil || records[0].Title != "Requested AI title" || records[0].TitleSource != TitleUser || records[0].TitleRevision != 2 {
		t.Fatalf("user-requested AI rename = %#v, %v", records, err)
	}
}

func TestFallbackTitleCanBeDerivedThenGenerated(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-fallback.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Import(ctx, root, []Candidate{{ID: "fallback", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTitle(ctx, "fallback", 0, "First message", TitleFirstMessage); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTitle(ctx, "fallback", 1, "Another first message", TitleFirstMessage); !errors.Is(err, ErrTitleProtected) {
		t.Fatalf("second first-message title = %v", err)
	}
	if err := store.SetTitle(ctx, "fallback", 1, "Generated", TitleAutomaticGeneration); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx)
	if err != nil || records[0].TitleSource != TitleGenerated || records[0].TitleRevision != 2 {
		t.Fatalf("generated fallback title = %#v, %v", records, err)
	}
}

func TestConcurrentTitleWritersCannotBothCommitSameRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "tauri-race.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "identity.sqlite")
	first, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if err := first.Import(ctx, root, []Candidate{{ID: "race", Path: path}}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		go func(store *Store) {
			<-start
			results <- store.SetTitle(ctx, "race", 0, "Renamed", TitleManualRename)
		}(store)
	}
	close(start)
	a, b := <-results, <-results
	if !((a == nil && errors.Is(b, ErrTitleConflict)) || (b == nil && errors.Is(a, ErrTitleConflict))) {
		t.Fatalf("concurrent title updates = %v, %v; want one success and one conflict", a, b)
	}
	records, err := first.List(ctx)
	if err != nil || len(records) != 1 || records[0].TitleRevision != 1 {
		t.Fatalf("concurrent title result = %#v, %v", records, err)
	}
}

func TestOpenMigratesV1TitlesWithoutAssumingUserIntent(t *testing.T) {
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
		id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, workspace_root TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0,
		missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),
		created_at_ms INTEGER NOT NULL, updated_at_ms INTEGER NOT NULL);
		INSERT INTO sessions VALUES ('named', '` + filepath.ToSlash(filepath.Join(root, "sessions", "a.jsonl")) + `', '', 'Legacy title', 0, 0, 1, 1);
		INSERT INTO sessions VALUES ('empty', '` + filepath.ToSlash(filepath.Join(root, "sessions", "b.jsonl")) + `', '', '', 1, 1, 1, 1);
		PRAGMA user_version=1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	records, err := store.List(ctx)
	if err != nil || len(records) != 2 || records[0].TitleSource != TitleLegacyUnknown || records[1].TitleSource != TitleFallback {
		t.Fatalf("migrated provenance = %#v, %v", records, err)
	}
	var version int
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatalf("migrated version = %d, %v", version, err)
	}
	if records[0].State != StateReady || records[1].State != StateMissing || !records[1].Missing {
		t.Fatalf("migrated lifecycle states = %#v", records)
	}
	if got, want := records[1].Path, filepath.Join(root, "sessions", "b.jsonl"); got != want {
		t.Fatalf("v1 migrated absolute path = %q, want %q", got, want)
	}
	var relative string
	if err := store.db.QueryRowContext(ctx, "SELECT relative_path FROM sessions WHERE id='empty'").Scan(&relative); err != nil || relative != "sessions/b.jsonl" {
		t.Fatalf("v1 stored relative path = %q, %v", relative, err)
	}
}

func TestOpenMigratesV2MissingFlagToLifecycleState(t *testing.T) {
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
		id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, workspace_root TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '', title_source TEXT NOT NULL DEFAULT 'fallback',
		title_revision INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0,
		missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),
		created_at_ms INTEGER NOT NULL, updated_at_ms INTEGER NOT NULL);
		INSERT INTO sessions VALUES ('ready', '` + filepath.ToSlash(filepath.Join(root, "sessions", "ready.jsonl")) + `', '', '', 'fallback', 0, 0, 0, 1, 1);
		INSERT INTO sessions VALUES ('missing', '` + filepath.ToSlash(filepath.Join(root, "sessions", "missing.jsonl")) + `', '', '', 'fallback', 0, 1, 1, 1, 1);
		PRAGMA user_version=2`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	records, err := store.List(ctx)
	if err != nil || len(records) != 2 || records[0].State != StateReady || records[1].State != StateMissing {
		t.Fatalf("v2 migration records = %#v, %v", records, err)
	}
	var version int
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatalf("migrated schema version = %d, %v", version, err)
	}
}

func TestV3PathMigrationRejectsPathsOutsideProfileWithoutChangingDatabase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "identity.sqlite")
	external := filepath.Join(t.TempDir(), "sessions", "tauri-outside.jsonl")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, workspace_root TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '', title_source TEXT NOT NULL DEFAULT 'fallback',
		title_revision INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0,
		state TEXT NOT NULL DEFAULT 'ready', created_at_ms INTEGER NOT NULL, updated_at_ms INTEGER NOT NULL);
		INSERT INTO sessions VALUES ('outside', '` + filepath.ToSlash(external) + `', '', '', 'fallback', 0, 0, 'ready', 1, 1);
		CREATE INDEX sessions_state_position_id ON sessions(state, position, id);
		PRAGMA user_version=3`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(ctx, dbPath, root); err == nil {
		_ = store.Close()
		t.Fatal("migration accepted a transcript path outside the supplied profile")
	}
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	var storedPath string
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("failed migration changed schema version to %d: %v", version, err)
	}
	if err := db.QueryRow("SELECT path FROM sessions WHERE id='outside'").Scan(&storedPath); err != nil || filepath.ToSlash(storedPath) != filepath.ToSlash(external) {
		t.Fatalf("failed migration changed stored path to %q: %v", storedPath, err)
	}
}
