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

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var ErrPathChanged = errors.New("session identity path changed without an explicit move")

type Candidate struct {
	ID            string
	Path          string
	WorkspaceRoot string
	Title         string
	Position      int
}

type Record struct {
	Candidate
	Missing     bool
	CreatedAtMS int64
	UpdatedAtMS int64
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
			position INTEGER NOT NULL DEFAULT 0,
			missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			_ = tx.Rollback()
			return fail(err)
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
			_, err = tx.ExecContext(ctx, `INSERT INTO sessions
				(id, path, workspace_root, title, position, missing, created_at_ms, updated_at_ms)
				VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, incoming.ID, incoming.Path,
				incoming.WorkspaceRoot, incoming.Title, incoming.Position, now, now)
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
		if incoming.Title == "" {
			incoming.Title = current.Title
		}
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
	return nil
}

func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, workspace_root, title, position, missing,
		created_at_ms, updated_at_ms FROM sessions ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var record Record
		var missing int
		if err := rows.Scan(&record.ID, &record.Path, &record.WorkspaceRoot, &record.Title,
			&record.Position, &missing, &record.CreatedAtMS, &record.UpdatedAtMS); err != nil {
			return nil, err
		}
		record.Missing = missing != 0
		records = append(records, record)
	}
	return records, rows.Err()
}
