// Package sessionidentity keeps Preview session IDs independent of transcript paths.
package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

const schemaVersion = 2

var ErrPathChanged = errors.New("session identity path changed without an explicit move")
var ErrTitleConflict = errors.New("session title revision changed")
var ErrTitleProtected = errors.New("session title is protected from automatic replacement")
var ErrSessionNotFound = errors.New("session identity not found")
var ErrInvalidTitle = errors.New("invalid session title")

// TitleSource records the authority of a title, not merely whether AI wrote
// its text. A user-requested AI rename has user authority; an imported legacy
// title cannot safely be classified as manual or generated.
type TitleSource string

const (
	TitleFallback      TitleSource = "fallback"
	TitleGenerated     TitleSource = "generated"
	TitleUser          TitleSource = "user"
	TitleLegacyUnknown TitleSource = "legacy_unknown"
)

// TitleOperation is intentionally an action, not a caller-provided source.
// This prevents a background generator from claiming user authority.
type TitleOperation int

const (
	TitleManualRename TitleOperation = iota
	TitleAutomaticGeneration
	TitleFirstMessage
	TitleUserRequestedGeneration
)

type Candidate struct {
	ID            string
	Path          string
	WorkspaceRoot string
	Title         string
	Position      int
}

type Record struct {
	Candidate
	Missing       bool
	TitleSource   TitleSource
	TitleRevision int64
	CreatedAtMS   int64
	UpdatedAtMS   int64
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("session identity path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create session identity directory: %w", err)
	}
	_ = os.Chmod(filepath.Dir(path), 0o700)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	slash := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' {
		slash = "/" + slash
	}
	dsn := (&url.URL{Scheme: "file", Path: slash}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(cause error) (*Store, error) {
		_ = db.Close()
		return nil, cause
	}
	if err := db.PingContext(ctx); err != nil {
		return fail(err)
	}
	for _, pragma := range []string{"PRAGMA busy_timeout=2000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fail(err)
		}
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return fail(err)
	}
	if integrity != "ok" {
		return fail(fmt.Errorf("session identity quick check: %s", integrity))
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > schemaVersion {
		return fail(fmt.Errorf("session identity schema %d is newer than supported %d", version, schemaVersion))
	}
	if version == 0 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			workspace_root TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			title_source TEXT NOT NULL DEFAULT 'fallback' CHECK (title_source IN ('fallback','generated','user','legacy_unknown')),
			title_revision INTEGER NOT NULL DEFAULT 0 CHECK (title_revision >= 0),
			position INTEGER NOT NULL DEFAULT 0,
			missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
	} else if version == 1 {
		// Legacy catalog titles may have been derived from the first user message,
		// manually renamed, or copied from a sidecar. Preserve them without
		// pretending that all nonempty values were explicitly user-authored.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			"ALTER TABLE sessions ADD COLUMN title_source TEXT NOT NULL DEFAULT 'fallback' CHECK (title_source IN ('fallback','generated','user','legacy_unknown'))",
			"ALTER TABLE sessions ADD COLUMN title_revision INTEGER NOT NULL DEFAULT 0 CHECK (title_revision >= 0)",
			"UPDATE sessions SET title_source=CASE WHEN title='' THEN 'fallback' ELSE 'legacy_unknown' END",
			"PRAGMA user_version=2",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v2: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fail(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return fail(err)
	}
	_ = os.Chmod(abs, 0o600)
	_ = os.Chmod(abs+"-wal", 0o600)
	_ = os.Chmod(abs+"-shm", 0o600)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Import(ctx context.Context, previewRoot string, candidates []Candidate) error {
	root, err := filepath.Abs(previewRoot)
	if err != nil || strings.TrimSpace(previewRoot) == "" {
		return errors.New("preview root is invalid")
	}
	prepared := make([]Record, 0, len(candidates))
	seenIDs := make(map[string]bool, len(candidates))
	seenPaths := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidate(root, candidate); err != nil {
			return err
		}
		candidate.Path = filepath.Clean(candidate.Path)
		if seenIDs[candidate.ID] || seenPaths[candidate.Path] {
			return errors.New("duplicate session ID or path in import")
		}
		seenIDs[candidate.ID], seenPaths[candidate.Path] = true, true
		info, err := os.Lstat(candidate.Path)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return fmt.Errorf("inspect transcript: %w", err)
		}
		if !missing && !info.Mode().IsRegular() {
			return fmt.Errorf("transcript is not a regular file: %s", candidate.Path)
		}
		prepared = append(prepared, Record{Candidate: candidate, Missing: missing})
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UnixMilli()
	for _, incoming := range prepared {
		var current Record
		var missing int
		err := tx.QueryRowContext(ctx, `SELECT path, workspace_root, title, position, missing, created_at_ms, updated_at_ms
			FROM sessions WHERE id=?`, incoming.ID).Scan(&current.Path, &current.WorkspaceRoot, &current.Title,
			&current.Position, &missing, &current.CreatedAtMS, &current.UpdatedAtMS)
		if errors.Is(err, sql.ErrNoRows) {
			if incoming.Missing {
				continue
			}
			source := TitleFallback
			if incoming.Title != "" {
				source = TitleLegacyUnknown
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO sessions
				(id, path, workspace_root, title, title_source, position, missing, created_at_ms, updated_at_ms)
				VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`, incoming.ID, incoming.Path,
				incoming.WorkspaceRoot, incoming.Title, source, incoming.Position, now, now)
			if err != nil {
				return fmt.Errorf("register session %s: %w", incoming.ID, err)
			}
			continue
		}
		if err != nil {
			return err
		}
		if current.Path != incoming.Path {
			return fmt.Errorf("%w: %s", ErrPathChanged, incoming.ID)
		}
		// Import is registration/reconciliation, never a title command. The
		// catalog can be stale after a manual rename or automatic generation.
		incoming.Title = current.Title
		if incoming.WorkspaceRoot == "" {
			incoming.WorkspaceRoot = current.WorkspaceRoot
		}
		newMissing := 0
		if incoming.Missing {
			newMissing = 1
		}
		if current.Title == incoming.Title && current.WorkspaceRoot == incoming.WorkspaceRoot &&
			current.Position == incoming.Position && missing == newMissing {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET title=?, workspace_root=?, position=?, missing=?, updated_at_ms=? WHERE id=?`,
			incoming.Title, incoming.WorkspaceRoot, incoming.Position, newMissing, now, incoming.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateCandidate(root string, candidate Candidate) error {
	if candidate.ID == "" || len(candidate.ID) > 128 || candidate.Position < 0 {
		return errors.New("invalid session identity candidate")
	}
	for _, c := range []byte(candidate.ID) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return errors.New("invalid session identity candidate")
		}
	}
	if !filepath.IsAbs(candidate.Path) || filepath.Ext(candidate.Path) != ".jsonl" {
		return errors.New("transcript path is invalid")
	}
	rel, err := filepath.Rel(root, filepath.Clean(candidate.Path))
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("transcript path escapes preview root")
	}
	// A lexical relative path is not enough: root/sessions may be a symlink
	// outside the profile. Resolve the nearest existing parent so even a
	// missing transcript below a linked directory cannot be imported later.
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve preview root: %w", err)
	}
	parent := filepath.Dir(candidate.Path)
	for {
		resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			resolvedRel, relErr := filepath.Rel(resolvedRoot, resolvedParent)
			if relErr != nil || resolvedRel == ".." || filepath.IsAbs(resolvedRel) ||
				strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
				return errors.New("transcript path escapes preview root through a symlink")
			}
			break
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return fmt.Errorf("resolve transcript parent: %w", resolveErr)
		}
		if parent == root {
			return fmt.Errorf("resolve transcript parent: %w", resolveErr)
		}
		parent = filepath.Dir(parent)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, workspace_root, title, title_source, title_revision, position, missing,
		created_at_ms, updated_at_ms FROM sessions ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var record Record
		var missing int
		if err := rows.Scan(&record.ID, &record.Path, &record.WorkspaceRoot, &record.Title, &record.TitleSource, &record.TitleRevision,
			&record.Position, &missing, &record.CreatedAtMS, &record.UpdatedAtMS); err != nil {
			return nil, err
		}
		record.Missing = missing != 0
		records = append(records, record)
	}
	return records, rows.Err()
}

