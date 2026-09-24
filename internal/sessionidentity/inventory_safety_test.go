package sessionidentity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryNeverOffersSymlinksForImport(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writeTranscript(t, outside)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(sessionDir, "tauri-tauri-linked.jsonl")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-linked"}})
	report, err := Inventory(context.Background(), nil, sessionDir, catalog)
	if err != nil {
		t.Fatal(err)
	}
	row := findEntry(t, report, "tauri-linked")
	if row.Claim == Claimable || row.Claim == ClaimUnclaimed {
		t.Fatalf("symlink was offered for import: %#v", row)
	}
}

func TestInventoryRejectsNameThatDoesNotRoundTripToBridgePath(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "other.jsonl"))
	report, err := Inventory(context.Background(), nil, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	row := findEntry(t, report, "other")
	if row.Claim != ClaimInvalidFile || len(report.Unclaimed) != 0 {
		t.Fatalf("non-round-tripping file name was offered: %#v", report)
	}
}

func TestInventoryDoesNotOfferDuplicateCatalogIDs(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-duplicate.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{
		{SessionID: "tauri-duplicate", Title: "First"},
		{SessionID: "tauri-duplicate", Title: "Second"},
	})
	report, err := Inventory(context.Background(), nil, sessionDir, catalog)
	if err != nil {
		t.Fatal(err)
	}
	claimable := 0
	for _, row := range report.Entries {
		if row.ID == "tauri-duplicate" && row.Claim == Claimable {
			claimable++
		}
	}
	if claimable > 0 || len(report.Errors) == 0 {
		t.Fatalf("duplicate catalog ID was offered: %#v", report)
	}
}

func TestInventoryDoesNotHideAFileWithTheSameNameInAnotherDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "tauri-tauri-same.jsonl")
	currentPath := filepath.Join(root, "sessions", "tauri-tauri-same.jsonl")
	writeTranscript(t, oldPath)
	writeTranscript(t, currentPath)
	identities, err := Open(ctx, filepath.Join(root, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	if err := identities.Import(ctx, root, []Candidate{{ID: "tauri-same", Path: oldPath}}); err != nil {
		t.Fatal(err)
	}
	report, err := Inventory(ctx, identities, filepath.Dir(currentPath), "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range report.Entries {
		if row.Path == currentPath && row.Claim == ClaimPathChanged {
			found = true
		}
	}
	if !found || len(report.Errors) == 0 {
		t.Fatalf("same-ID file in current directory was hidden: %#v", report)
	}
}
