package sessionidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

func TestEmptyInventorySerializesExplicitEmptyArrays(t *testing.T) {
	sessionDir := t.TempDir()
	report, err := Inventory(context.Background(), nil, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"entries", "unclaimed", "errors"} {
		if got := string(payload[field]); got != "[]" {
			t.Errorf("empty inventory %s = %s, want []", field, got)
		}
	}
}

func TestInventoryVisibleSnapshotMatchesStandaloneDirectorySnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	candidates := make([]Candidate, 0, 4)
	for index, id := range []string{"first", "second", "missing", "deleting"} {
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		writeTranscript(t, path)
		candidates = append(candidates, Candidate{ID: id, Path: path, Title: "Title " + id, Position: index})
	}
	if err := identities.Import(ctx, sessionDir, candidates); err != nil {
		t.Fatal(err)
	}
	for id, state := range map[string]SessionState{"missing": StateMissing, "deleting": StateDeleting} {
		if _, err := identities.db.ExecContext(ctx, "UPDATE sessions SET state=? WHERE id=?", state, id); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	combined, err := report.VisibleSnapshot(MaxVisibleSnapshotSize)
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := identities.ListVisibleSnapshot(ctx, MaxVisibleSnapshotSize, "")
	if err != nil {
		t.Fatal(err)
	}
	if combined.SnapshotID != standalone.SnapshotID || combined.Total != standalone.Total ||
		!reflect.DeepEqual(combined.Records, standalone.Records) {
		t.Fatalf("inventory directory snapshot = %#v, want %#v", combined, standalone)
	}
	if _, err := report.VisibleSnapshot(1); err == nil {
		t.Fatal("bounded inventory snapshot silently truncated visible identities")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"registered":[`) || strings.Contains(string(encoded), "relativePath") {
		t.Fatalf("inventory serialized internal snapshot rows: %s", encoded)
	}
}

func TestInventoryAuditsLegacyPhysicalTranscriptAliases(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-shared.jsonl"))
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}

	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	// Seed a legacy duplicate directly: current Import correctly refuses to
	// create this state, but inventory must be able to audit an older database.
	for position, identity := range []struct{ id, path string }{
		{id: "legacy-first", path: filepath.Join(sessionDir, "tauri-shared.jsonl")},
		{id: "legacy-second", path: filepath.Join(aliasDir, "tauri-shared.jsonl")},
	} {
		relative, err := relativeTranscriptPath(root, identity.id, identity.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, identity.id, relative, position); err != nil {
			t.Fatal(err)
		}
	}

	report, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy-first", "legacy-second"} {
		entry := findEntry(t, report, id)
		if entry.Claim != ClaimPathConflict || !strings.Contains(entry.Detail, "physical transcript") {
			t.Fatalf("legacy physical alias %q not flagged: %#v", id, entry)
		}
	}
	if len(report.Errors) == 0 {
		t.Fatal("physical alias audit did not add an inventory error")
	}
}

func TestInventoryAuditsLegacyHardLinkedTranscripts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	firstPath := filepath.Join(sessionDir, "tauri-first.jsonl")
	secondPath := filepath.Join(sessionDir, "tauri-second.jsonl")
	writeTranscript(t, firstPath)
	if err := os.Link(firstPath, secondPath); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	for position, identity := range []struct{ id, path string }{
		{id: "legacy-first", path: firstPath},
		{id: "legacy-second", path: secondPath},
	} {
		relative, err := relativeTranscriptPath(root, identity.id, identity.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, identity.id, relative, position); err != nil {
			t.Fatal(err)
		}
	}
	first, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("hard-link audit is not repeatable: %#v then %#v", first, second)
	}
	for _, id := range []string{"legacy-first", "legacy-second"} {
		entry := findEntry(t, first, id)
		if entry.Claim != ClaimPathConflict || !strings.Contains(entry.Detail, "physical transcript") {
			t.Fatalf("legacy hard link %q not flagged: %#v", id, entry)
		}
	}
}

func TestInventoryAuditsLateHardLinkAmongManyDistinctFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	for index := range 64 {
		id := fmt.Sprintf("peer-%02d", index)
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		writeTranscript(t, path)
		if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, id, filepath.ToSlash(filepath.Join("sessions", filepath.Base(path))), index); err != nil {
			t.Fatal(err)
		}
	}
	aliasPath := filepath.Join(sessionDir, "tauri-late-alias.jsonl")
	if err := os.Link(filepath.Join(sessionDir, "tauri-peer-00.jsonl"), aliasPath); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
		(id, relative_path, position, state, created_at_ms, updated_at_ms)
		VALUES ('late-alias', 'sessions/tauri-late-alias.jsonl', 64, 'ready', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	report, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"peer-00", "late-alias"} {
		entry := findEntry(t, report, id)
		if entry.Claim != ClaimPathConflict {
			t.Fatalf("late hard-link owner %s was not flagged: %#v", id, entry)
		}
	}
	if entry := findEntry(t, report, "peer-63"); entry.Claim != ClaimRegistered {
		t.Fatalf("unrelated identity was flagged: %#v", entry)
	}
}

func TestFoldedResolvedPathMatchesUnicodeSimpleFold(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("conservative case folding is enabled only on macOS and Windows")
	}
	for _, pair := range [][2]string{{"K", "K"}, {"Σ", "ς"}, {"A", "a"}} {
		if !strings.EqualFold(pair[0], pair[1]) {
			t.Fatalf("invalid EqualFold fixture: %q and %q", pair[0], pair[1])
		}
		if foldedResolvedPath(pair[0]) != foldedResolvedPath(pair[1]) {
			t.Fatalf("EqualFold paths received different buckets: %q and %q", pair[0], pair[1])
		}
	}
}

func TestInventoryAuditsCaseOnlyPathAliasesOnCaseInsensitivePlatforms(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("conservative case-folding is enabled only on macOS and Windows")
	}
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	for position, identity := range []struct{ id, name string }{
		{id: "case-first", name: "tauri-Session.jsonl"},
		{id: "case-second", name: "tauri-session.jsonl"},
	} {
		relative := filepath.ToSlash(filepath.Join("sessions", identity.name))
		if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'reserved', 0, 0)`, identity.id, relative, position); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Inventory(ctx, identities, sessionDir, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"case-first", "case-second"} {
		entry := findEntry(t, report, id)
		if entry.Claim != ClaimPathConflict {
			t.Fatalf("case-only legacy alias %q not flagged: %#v", id, entry)
		}
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
