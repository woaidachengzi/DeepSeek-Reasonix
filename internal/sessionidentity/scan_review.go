package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/profilegate"
)

// ScanImportSelection records an explicit decision about one scan-only file.
// Pointer metadata distinguishes a confirmed empty value from an omitted one.
// The SHA-256 must come from a previous read-only inventory/review of the file.
type ScanImportSelection struct {
	ID               string  `json:"id"`
	Title            *string `json:"title"`
	WorkspaceRoot    *string `json:"workspaceRoot"`
	TranscriptSHA256 string  `json:"transcriptSha256"`
}

// ScanImportReview is a one-shot, offline plan. It never infers consent from
// the presence of a file in the sessions directory.
type ScanImportReview struct {
	SessionDir    string                `json:"sessionDir"`
	CatalogPath   string                `json:"catalogPath"`
	CatalogSHA256 string                `json:"catalogSha256"`
	Selected      []ScanImportSelection `json:"selected"`
	Rows          []ReviewedImportRow   `json:"rows"`
}

// ScanImportCandidate is the path-free information needed to review one
// eligible scan-only transcript in the Preview UI.
type ScanImportCandidate struct {
	ID               string `json:"id"`
	File             string `json:"file"`
	TranscriptSHA256 string `json:"transcriptSha256"`
}

// ScanImportCandidateList reports reviewable files and a count of files that
// were scanned but failed eligibility checks. It never returns absolute paths.
type ScanImportCandidateList struct {
	Candidates   []ScanImportCandidate `json:"candidates"`
	BlockedCount int                   `json:"blockedCount"`
}

// ListScanImportCandidates finds eligible scan-only transcripts without
// opening or creating an identity store and without returning file paths.
func ListScanImportCandidates(ctx context.Context, identities *Store, sessionDir, catalogPath string) (ScanImportCandidateList, error) {
	report, err := Inventory(ctx, identities, sessionDir, catalogPath)
	if err != nil {
		return ScanImportCandidateList{}, err
	}
	result := ScanImportCandidateList{Candidates: make([]ScanImportCandidate, 0)}
	for _, row := range report.Entries {
		if row.Source != InventoryFromScan {
			continue
		}
		if row.Claim != ClaimUnclaimed || !row.Exists || row.Detail != "no catalog entry or identity row explains this file" {
			result.BlockedCount++
			continue
		}
		digest, err := fileSHA256(ctx, row.Path)
		if err != nil {
			result.BlockedCount++
			continue
		}
		result.Candidates = append(result.Candidates, ScanImportCandidate{
			ID: row.ID, File: filepath.Base(row.Path), TranscriptSHA256: digest,
		})
	}
	return result, nil
}

// PrepareScanImportReview reads only scan-only candidates with exact approved
// ID, metadata, and transcript digest. Catalog and registered IDs always win.
// Invalid, unreadable, or changed selections reject the plan rather than being
// silently dropped from a user's explicit selection.
func PrepareScanImportReview(ctx context.Context, identities *Store, sessionDir, catalogPath string, selected []ScanImportSelection) (ScanImportReview, error) {
	if err := ctx.Err(); err != nil {
		return ScanImportReview{}, err
	}
	if len(selected) == 0 || len(selected) > MaxVisibleSnapshotSize {
		return ScanImportReview{}, errors.New("scan import requires a bounded, non-empty selection")
	}
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(catalogPath) == "" {
		return ScanImportReview{}, errors.New("session directory and workbench catalog path are required")
	}
	dir, err := filepath.Abs(sessionDir)
	if err != nil {
		return ScanImportReview{}, err
	}
	catalog, err := filepath.Abs(catalogPath)
	if err != nil {
		return ScanImportReview{}, err
	}
	catalogHash, err := catalogFingerprint(ctx, catalog)
	if err != nil {
		return ScanImportReview{}, err
	}
	entries, err := readCatalog(catalog)
	if err != nil {
		return ScanImportReview{}, err
	}
	if len(entries) > 50 {
		return ScanImportReview{}, errors.New("workbench catalog contains too many sessions")
	}
	report, err := Inventory(ctx, identities, dir, catalog)
	if err != nil {
		return ScanImportReview{}, err
	}
	for _, row := range report.Entries {
		if row.Claim == ClaimPathChanged || row.Claim == ClaimPathConflict {
			return ScanImportReview{}, fmt.Errorf("session inventory conflict for %s: %s", row.ID, row.Claim)
		}
	}
	byID := make(map[string]InventoryEntry)
	for _, row := range report.Entries {
		if row.Source == InventoryFromScan {
			if _, duplicate := byID[row.ID]; duplicate {
				return ScanImportReview{}, errors.New("scan contains a duplicate session ID")
			}
			byID[row.ID] = row
		}
	}
	position := len(entries)
	if identities != nil {
		records, err := identities.List(ctx)
		if err != nil {
			return ScanImportReview{}, err
		}
		for _, record := range records {
			if record.Position >= position {
				if record.Position == math.MaxInt {
					return ScanImportReview{}, errors.New("session order is exhausted")
				}
				position = record.Position + 1
			}
		}
	}
	if position > math.MaxInt-len(selected) {
		return ScanImportReview{}, errors.New("session order is exhausted")
	}
	plan := ScanImportReview{
		SessionDir: dir, CatalogPath: catalog, CatalogSHA256: catalogHash,
		Selected: make([]ScanImportSelection, 0, len(selected)),
		Rows:     make([]ReviewedImportRow, 0, len(selected)),
	}
	seen := make(map[string]bool, len(selected))
	for _, choice := range selected {
		if !sessionpath.ValidID(choice.ID) || seen[choice.ID] || choice.Title == nil || choice.WorkspaceRoot == nil ||
			!ValidWorkbenchCatalogTitle(*choice.Title) || !ValidWorkbenchCatalogWorkspaceRoot(*choice.WorkspaceRoot) ||
			len(choice.TranscriptSHA256) != 64 || !isLowerSHA256(choice.TranscriptSHA256) {
			return ScanImportReview{}, errors.New("scan import selection has invalid or unconfirmed metadata")
		}
		seen[choice.ID] = true
		row, ok := byID[choice.ID]
		if !ok || row.Claim != ClaimUnclaimed || !row.Exists {
			return ScanImportReview{}, fmt.Errorf("selected session %s is not an unclaimed scan candidate", choice.ID)
		}
		path, err := sessionpath.TranscriptPath(dir, choice.ID)
		if err != nil || row.Path != path {
			return ScanImportReview{}, fmt.Errorf("selected session %s path does not round-trip", choice.ID)
		}
		workspace := *choice.WorkspaceRoot
		if workspace != "" && (!filepath.IsAbs(workspace) || filepath.Clean(workspace) != workspace) {
			return ScanImportReview{}, errors.New("confirmed workspace must be an absolute, clean path or empty")
		}
		candidate := Candidate{ID: choice.ID, Path: path, Title: *choice.Title, WorkspaceRoot: workspace, Position: position}
		if err := validateCandidate(dir, candidate); err != nil {
			return ScanImportReview{}, fmt.Errorf("session %s: %w", choice.ID, err)
		}
		digest, err := fileSHA256(ctx, path)
		if err != nil || digest != choice.TranscriptSHA256 {
			return ScanImportReview{}, fmt.Errorf("%w: selected transcript %s changed", ErrImportReviewChanged, choice.ID)
		}
		plan.Selected = append(plan.Selected, choice)
		plan.Rows = append(plan.Rows, ReviewedImportRow{Candidate: candidate, TranscriptSHA256: digest})
		position++
	}
	endingHash, err := catalogFingerprint(ctx, catalog)
	if err != nil || endingHash != catalogHash {
		return ScanImportReview{}, ErrImportReviewChanged
	}
	return plan, nil
}

