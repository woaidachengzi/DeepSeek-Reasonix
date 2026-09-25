package sessionidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/profilegate"
)

var ErrImportReviewChanged = errors.New("session import review is stale")

// ReviewedImportRow is the catalog metadata and transcript fingerprint a
// reviewer approved. The fingerprint is a change detector, not an identity:
// two copied transcripts can have identical bytes but distinct session IDs.
type ReviewedImportRow struct {
	Candidate
	TranscriptSHA256 string
}

// ImportReviewIssue reports one explicitly selected catalog row that was
// skipped without preventing unrelated valid rows from being reviewed.
// Reasons are deliberately path-free so a review can be rendered safely.
type ImportReviewIssue struct {
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}

// ImportReview is a read-only snapshot of an explicitly selected catalog
// subset. Unclaimed scan results are deliberately never included.
type ImportReview struct {
	SessionDir    string
	CatalogPath   string
	CatalogSHA256 string
	SelectedIDs   []string
	Rows          []ReviewedImportRow
	Errors        []ImportReviewIssue
}

// ImportReviewResult accompanies the transactional apply outcome with the
// rows accepted for application and every selected row skipped during review.
type ImportReviewResult struct {
	Applied int                 `json:"applied"`
	Errors  []ImportReviewIssue `json:"errors"`
}

// PrepareImportReview does not open or create an identity database. identities
// may be nil when no database exists. The caller must explicitly select IDs;
// an empty selection cannot turn a directory scan into an automatic import.
func PrepareImportReview(ctx context.Context, identities *Store, sessionDir, catalogPath string, selectedIDs []string) (ImportReview, error) {
	if err := ctx.Err(); err != nil {
		return ImportReview{}, err
	}
	if len(selectedIDs) == 0 {
		return ImportReview{}, errors.New("no catalog sessions selected for import")
	}
	dir, err := filepath.Abs(sessionDir)
	if err != nil || strings.TrimSpace(sessionDir) == "" {
		return ImportReview{}, errors.New("session directory is invalid")
	}
	catalog, err := filepath.Abs(catalogPath)
	if err != nil || strings.TrimSpace(catalogPath) == "" {
		return ImportReview{}, errors.New("workbench catalog path is invalid")
	}
	info, err := os.Lstat(catalog)
	if err != nil {
		return ImportReview{}, fmt.Errorf("inspect workbench catalog: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ImportReview{}, errors.New("workbench catalog is not a regular file")
	}
	catalogHash, err := fileSHA256(ctx, catalog)
	if err != nil {
		return ImportReview{}, err
	}
	entries, err := readCatalog(catalog)
	if err != nil {
		return ImportReview{}, err
	}
	if len(entries) > 50 {
		return ImportReview{}, errors.New("workbench catalog contains too many sessions")
	}
	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		if !sessionpath.ValidID(id) || selected[id] {
			return ImportReview{}, errors.New("selected session IDs are invalid or duplicated")
		}
		selected[id] = true
	}
	counts := make(map[string]int, len(entries))
	for _, entry := range entries {
		counts[entry.SessionID]++
	}
	report, err := Inventory(ctx, identities, dir, catalog)
	if err != nil {
		return ImportReview{}, err
	}
	// An already registered ID moving to a new path, or two claims on one
	// path, invalidates the whole batch. Unreadable unrelated files remain
	// diagnostics rather than blocking valid selected rows.
	for _, row := range report.Entries {
		if row.Claim == ClaimPathChanged || row.Claim == ClaimPathConflict {
			return ImportReview{}, fmt.Errorf("session inventory conflict for %s: %s", row.ID, row.Claim)
		}
	}
	byID := make(map[string][]InventoryEntry, len(report.Entries))
	for _, row := range report.Entries {
		if row.Source == InventoryFromWorkbench {
			byID[row.ID] = append(byID[row.ID], row)
		}
	}
	plan := ImportReview{
		SessionDir: dir, CatalogPath: catalog, CatalogSHA256: catalogHash,
		SelectedIDs: append([]string(nil), selectedIDs...),
		Rows:        make([]ReviewedImportRow, 0, len(selected)),
		Errors:      make([]ImportReviewIssue, 0),
	}
	selectedFound := make(map[string]bool, len(selected))
	for position, entry := range entries {
		if !selected[entry.SessionID] {
			continue
		}
		selectedFound[entry.SessionID] = true
		if utf8.RuneCountInString(entry.Title) > 120 || strings.IndexFunc(entry.Title, unicode.IsControl) >= 0 ||
			len(entry.WorkspaceRoot) > 4096 || strings.IndexFunc(entry.WorkspaceRoot, unicode.IsControl) >= 0 {
			plan.Errors = append(plan.Errors, ImportReviewIssue{SessionID: entry.SessionID, Reason: "catalog metadata is invalid"})
			continue
		}
		if counts[entry.SessionID] != 1 {
			return ImportReview{}, fmt.Errorf("session %s is duplicated or invalid in the catalog", entry.SessionID)
		}
		if len(byID[entry.SessionID]) != 1 {
			plan.Errors = append(plan.Errors, ImportReviewIssue{SessionID: entry.SessionID, Reason: "session inventory entry is unavailable"})
			continue
		}
		row := byID[entry.SessionID][0]
		if !row.Exists || row.Detail != "" || (row.Claim != Claimable && row.Claim != ClaimRegistered) {
			plan.Errors = append(plan.Errors, ImportReviewIssue{SessionID: entry.SessionID, Reason: importReviewSkipReason(row)})
			continue
		}
		if identities != nil {
			record, exists, err := identities.Get(ctx, entry.SessionID)
			if err != nil {
				return ImportReview{}, err
			}
			if exists && (record.State == StateDeleting || record.State == StateDeleted) {
				plan.Errors = append(plan.Errors, ImportReviewIssue{SessionID: entry.SessionID, Reason: "identity is retired or deleting"})
				continue
			}
		}
		path, err := sessionpath.TranscriptPath(dir, entry.SessionID)
		if err != nil || path != row.Path {
			return ImportReview{}, fmt.Errorf("session %s path does not match the bridge", entry.SessionID)
		}
		candidate := Candidate{ID: entry.SessionID, Path: path, Title: entry.Title, Position: position}
		workspace := strings.TrimSpace(entry.WorkspaceRoot)
		if workspace != "" {
			candidate.WorkspaceRoot, err = filepath.Abs(workspace)
			if err != nil {
				return ImportReview{}, fmt.Errorf("resolve session %s workspace: %w", entry.SessionID, err)
			}
		}
		if err := validateCandidate(dir, candidate); err != nil {
			return ImportReview{}, fmt.Errorf("session %s: %w", entry.SessionID, err)
		}
		fingerprint, err := fileSHA256(ctx, path)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return ImportReview{}, contextErr
			}
			plan.Errors = append(plan.Errors, ImportReviewIssue{SessionID: entry.SessionID, Reason: "transcript could not be read"})
			continue
		}
		plan.Rows = append(plan.Rows, ReviewedImportRow{Candidate: candidate, TranscriptSHA256: fingerprint})
	}
	for _, id := range selectedIDs {
		if !selectedFound[id] {
			return ImportReview{}, fmt.Errorf("selected session %s is absent from the catalog", id)
		}
	}
	endingHash, err := fileSHA256(ctx, catalog)
	if err != nil || endingHash != catalogHash {
		return ImportReview{}, ErrImportReviewChanged
	}
	return plan, nil
}

