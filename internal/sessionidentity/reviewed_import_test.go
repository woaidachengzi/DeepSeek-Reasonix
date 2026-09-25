package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/profilegate"
)

func TestReviewedImportIsSelectedTransactionalAndReplayable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	firstPath := filepath.Join(sessionDir, "tauri-tauri-first.jsonl")
	otherPath := filepath.Join(sessionDir, "tauri-tauri-other.jsonl")
	unclaimedPath := filepath.Join(sessionDir, "tauri-tauri-unclaimed.jsonl")
	for _, path := range []string{firstPath, otherPath, unclaimedPath} {
		writeTranscript(t, path)
	}
	sidecar := firstPath + ".meta"
	sidecarBytes := []byte("{\"customTitle\":\"legacy\"}\n")
	if err := os.WriteFile(sidecar, sidecarBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{
		{SessionID: "tauri-first", Title: "First", WorkspaceRoot: "/work/first"},
		{SessionID: "tauri-other", Title: "Other"},
	})
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-first"})
	if err != nil || len(plan.Rows) != 1 || plan.Rows[0].ID != "tauri-first" {
		t.Fatalf("review plan = %#v, %v", plan, err)
	}
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	release, err := profilegate.TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyImportReview(ctx, plan); !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("apply while profile is owned = %v, want profilegate.ErrHeld", err)
	}
	release()
	before, err := store.List(ctx)
	if err != nil || len(before) != 0 {
		t.Fatalf("profile lock rejection changed identities: %#v, %v", before, err)
	}
	result, err := store.ApplyImportReview(ctx, plan)
	if err != nil || result.Applied != 1 || len(result.Errors) != 0 {
		t.Fatalf("apply result = %#v, %v", result, err)
	}
	first, err := store.List(ctx)
	if err != nil || len(first) != 1 || first[0].ID != "tauri-first" || first[0].TitleSource != TitleLegacyUnknown {
		t.Fatalf("imported identities = %#v, %v", first, err)
	}
	result, err = store.ApplyImportReview(ctx, plan)
	if err != nil || result.Applied != 1 || len(result.Errors) != 0 {
		t.Fatalf("review replay result = %#v, %v", result, err)
	}
	second, err := store.List(ctx)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("review replay changed identities: %#v, %v", second, err)
	}
	got, err := os.ReadFile(sidecar)
	if err != nil || !reflect.DeepEqual(got, sidecarBytes) {
		t.Fatalf("reviewed import changed sidecar: %q, %v", got, err)
	}
	for _, path := range []string{firstPath, otherPath, unclaimedPath} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "{}\n" {
			t.Fatalf("reviewed import changed transcript %s: %q, %v", path, got, err)
		}
	}
}

func TestReviewedImportRejectsStaleTranscriptWithoutPartialRows(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	firstPath := filepath.Join(sessionDir, "tauri-tauri-first.jsonl")
	secondPath := filepath.Join(sessionDir, "tauri-tauri-second.jsonl")
	writeTranscript(t, firstPath)
	writeTranscript(t, secondPath)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-first"}, {SessionID: "tauri-second"}})
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-first", "tauri-second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("{\"changed\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("stale transcript import = %v", err)
	}
	records, err := store.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("partial import after stale transcript: %#v, %v", records, err)
	}
}

func TestReviewedImportRejectsCatalogChangeAndUnclaimedSelection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-known.jsonl"))
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-unclaimed.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-known", Title: "Old"}})
	if _, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-unclaimed"}); err == nil {
		t.Fatal("unclaimed scan result was selectable for import")
	}
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-known"})
	if err != nil {
		t.Fatal(err)
	}
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-known", Title: "New"}})
	store, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("stale catalog import = %v", err)
	}
	records, err := store.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("catalog change produced identities: %#v, %v", records, err)
	}
}

func TestReviewedImportRejectsUnrelatedRegisteredPathConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	oldPath := filepath.Join(root, "old", "tauri-tauri-moved.jsonl")
	writeTranscript(t, oldPath)
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-good.jsonl"))
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-moved.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-good"}, {SessionID: "tauri-moved"}})
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	if err := identities.Import(ctx, root, []Candidate{{ID: "tauri-moved", Path: oldPath}}); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareImportReview(ctx, identities, sessionDir, catalog, []string{"tauri-good"}); err == nil {
		t.Fatal("unrelated registered path conflict did not block the batch")
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "tauri-moved" {
		t.Fatalf("conflicted review changed identities: %#v, %v", records, err)
	}
}

