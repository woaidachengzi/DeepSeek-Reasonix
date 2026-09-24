package sessionidentity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeCatalog(t *testing.T, path string, entries []catalogEntry) {
	t.Helper()
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTranscript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findEntry(t *testing.T, report InventoryReport, id string) InventoryEntry {
	t.Helper()
	for _, entry := range report.Entries {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("inventory has no row for %q: %#v", id, report.Entries)
	return InventoryEntry{}
}

// The listing must run before anything is registered, so it may not create the
// database: reading a listing must never bring an identity store into
// existence as a side effect.
func TestInventoryDoesNotCreateTheDatabase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-tauri-only.jsonl")
	writeTranscript(t, transcript)
	dbPath := filepath.Join(root, "desktop", "state.sqlite")

	// A profile with no identity database is the common case before any import,
	// so the listing must work from a nil store and leave the path untouched.
	report, err := Inventory(ctx, nil, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Claim != ClaimUnclaimed {
		t.Fatalf("listing without a store = %#v", report.Entries)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("inventory created an identity database: %v", err)
	}
}

// Requirement: a file the host never recorded stays a candidate. Reporting it
// must not claim it.
func TestInventoryListsUnclaimedFilesWithoutClaimingThem(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	orphan := filepath.Join(sessionDir, "tauri-tauri-orphan.jsonl")
	writeTranscript(t, orphan)
	catalogPath := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalogPath, []catalogEntry{{SessionID: "tauri-known", Title: "Known"}})

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	report, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	orphanRow := findEntry(t, report, "tauri-orphan")
	if orphanRow.Claim != ClaimUnclaimed {
		t.Fatalf("orphan claim = %q, want %q", orphanRow.Claim, ClaimUnclaimed)
	}
	if orphanRow.Source != InventoryFromScan {
		t.Fatalf("orphan source = %q", orphanRow.Source)
	}
	if !orphanRow.Exists || orphanRow.Registered {
		t.Fatalf("orphan row = %#v", orphanRow)
	}
	if len(report.Unclaimed) != 1 || report.Unclaimed[0] != orphan {
		t.Fatalf("unclaimed = %#v", report.Unclaimed)
	}
	// The known catalog entry is not a transcript yet, so it is a missing file
	// rather than a claimable registration.
	knownRow := findEntry(t, report, "tauri-known")
	if knownRow.Claim != ClaimMissingFile || knownRow.Exists {
		t.Fatalf("known row = %#v", knownRow)
	}
	// Nothing was written: the store still lists no sessions.
	records, err := store.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("inventory registered sessions: %#v, %v", records, err)
	}
}

// Requirement: every row states the actual path, so a reviewer can compare the
// list against the disk.
func TestInventoryReportsTheActualPathForEveryRow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	workspace := filepath.Join(t.TempDir(), "project")
	present := filepath.Join(sessionDir, "tauri-tauri-present.jsonl")
	writeTranscript(t, present)
	absent := filepath.Join(sessionDir, "tauri-tauri-absent.jsonl")
	catalogPath := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalogPath, []catalogEntry{
		{SessionID: "tauri-present", Title: "Present", WorkspaceRoot: workspace},
		{SessionID: "tauri-absent", Title: "Absent", WorkspaceRoot: workspace},
	})

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	report, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	presentRow := findEntry(t, report, "tauri-present")
	if presentRow.Path != present || !presentRow.Exists || presentRow.Claim != Claimable {
		t.Fatalf("present row = %#v", presentRow)
	}
	if presentRow.Title != "Present" || presentRow.WorkspaceRoot != workspace {
		t.Fatalf("present row metadata = %#v", presentRow)
	}
	absentRow := findEntry(t, report, "tauri-absent")
	if absentRow.Path != absent || absentRow.Exists || absentRow.Claim != ClaimMissingFile {
		t.Fatalf("absent row = %#v", absentRow)
	}
	// The workspace must never move a transcript into projects/<slug>/.
	if filepath.Dir(presentRow.Path) != sessionDir {
		t.Fatalf("transcript left the session directory: %s", presentRow.Path)
	}
}

// Requirement: a registered session whose file disappeared is reported, never
// presented as fresh work.
func TestInventoryFlagsARegisteredSessionWhoseFileIsGone(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	path := filepath.Join(sessionDir, "tauri-tauri-gone.jsonl")
	writeTranscript(t, path)
	catalogPath := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalogPath, []catalogEntry{{SessionID: "tauri-gone", Title: "Gone"}})

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ImportWorkbenchCatalog(ctx, sessionDir, catalogPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	report, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	row := findEntry(t, report, "tauri-gone")
	if row.Exists {
		t.Fatalf("row claims the transcript exists: %#v", row)
	}
	if row.Claim != ClaimRegistered && row.Claim != ClaimMissingFile {
		t.Fatalf("row claim = %q", row.Claim)
	}
	if row.Detail == "" {
		t.Fatalf("row does not explain the missing transcript: %#v", row)
	}
	if !row.Registered {
		t.Fatalf("row must stay registered: %#v", row)
	}
}

