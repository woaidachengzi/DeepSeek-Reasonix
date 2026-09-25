package sessionidentity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImportWorkbenchCatalogRejectsRawInvalidMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, test := range []struct {
		name, title, workspace string
	}{
		{name: "title", title: strings.Repeat("a", 121)},
		{name: "workspace before trimming", workspace: "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := filepath.Join(root, "workbench-sessions.json")
			encoded, err := json.Marshal([]catalogEntry{{SessionID: "legacy", Title: test.title, WorkspaceRoot: test.workspace}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(catalog, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := store.ImportWorkbenchCatalog(ctx, sessionDir, catalog); err == nil {
				t.Fatal("invalid workbench catalog metadata was imported")
			}
			if _, exists, err := store.Get(ctx, "legacy"); err != nil || exists {
				t.Fatalf("invalid catalog registered an identity: exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestSyncWorkbenchOrderRejectsControlCharactersBeforeMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join(sessionDir, "tauri-one.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, sessionDir, []Candidate{{ID: "one", Path: path, WorkspaceRoot: "/original"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncWorkbenchOrder(ctx, sessionDir, []WorkbenchOrderEntry{{
		ID: "one", WorkspaceRoot: "/project\nother",
	}}); err == nil {
		t.Fatal("invalid workspace metadata was synchronized")
	}
	record, exists, err := store.Get(ctx, "one")
	if err != nil || !exists || record.WorkspaceRoot != "/original" {
		t.Fatalf("invalid sync mutated identity: %#v exists=%v err=%v", record, exists, err)
	}
}

func TestSyncWorkbenchOrderUpdatesListedMetadataAndPreservesHiddenRows(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	candidates := make([]Candidate, 0, 4)
	for position, id := range []string{"a", "b", "hidden-c", "hidden-d"} {
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, Candidate{
			ID: id, Path: path, WorkspaceRoot: "/old/project", Title: "Title " + id, Position: position,
		})
	}
	if err := store.Import(ctx, sessionDir, candidates); err != nil {
		t.Fatal(err)
	}

	count, err := store.SyncWorkbenchOrder(ctx, sessionDir, []WorkbenchOrderEntry{
		{ID: "b", WorkspaceRoot: "/new/project"},
		{ID: "a"},
	})
	if err != nil || count != 2 {
		t.Fatalf("sync count=%d err=%v", count, err)
	}
	records, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	if !reflect.DeepEqual(ids, []string{"b", "a", "hidden-c", "hidden-d"}) {
		t.Fatalf("synced order = %#v", ids)
	}
	if records[0].Position != 0 || records[0].WorkspaceRoot != "/new/project" || records[0].Title != "Title b" {
		t.Fatalf("listed metadata = %#v", records[0])
	}
	if records[1].Position != 1 || records[1].WorkspaceRoot != "" || records[1].Title != "Title a" {
		t.Fatalf("cleared project metadata = %#v", records[1])
	}
	if records[2].Position != 2 || records[2].WorkspaceRoot != "/old/project" || records[3].Position != 3 {
		t.Fatalf("hidden rows were not preserved after the host snapshot: %#v", records[2:])
	}
	count, err = store.SyncWorkbenchOrder(ctx, sessionDir, []WorkbenchOrderEntry{
		{ID: "b", WorkspaceRoot: "/new/project"},
		{ID: "a"},
	})
	if err != nil || count != 2 {
		t.Fatalf("idempotent sync count=%d err=%v", count, err)
	}
}

func TestSyncWorkbenchOrderRejectsDuplicateBeforeMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(sessionDir, "tauri-one.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Import(ctx, sessionDir, []Candidate{{ID: "one", Path: path, WorkspaceRoot: "/before", Position: 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncWorkbenchOrder(ctx, sessionDir, []WorkbenchOrderEntry{
		{ID: "one", WorkspaceRoot: "/after"}, {ID: "one", WorkspaceRoot: "/other"},
	}); err == nil {
		t.Fatal("duplicate catalog entries were accepted")
	}
	record, exists, err := store.Get(ctx, "one")
	if err != nil || !exists || record.WorkspaceRoot != "/before" || record.Position != 0 {
		t.Fatalf("invalid sync mutated identity: %#v exists=%v err=%v", record, exists, err)
	}
}