// SetTitle is the only title writer in the identity store. The caller names
// the action, never the desired source, and supplies the revision it observed.
// The revision and protection policy are both checked by the UPDATE itself,
// so a concurrent rename cannot slip between a read and the write.
func (s *Store) SetTitle(ctx context.Context, id string, expectedRevision int64, title string, operation TitleOperation) error {
	if strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 120 ||
		strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return ErrInvalidTitle
	}
	if expectedRevision < 0 {
		return ErrTitleConflict
	}
	var source TitleSource
	var guard string
	switch operation {
	case TitleManualRename, TitleUserRequestedGeneration:
		source = TitleUser
		guard = "1=1"
	case TitleAutomaticGeneration:
		source = TitleGenerated
		guard = "title_source IN ('fallback','generated')"
	case TitleFirstMessage:
		source = TitleFallback
		guard = "title_source='fallback' AND title=''"
	default:
		return errors.New("invalid session title operation")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET title=?, title_source=?,
		title_revision=title_revision+1, updated_at_ms=? WHERE id=? AND title_revision=? AND `+guard,
		title, source, time.Now().UnixMilli(), id, expectedRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 1 {
		return nil
	}
	var revision int64
	if err := s.db.QueryRowContext(ctx, `SELECT title_revision FROM sessions WHERE id=?`, id).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	if revision != expectedRevision {
		return ErrTitleConflict
	}
	return ErrTitleProtected
}
