package sessionidentity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/desktopbridge/sessionpath"
)

func TestInventoryUsesBoundOriginalLegacyPathAfterWorkbenchRemembersIt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "projects", "中文-project", "sessions", "old-topic.jsonl")
	writeTranscript(t, path)
	id, err := sessionpath.LegacyID(root, path)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	if err := identities.Import(ctx, root, []Candidate{{ID: id, Path: path, WorkspaceRoot: "/project", Title: "original topic"}}); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(t.TempDir(), "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: id, WorkspaceRoot: "/project", Title: "original topic"}})
	report, err := Inventory(ctx, identities, sessionDir, catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Entries {
		if row.ID == id && (row.Path != path || row.Claim != ClaimRegistered || !row.Exists) {
			t.Fatalf("original path lost: %+v", row)
		}
	}
	if len(report.Errors) != 0 || len(report.Unclaimed) != 0 {
		t.Fatalf("false mismatch: %+v", report)
	}
	// Altering the stored path must not turn another file into this identity.
	other := filepath.Join(root, "projects", "中文-project", "sessions", "other.jsonl")
	writeTranscript(t, other)
	rel, err := filepath.Rel(root, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identities.db.ExecContext(ctx, "UPDATE sessions SET relative_path=? WHERE id=?", filepath.ToSlash(rel), id); err != nil {
		t.Fatal(err)
	}
	report, err = Inventory(ctx, identities, sessionDir, catalog)
	if err != nil {
		t.Fatal(err)
	}
	var rejected bool
	for _, row := range report.Entries {
		if row.ID == id && row.Source == InventoryFromWorkbench && row.Claim == ClaimPathChanged {
			rejected = true
		}
	}
	if !rejected {
		t.Fatal("changed legacy path was accepted")
	}
}