// Session sidecars share the transcript stem; they are not conversations.
func TestInventoryIgnoresSessionSidecars(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-one.jsonl"))
	for _, sidecar := range []string{
		"tauri-tauri-one.events.jsonl",
		"tauri-tauri-one.turns.jsonl",
		"tauri-tauri-one.conflicts.jsonl",
		"tauri-tauri-one.guardian.jsonl",
	} {
		writeTranscript(t, filepath.Join(sessionDir, sidecar))
	}

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	report, err := Inventory(ctx, store, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("sidecars leaked into the inventory: %#v", report.Entries)
	}
	if len(report.Unclaimed) != 1 {
		t.Fatalf("unclaimed = %#v", report.Unclaimed)
	}
}

// A name that yields no safe session ID is reported, not silently dropped and
// never registered.
func TestInventoryReportsFilesThatYieldNoSessionID(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-has space.jsonl"))

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	report, err := Inventory(ctx, store, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	row := findEntry(t, report, "has space")
	if row.Claim != ClaimInvalidFile {
		t.Fatalf("row = %#v", row)
	}
	if len(report.Errors) == 0 {
		t.Fatal("an unusable transcript name must be reported")
	}
	if len(report.Unclaimed) != 0 {
		t.Fatalf("an unusable name must not be offered as a candidate: %#v", report.Unclaimed)
	}
}

// A catalog entry whose derived path is already owned by another session is
// reported as a conflict instead of being silently claimable.
func TestInventoryFlagsACatalogPathOwnedByAnotherSession(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	occupied := filepath.Join(sessionDir, "tauri-tauri-shared.jsonl")
	writeTranscript(t, occupied)

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Import(ctx, sessionDir, []Candidate{
		{ID: "tauri-shared", Path: occupied},
	}); err != nil {
		t.Fatal(err)
	}

	// A file name recovers exactly one ID, so the conflicting entry is one whose
	// ID maps to the same transcript as the registered session.
	catalogPath := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalogPath, []catalogEntry{{SessionID: "tauri-shared", Title: "Owner"}})
	report, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	owner := findEntry(t, report, "tauri-shared")
	if owner.Claim != ClaimRegistered || !owner.Registered {
		t.Fatalf("owner row = %#v", owner)
	}
	if len(report.Unclaimed) != 0 {
		t.Fatalf("a registered transcript was offered as unclaimed: %#v", report.Unclaimed)
	}
}

// Two inventories over an unchanged directory must agree, or the listing cannot
// be used to approve an import.
func TestInventoryIsRepeatable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-one.jsonl"))
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-two.jsonl"))
	catalogPath := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalogPath, []catalogEntry{{SessionID: "tauri-three", Title: "Missing"}})

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Inventory(ctx, store, sessionDir, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != len(second.Entries) {
		t.Fatalf("row count changed: %d then %d", len(first.Entries), len(second.Entries))
	}
	for i := range first.Entries {
		if first.Entries[i] != second.Entries[i] {
			t.Fatalf("row %d changed:\n%#v\n%#v", i, first.Entries[i], second.Entries[i])
		}
	}
	// The imported catalog wrote nothing, so both runs describe only the disk.
	records, err := store.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("inventory wrote identities: %#v, %v", records, err)
	}
}

// Phase 5 has to stop `resumeBridgeSession` from recreating a deleted session
// as an empty one. That decision needs the identity store, so this test only
// pins the evidence it will rely on: deleting the transcript leaves the session
// sidecars behind, which is what tells a missing session apart from a new one.
func TestDeletingATranscriptLeavesSidecarEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-tauri-residue.jsonl")
	writeTranscript(t, transcript)
	// A registered session always owns a metadata sidecar once it is titled,
	// and the bridge creates the inbox when it opens the session.
	metaPath := transcript + ".meta"
	if err := os.WriteFile(metaPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sessionDir, "tauri-tauri-residue.inbox"), 0o700); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Import(ctx, sessionDir, []Candidate{{ID: "tauri-residue", Path: transcript}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(transcript); err != nil {
		t.Fatal(err)
	}

	report, err := Inventory(ctx, store, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	row := findEntry(t, report, "tauri-residue")
	if row.Exists {
		t.Fatalf("row claims a deleted transcript exists: %#v", row)
	}
	if row.Claim != ClaimRegistered {
		t.Fatalf("row claim = %q, want a registered-and-missing row", row.Claim)
	}
	// The residue is the evidence a later phase uses to refuse a silent reopen.
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("expected the metadata sidecar to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "tauri-tauri-residue.inbox")); err != nil {
		t.Fatalf("expected the inbox to remain: %v", err)
	}
}
