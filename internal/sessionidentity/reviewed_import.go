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
)

var ErrImportReviewChanged = errors.New("session import review is stale")

// ReviewedImportRow is the catalog metadata and transcript fingerprint a
// reviewer approved. The fingerprint is a change detector, not an identity:
// two copied transcripts can have identical bytes but distinct session IDs.
type ReviewedImportRow struct {
	Candidate
	TranscriptSHA256 string
}

// ImportReview is a read-only snapshot of an explicitly selected catalog
// subset. Unclaimed scan results are deliberately never included.
type ImportReview struct {
	SessionDir    string
	CatalogPath   string
	CatalogSHA256 string
	Rows          []ReviewedImportRow
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
	plan := ImportReview{SessionDir: dir, CatalogPath: catalog, CatalogSHA256: catalogHash}
	for position, entry := range entries {
		if !selected[entry.SessionID] {
			continue
		}
		if utf8.RuneCountInString(entry.Title) > 120 || strings.IndexFunc(entry.Title, unicode.IsControl) >= 0 ||
			len(entry.WorkspaceRoot) > 4096 || strings.IndexFunc(entry.WorkspaceRoot, unicode.IsControl) >= 0 {
			return ImportReview{}, fmt.Errorf("session %s catalog metadata is invalid", entry.SessionID)
		}
		if counts[entry.SessionID] != 1 || len(byID[entry.SessionID]) != 1 {
			return ImportReview{}, fmt.Errorf("session %s is duplicated or invalid in the catalog", entry.SessionID)
		}
		row := byID[entry.SessionID][0]
		if !row.Exists || row.Detail != "" || (row.Claim != Claimable && row.Claim != ClaimRegistered) {
			return ImportReview{}, fmt.Errorf("session %s cannot be imported: %s (%s)", entry.SessionID, row.Claim, row.Detail)
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
			return ImportReview{}, fmt.Errorf("hash session %s: %w", entry.SessionID, err)
		}
		plan.Rows = append(plan.Rows, ReviewedImportRow{Candidate: candidate, TranscriptSHA256: fingerprint})
	}
	if len(plan.Rows) != len(selected) {
		return ImportReview{}, errors.New("one or more selected sessions are absent from the catalog")
	}
	endingHash, err := fileSHA256(ctx, catalog)
	if err != nil || endingHash != catalogHash {
		return ImportReview{}, ErrImportReviewChanged
	}
	return plan, nil
}

// ApplyImportReview rechecks every selected catalog row and transcript digest
// before the existing single-transaction importer writes any identity row.
// This is an offline S1 operation: its caller must quiesce transcript writers
// and hold a profile-level ownership lock before using it on a real profile.
func (s *Store) ApplyImportReview(ctx context.Context, plan ImportReview) error {
	if s.profileRoot == "" {
		if err := s.bindProfileRoot(filepath.Dir(plan.SessionDir)); err != nil {
			return err
		}
	}
	ids := make([]string, len(plan.Rows))
	for i, row := range plan.Rows {
		ids[i] = row.ID
	}
	fresh, err := PrepareImportReview(ctx, s, plan.SessionDir, plan.CatalogPath, ids)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrImportReviewChanged, err)
	}
	if !reflect.DeepEqual(plan, fresh) {
		return ErrImportReviewChanged
	}
	candidates := make([]Candidate, len(plan.Rows))
	for i, row := range plan.Rows {
		candidates[i] = row.Candidate
	}
	return s.importCandidates(ctx, plan.SessionDir, candidates, true, false, false)
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