func importReviewSkipReason(row InventoryEntry) string {
	if !row.Exists && (row.Detail == "" || row.Detail == "transcript is absent") {
		return "transcript is missing"
	}
	if row.Detail != "" && row.Detail != "transcript is absent" {
		return "transcript is unreadable"
	}
	switch row.Claim {
	case ClaimMissingFile:
		return "transcript is missing"
	case ClaimUnreadable:
		return "transcript is unreadable"
	case ClaimInvalidFile:
		return "transcript failed path validation"
	default:
		if row.Detail != "" {
			return "transcript failed inventory validation"
		}
		return "session is not eligible for import"
	}
}

// ApplyImportReview rechecks every selected catalog row and transcript digest
// before the existing single-transaction importer writes any identity row.
// This is an offline S1 operation: it acquires the profile gate itself, while
// callers must separately quiesce writers (such as old Wails versions) that do
// not participate in that gate.
func (s *Store) ApplyImportReview(ctx context.Context, plan ImportReview) (ImportReviewResult, error) {
	if strings.TrimSpace(plan.SessionDir) == "" || strings.TrimSpace(plan.CatalogPath) == "" {
		return ImportReviewResult{}, ErrImportReviewChanged
	}
	if s.profileRoot == "" {
		if err := s.bindProfileRoot(filepath.Dir(plan.SessionDir)); err != nil {
			return ImportReviewResult{}, err
		}
	}
	releaseProfile, err := profilegate.TryAcquire(s.profileRoot)
	if err != nil {
		return ImportReviewResult{}, fmt.Errorf("session profile ownership: %w", err)
	}
	defer releaseProfile()
	ids := append([]string(nil), plan.SelectedIDs...)
	if len(ids) == 0 {
		return ImportReviewResult{}, ErrImportReviewChanged
	}
	fresh, err := PrepareImportReview(ctx, s, plan.SessionDir, plan.CatalogPath, ids)
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
	result := ImportReviewResult{Applied: len(candidates), Errors: append([]ImportReviewIssue(nil), plan.Errors...)}
	if len(candidates) == 0 {
		return result, nil
	}
	if err := s.importCandidates(ctx, plan.SessionDir, candidates, true, false, false); err != nil {
		result.Applied = 0
		return result, err
	}
	return result, nil
}

func fileSHA256(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("file is not regular")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: file}); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
