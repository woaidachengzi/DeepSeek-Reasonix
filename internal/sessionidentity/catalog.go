package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge/sessionpath"
)

type catalogEntry struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

// WorkbenchOrderEntry is host-owned grouping and ordering metadata. It does
// not carry a title or transcript path so syncing the sidebar cannot rewrite
// either identity.
type WorkbenchOrderEntry struct {
	ID            string `json:"sessionId"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

// Keep the Go catalog write boundaries aligned with the host catalog reader.
// Empty legacy titles and workspace roots are valid; callers must validate
// the raw fields before trimming or resolving workspace paths.
func ValidWorkbenchCatalogTitle(title string) bool {
	return utf8.ValidString(title) && utf8.RuneCountInString(title) <= 120 &&
		strings.IndexFunc(title, unicode.IsControl) < 0
}

func ValidWorkbenchCatalogWorkspaceRoot(root string) bool {
	return utf8.ValidString(root) && len(root) <= 4096 &&
		strings.IndexFunc(root, unicode.IsControl) < 0
}

// SyncWorkbenchOrder mirrors the host's bounded recent-session order and
// workspace grouping into the identity store. Existing identity-only rows are
// retained after the host snapshot in their previous relative order. Its
// count reports host entries whose identity and path are recognized, including
// deletion tombstones that remain excluded from the visible order.
func (s *Store) SyncWorkbenchOrder(ctx context.Context, sessionDir string, entries []WorkbenchOrderEntry) (int, error) {
	if strings.TrimSpace(sessionDir) == "" || len(entries) > 50 {
		return 0, errors.New("workbench catalog is invalid")
	}
	if s.profileRoot == "" {
		if err := s.bindProfileRoot(filepath.Dir(sessionDir)); err != nil {
			return 0, err
		}
	}
	if err := normalizeImportPathRoot(s.profileRoot, sessionDir); err != nil {
		return 0, err
	}
	root, err := filepath.Abs(sessionDir)
	if err != nil {
		return 0, fmt.Errorf("resolve session directory: %w", err)
	}
	seen := make(map[string]struct{}, len(entries))
	paths := make(map[string]string, len(entries))
	workspaces := make(map[string]string, len(entries))
	for position, entry := range entries {
		if !ValidWorkbenchCatalogWorkspaceRoot(entry.WorkspaceRoot) {
			return 0, errors.New("workbench workspace path is invalid")
		}
		workspace := entry.WorkspaceRoot
		if strings.TrimSpace(workspace) == "" {
			workspace = ""
		}
		if workspace != "" {
			workspace, err = filepath.Abs(workspace)
			if err != nil {
				return 0, fmt.Errorf("resolve workbench workspace: %w", err)
			}
		}
		path, pathErr := sessionpath.TranscriptPath(root, entry.ID)
		if pathErr != nil {
			return 0, pathErr
		}
		if err := validateCandidate(root, Candidate{ID: entry.ID, Path: path, Position: position}); err != nil {
			return 0, err
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return 0, errors.New("workbench catalog contains a duplicate session")
		}
		seen[entry.ID] = struct{}{}
		paths[entry.ID] = path
		workspaces[entry.ID] = workspace
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, relative_path, workspace_root, title, title_source, title_revision, position, state,
		created_at_ms, updated_at_ms FROM sessions ORDER BY position, id`)
	if err != nil {
		return 0, err
	}
	current := make([]Record, 0)
	byID := make(map[string]Record)
	for rows.Next() {
		record, scanErr := scanIdentityRecord(rows, s.profileRoot)
		if scanErr != nil {
			_ = rows.Close()
			return 0, scanErr
		}
		current = append(current, record)
		byID[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	ordered := make([]Record, 0, len(current))
	listed := make(map[string]struct{}, len(entries))
	syncedEntries := 0
	for _, entry := range entries {
		record, exists := byID[entry.ID]
		if !exists {
			continue
		}
		if filepath.Clean(record.Path) != filepath.Clean(paths[entry.ID]) {
			return 0, fmt.Errorf("%w: %s", ErrPathChanged, entry.ID)
		}
		// A stale host row can outlive a deletion that was fenced or already
		// tombstoned before the host removed its JSON entry. It is synchronized
		// when its ID and path still match, even though it must stay out of the
		// visible order until the host explicitly forgets it.
		syncedEntries++
		if record.State == StateDeleting || record.State == StateDeleted {
			continue
		}
		listed[entry.ID] = struct{}{}
		ordered = append(ordered, record)
	}
	for _, record := range current {
		if record.State == StateDeleting || record.State == StateDeleted {
			continue
		}
		if _, exists := listed[record.ID]; !exists {
			ordered = append(ordered, record)
		}
	}

	now := time.Now().UnixMilli()
	for position, record := range ordered {
		workspace := record.WorkspaceRoot
		if _, isListed := listed[record.ID]; isListed {
			workspace = workspaces[record.ID]
		}
		if record.Position == position && record.WorkspaceRoot == workspace {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET position=?, workspace_root=?, updated_at_ms=? WHERE id=?`,
			position, workspace, now, record.ID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return syncedEntries, nil
}

// ImportWorkbenchCatalog registers the sessions the Preview host lists.
//
// sessionDir must be the directory the writer actually uses, so this importer
// shares one path rule with the bridge instead of re-deriving a layout: a
// session's workspace is UI metadata and never selects its transcript
// directory. The catalog keeps its own order, titles and workspaces.
func (s *Store) ImportWorkbenchCatalog(ctx context.Context, sessionDir, catalogPath string) error {
	entries, err := readCatalog(catalogPath)
	if err != nil {
		return err
	}
	if len(entries) > 50 {
		return errors.New("workbench catalog contains too many sessions")
	}
	candidates := make([]Candidate, 0, len(entries))
	for position, entry := range entries {
		if !ValidWorkbenchCatalogTitle(entry.Title) || !ValidWorkbenchCatalogWorkspaceRoot(entry.WorkspaceRoot) {
			return errors.New("workbench catalog metadata is invalid")
		}
		workspace := entry.WorkspaceRoot
		if strings.TrimSpace(workspace) == "" {
			workspace = ""
		}
		if workspace != "" {
			abs, err := filepath.Abs(workspace)
			if err != nil {
				return fmt.Errorf("resolve workbench workspace: %w", err)
			}
			workspace = abs
		}
		path, err := sessionpath.TranscriptPath(sessionDir, entry.SessionID)
		if err != nil {
			return fmt.Errorf("workbench session %s: %w", entry.SessionID, err)
		}
		candidates = append(candidates, Candidate{
			ID: entry.SessionID, Path: path,
			WorkspaceRoot: workspace, Title: entry.Title, Position: position,
		})
	}
	return s.ImportLegacyCatalog(ctx, sessionDir, candidates)
}
