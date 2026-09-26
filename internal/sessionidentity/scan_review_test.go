package sessionidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/profilegate"
)

func scanChoice(t *testing.T, path, id, title, workspace string) ScanImportSelection {
	t.Helper()
	digest, err := fileSHA256(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return ScanImportSelection{ID: id, Title: &title, WorkspaceRoot: &workspace, TranscriptSHA256: digest}
}

func TestScanImportRequiresExplicitMetadataAndUsesStagedFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "sessions")
	path := filepath.Join(dir, "tauri-orphan.jsonl")
	writeTranscript(t, path)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{})
	choice := scanChoice(t, path, "orphan", "Reviewed", "/work/reviewed")
	if _, err := PrepareScanImportReview(ctx, nil, dir, catalog, nil); err == nil {
		t.Fatal("empty selection was accepted")
	}
	withoutTitle := choice
	withoutTitle.Title = nil
	if _, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{withoutTitle}); err == nil {
		t.Fatal("unconfirmed title was accepted")
	}
	plan, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{choice})
	if err != nil || len(plan.Rows) != 1 || plan.Rows[0].Position != 0 {
		t.Fatalf("scan review = %#v, %v", plan, err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	release, err := profilegate.TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identities.ApplyScanImportReview(ctx, plan); !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("apply under held profile gate = %v", err)
	}
	release()
	result, err := identities.ApplyScanImportReview(ctx, plan)
	if err != nil || result.Applied != 1 {
		t.Fatalf("apply = %#v, %v", result, err)
	}
	record, found, err := identities.Get(ctx, "orphan")
	if err != nil || !found || record.Path != path || record.Title != "Reviewed" || record.WorkspaceRoot != "/work/reviewed" || record.State != StateReady {
		t.Fatalf("imported record = %#v, found=%v, err=%v", record, found, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "{}\n" {
		t.Fatalf("transcript changed: %q, %v", contents, err)
	}
}

func TestListScanImportCandidatesIsPathFreeAndSupportsMissingCatalog(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "sessions")
	path := filepath.Join(dir, "tauri-orphan.jsonl")
	writeTranscript(t, path)
	catalog := filepath.Join(root, "workbench-sessions.json")

	listing, err := ListScanImportCandidates(ctx, nil, dir, catalog)
	if err != nil || len(listing.Candidates) != 1 || listing.BlockedCount != 0 {
		t.Fatalf("candidate listing = %#v, %v", listing, err)
	}
	if listing.Candidates[0].ID != "orphan" || listing.Candidates[0].File != filepath.Base(path) || listing.Candidates[0].TranscriptSHA256 == "" {
		t.Fatalf("candidate = %#v", listing.Candidates[0])
	}
	choice := scanChoice(t, path, "orphan", "Reviewed", "")
	plan, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{choice})
	if err != nil || plan.CatalogSHA256 != "missing" {
		t.Fatalf("missing-catalog review = %#v, %v", plan, err)
	}
}

func TestScanImportRejectsCatalogOwnershipAndChangedTranscript(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "sessions")
	known := filepath.Join(dir, "tauri-known.jsonl")
	orphan := filepath.Join(dir, "tauri-orphan.jsonl")
	writeTranscript(t, known)
	writeTranscript(t, orphan)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "known", Title: "Catalog"}})
	if _, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{scanChoice(t, known, "known", "Override", "")}); err == nil {
		t.Fatal("catalog-owned file was accepted as scan-only")
	}
	choice := scanChoice(t, orphan, "orphan", "Orphan", "")
	plan, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{choice})
	if err != nil || plan.Rows[0].Position != 1 {
		t.Fatalf("scan plan = %#v, %v", plan, err)
	}
	if err := os.WriteFile(orphan, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	if _, err := identities.ApplyScanImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("stale transcript accepted: %v", err)
	}
	records, err := identities.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("stale import wrote rows: %#v, %v", records, err)
	}
}

func TestScanImportRejectsForgedPlanAndCatalogChange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "sessions")
	path := filepath.Join(dir, "tauri-orphan.jsonl")
	writeTranscript(t, path)
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{})
	plan, err := PrepareScanImportReview(ctx, nil, dir, catalog, []ScanImportSelection{scanChoice(t, path, "orphan", "Title", "")})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := Open(ctx, filepath.Join(root, "desktop", "state.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identities.Close() })
	forged := plan
	forged.Rows = append([]ReviewedImportRow(nil), plan.Rows...)
	forged.Rows[0].Title = "Forged"
	if _, err := identities.ApplyScanImportReview(ctx, forged); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("forged plan accepted: %v", err)
	}
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "other"}})
	if _, err := identities.ApplyScanImportReview(ctx, plan); !errors.Is(err, ErrImportReviewChanged) {
		t.Fatalf("changed catalog accepted: %v", err)
	}
}
