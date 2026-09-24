package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
	if err := store.ApplyImportReview(ctx, plan); err != nil {
		t.Fatal(err)
	}
	first, err := store.List(ctx)
	if err != nil || len(first) != 1 || first[0].ID != "tauri-first" || first[0].TitleSource != TitleLegacyUnknown {
		t.Fatalf("imported identities = %#v, %v", first, err)
	}
	if err := store.ApplyImportReview(ctx, plan); err != nil {
		t.Fatalf("review replay: %v", err)
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
	if err := store.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
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
	if err := store.ApplyImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
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
	if err := identities.ApplyImportReview(ctx, plan); err != nil {
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
	}, true)
	if !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("strict import of missing selected file = %v", err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("strict import left partial identities: %#v, %v", records, err)
	}
}

func TestReviewedImportRejectsInvalidSelectedCatalogMetadata(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	writeTranscript(t, filepath.Join(sessionDir, "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	for _, entry := range []catalogEntry{
		{SessionID: "tauri-one", Title: "bad\ntitle"},
		{SessionID: "tauri-one", WorkspaceRoot: "bad\nworkspace"},
	} {
		writeCatalog(t, catalog, []catalogEntry{entry})
		if _, err := PrepareImportReview(context.Background(), nil, sessionDir, catalog, []string{"tauri-one"}); err == nil {
			t.Fatalf("invalid catalog metadata was accepted: %#v", entry)
		}
	}
}