func TestReviewedImportRejectsLegacyPhysicalPathConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-good.jsonl")
	writeTranscript(t, transcript)
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "good"}})
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	legacyPath := filepath.Join(aliasDir, filepath.Base(transcript))
	relative, err := relativeTranscriptPath(root, "legacy-owner", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
		(id, relative_path, position, state, created_at_ms, updated_at_ms)
		VALUES ('legacy-owner', ?, 0, 'ready', 0, 0)`, relative); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareImportReview(ctx, identities, sessionDir, catalog, []string{"good"}); err == nil {
		t.Fatal("review accepted a transcript already owned through a physical path alias")
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "legacy-owner" {
		t.Fatalf("conflicted review changed identities: %#v, %v", records, err)
	}
}

func TestApplyImportReviewRejectsPhysicalConflictAddedAfterReview(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-good.jsonl")
	writeTranscript(t, transcript)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "good"}})
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"good"})
	if err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	legacyPath := filepath.Join(aliasDir, filepath.Base(transcript))
	relative, err := relativeTranscriptPath(root, "legacy-owner", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identities.db.ExecContext(ctx, `INSERT INTO sessions
		(id, relative_path, position, state, created_at_ms, updated_at_ms)
		VALUES ('legacy-owner', ?, 0, 'ready', 0, 0)`, relative); err != nil {
		t.Fatal(err)
	}
	if _, err := identities.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("apply after physical conflict = %v, want ErrImportReviewChanged", err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "legacy-owner" {
		t.Fatalf("stale review registered a conflicting identity: %#v, %v", records, err)
	}
}

func TestReviewedImportKeepsDistinctIDsForIdenticalTranscripts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-one.jsonl"))
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-two.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}, {SessionID: "tauri-two"}})
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-one", "tauri-two"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rows[0].TranscriptSHA256 != plan.Rows[1].TranscriptSHA256 {
		t.Fatal("test transcripts unexpectedly differ")
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	if _, err := identities.ApplyImportReview(ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 2 || records[0].ID == records[1].ID || records[0].Path == records[1].Path {
		t.Fatalf("identical content collapsed identity: %#v, %v", records, err)
	}
}

func TestReviewedImportStrictRegistrationNeverSkipsSelectedMissingFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	present := filepath.Join(sessionDir, "tauri-tauri-present.jsonl")
	missing := filepath.Join(sessionDir, "tauri-tauri-missing.jsonl")
	writeTranscript(t, present)
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	err = identities.importCandidates(ctx, sessionDir, []Candidate{
		{ID: "tauri-present", Path: present},
		{ID: "tauri-missing", Path: missing},
	}, true, false, false)
	if !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("strict import of missing selected file = %v", err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("strict import left partial identities: %#v, %v", records, err)
	}
}

func TestReviewedImportReportsInvalidSelectedCatalogMetadata(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	for _, entry := range []catalogEntry{
		{SessionID: "tauri-one", Title: "bad\ntitle"},
		{SessionID: "tauri-one", WorkspaceRoot: "bad\nworkspace"},
	} {
		writeCatalog(t, catalog, []catalogEntry{entry})
		ctx := context.Background()
		plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-one"})
		if err != nil || len(plan.Rows) != 0 || len(plan.Errors) != 1 || plan.Errors[0].SessionID != "tauri-one" {
			t.Fatalf("invalid catalog metadata was not reported as a skipped row: plan=%#v err=%v", plan, err)
		}
		identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
		if err != nil {
			t.Fatal(err)
		}
		result, err := identities.ApplyImportReview(ctx, plan)
		closeErr := identities.Close()
		if err != nil || closeErr != nil || result.Applied != 0 || !reflect.DeepEqual(result.Errors, plan.Errors) {
			t.Fatalf("all-skipped review result = %#v, apply err=%v close err=%v", result, err, closeErr)
		}
	}
}

func TestReviewedImportSkipsBadSelectedRowsAndCommitsValidRows(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	goodPath := filepath.Join(sessionDir, "tauri-tauri-good.jsonl")
	missingPath := filepath.Join(sessionDir, "tauri-tauri-missing.jsonl")
	unreadablePath := filepath.Join(sessionDir, "tauri-tauri-unreadable.jsonl")
	writeTranscript(t, goodPath)
	if err := os.MkdirAll(unreadablePath, 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{
		{SessionID: "tauri-good", Title: "Good"},
		{SessionID: "tauri-missing", Title: "Missing"},
		{SessionID: "tauri-unreadable", Title: "Unreadable"},
	})
	selected := []string{"tauri-good", "tauri-missing", "tauri-unreadable"}
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, selected)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.SelectedIDs, selected) || len(plan.Rows) != 1 || plan.Rows[0].ID != "tauri-good" || len(plan.Errors) != 2 {
		t.Fatalf("review did not separate usable and skipped rows: %#v", plan)
	}
	if plan.Errors[0].SessionID != "tauri-missing" || plan.Errors[0].Reason != "transcript is missing" ||
		plan.Errors[1].SessionID != "tauri-unreadable" || plan.Errors[1].Reason != "transcript is unreadable" {
		t.Fatalf("review issues = %#v", plan.Errors)
	}
	for _, issue := range plan.Errors {
		if strings.Contains(issue.Reason, root) {
			t.Fatalf("review issue leaked a private path: %#v", issue)
		}
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	result, err := identities.ApplyImportReview(ctx, plan)
	if err != nil || result.Applied != 1 || len(result.Errors) != 2 || !reflect.DeepEqual(result.Errors, plan.Errors) {
		t.Fatalf("partial review result = %#v, %v", result, err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "tauri-good" {
		t.Fatalf("valid row was not committed independently: %#v, %v", records, err)
	}
	if _, err := os.Lstat(missingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("skipped missing transcript was created: %v", err)
	}
}

func TestReviewedImportRejectsPlanWhenSkippedTranscriptAppears(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	goodPath := filepath.Join(sessionDir, "tauri-tauri-good.jsonl")
	missingPath := filepath.Join(sessionDir, "tauri-tauri-missing.jsonl")
	writeTranscript(t, goodPath)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-good"}, {SessionID: "tauri-missing"}})
	plan, err := PrepareImportReview(ctx, nil, sessionDir, catalog, []string{"tauri-good", "tauri-missing"})
	if err != nil || len(plan.Errors) != 1 || plan.Errors[0].SessionID != "tauri-missing" {
		t.Fatalf("initial review = %#v, %v", plan, err)
	}
	writeTranscript(t, missingPath)
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	if _, err := identities.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("review with a newly available skipped file = %v", err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("stale mixed review partially imported valid row: %#v, %v", records, err)
	}
}