func catalogFingerprint(ctx context.Context, catalog string) (string, error) {
	info, err := os.Lstat(catalog)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("workbench catalog is not a regular file")
	}
	return fileSHA256(ctx, catalog)
}

func isLowerSHA256(value string) bool {
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// ApplyScanImportReview replays the read-only review while holding the profile
// gate, then registers the exact reviewed rows in one SQLite transaction.
// Direct callers must use a staged copy or otherwise stop writers that ignore
// the gate; the managed Tauri Preview bridge uses the under-gate variant while
// holding exclusive ownership of its separate profile for its lifetime.
func (s *Store) ApplyScanImportReview(ctx context.Context, plan ScanImportReview) (ImportReviewResult, error) {
	if strings.TrimSpace(plan.SessionDir) == "" || strings.TrimSpace(plan.CatalogPath) == "" {
		return ImportReviewResult{}, ErrImportReviewChanged
	}
	if s.profileRoot == "" {
		if err := s.bindProfileRoot(filepath.Dir(plan.SessionDir)); err != nil {
			return ImportReviewResult{}, err
		}
	}
	if err := normalizeImportPathRoot(s.profileRoot, plan.SessionDir); err != nil {
		return ImportReviewResult{}, err
	}
	release, err := profilegate.TryAcquire(s.profileRoot)
	if err != nil {
		return ImportReviewResult{}, fmt.Errorf("session profile ownership: %w", err)
	}
	defer release()
	return s.ApplyScanImportReviewUnderProfileGate(ctx, plan)
}

// ApplyScanImportReviewUnderProfileGate is for the authenticated bridge, which
// holds the profile gate for its whole lifetime. All reviewed inputs are still
// re-read here; callers must prove the process owns that profile gate.
func (s *Store) ApplyScanImportReviewUnderProfileGate(ctx context.Context, plan ScanImportReview) (ImportReviewResult, error) {
	if strings.TrimSpace(plan.SessionDir) == "" || strings.TrimSpace(plan.CatalogPath) == "" {
		return ImportReviewResult{}, ErrImportReviewChanged
	}
	fresh, err := PrepareScanImportReview(ctx, s, plan.SessionDir, plan.CatalogPath, plan.Selected)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ImportReviewResult{}, err
		}
		return ImportReviewResult{}, fmt.Errorf("%w: %w", ErrImportReviewChanged, err)
	}
	if !reflect.DeepEqual(plan, fresh) {
		return ImportReviewResult{}, ErrImportReviewChanged
	}
	candidates := make([]Candidate, len(plan.Rows))
	for i, row := range plan.Rows {
		candidates[i] = row.Candidate
	}
	if err := s.importCandidates(ctx, plan.SessionDir, candidates, true, false, false); err != nil {
		return ImportReviewResult{}, err
	}
	return ImportReviewResult{Applied: len(candidates), Errors: []ImportReviewIssue{}}, nil
}
