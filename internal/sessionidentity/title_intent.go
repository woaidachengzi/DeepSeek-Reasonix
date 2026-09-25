package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ManualTitleIntent records enough evidence to finish or abandon a user rename
// after a crash between the sidecar write and the identity title CAS.
type ManualTitleIntent struct {
	SessionID            string
	Path                 string
	ExpectedRevision     int64
	PreviousSidecarTitle string
	NewTitle             string
}

// PendingManualTitleRenameIDs returns only identifiers. Inventory uses it to
// keep an interrupted rename from appearing as a clean shadow directory,
// including when the sidecar still has its old title.
func (s *Store) PendingManualTitleRenameIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT session_id FROM session_title_intents ORDER BY session_id LIMIT ?", MaxVisibleSnapshotSize+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > MaxVisibleSnapshotSize {
		return nil, errors.New("too many pending session title renames")
	}
	return ids, nil
}

// DiscardTerminalManualTitleRenames removes intents that an older bridge could
// leave after a session entered deleting/deleted. Those sessions cannot be
// reopened to replay a rename; the explicit deletion fence is the winning
// user action. Bridge startup calls this only while owning the profile gate.
// Intents for reserved, ready, and missing sessions are never inferred away.
func (s *Store) DiscardTerminalManualTitleRenames(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM session_title_intents
		WHERE EXISTS (SELECT 1 FROM sessions WHERE sessions.id=session_title_intents.session_id
		AND sessions.state IN ('deleting','deleted'))`)
	if err != nil {
		return 0, fmt.Errorf("discard terminal session title renames: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) PendingManualTitleRename(ctx context.Context, id string) (ManualTitleIntent, bool, error) {
	var intent ManualTitleIntent
	var relative string
	err := s.db.QueryRowContext(ctx, `SELECT session_id, relative_path, expected_revision,
		previous_sidecar_title, new_title FROM session_title_intents WHERE session_id=?`, id).
		Scan(&intent.SessionID, &relative, &intent.ExpectedRevision, &intent.PreviousSidecarTitle, &intent.NewTitle)
	if errors.Is(err, sql.ErrNoRows) {
		return ManualTitleIntent{}, false, nil
	}
	if err != nil {
		return ManualTitleIntent{}, false, err
	}
	intent.Path, err = resolveTranscriptPath(s.profileRoot, id, relative)
	if err != nil {
		return ManualTitleIntent{}, false, fmt.Errorf("resolve pending session title path: %w", err)
	}
	return intent, true, nil
}

// BeginManualTitleRename persists the user's intent before the legacy sidecar
// changes. A second title writer cannot begin while this intent is unresolved.
func (s *Store) BeginManualTitleRename(ctx context.Context, id, path, previousSidecarTitle, newTitle string, expectedRevision int64) error {
	if strings.TrimSpace(newTitle) == "" || !utf8.ValidString(newTitle) || utf8.RuneCountInString(newTitle) > 120 ||
		strings.IndexFunc(newTitle, unicode.IsControl) >= 0 {
		return ErrInvalidTitle
	}
	if expectedRevision < 0 || !utf8.ValidString(previousSidecarTitle) {
		return ErrTitleConflict
	}
	relative, err := relativeTranscriptPath(s.profileRoot, id, path)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET updated_at_ms=updated_at_ms WHERE id=?", id); err != nil {
		return err
	}
	var storedPath, state string
	var revision int64
	if err := tx.QueryRowContext(ctx, "SELECT relative_path, state, title_revision FROM sessions WHERE id=?", id).
		Scan(&storedPath, &state, &revision); errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	if storedPath != relative {
		return ErrPathChanged
	}
	if state != string(StateReserved) && state != string(StateReady) {
		return ErrSessionStateConflict
	}
	if revision != expectedRevision {
		return ErrTitleConflict
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM session_title_intents WHERE session_id=?)", id).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return ErrTitleConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_title_intents
		(session_id, relative_path, expected_revision, previous_sidecar_title, new_title, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?)`, id, relative, expectedRevision, previousSidecarTitle, newTitle, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("begin session title rename: %w", err)
	}
	return tx.Commit()
}

// CommitManualTitleRename updates the title and removes its durable intent in
// one SQLite transaction. The caller must first verify the sidecar has the
// intended title; crash recovery performs that same check before calling it.
func (s *Store) CommitManualTitleRename(ctx context.Context, id, path string) error {
	intent, ok, err := s.PendingManualTitleRename(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTitleConflict
	}
	if intent.Path != path {
		return ErrPathChanged
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET title=?, title_source='user',
		title_revision=title_revision+1, updated_at_ms=? WHERE id=? AND relative_path=(
		SELECT relative_path FROM session_title_intents WHERE session_id=? AND expected_revision=? AND new_title=?
		) AND title_revision=? AND state IN ('reserved','ready')`, intent.NewTitle, time.Now().UnixMilli(), id,
		id, intent.ExpectedRevision, intent.NewTitle, intent.ExpectedRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrTitleConflict
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_title_intents WHERE session_id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// AbandonManualTitleRename clears an intent only while the identity revision
// still matches. The caller must first prove the sidecar still has its old
// title (or has been safely restored to it).
func (s *Store) AbandonManualTitleRename(ctx context.Context, id string, expectedRevision int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM session_title_intents WHERE session_id=? AND expected_revision=?
		AND EXISTS (SELECT 1 FROM sessions WHERE id=? AND title_revision=?)`, id, expectedRevision, id, expectedRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrTitleConflict
	}
	return tx.Commit()
}
