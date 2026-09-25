package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// BeginDelete durably fences an identity before the core removes its artifact
// bundle. Retrying an interrupted deletion is safe: deleting remains deleting.
// The caller must not write this session after this transition succeeds.
func (s *Store) BeginDelete(ctx context.Context, id, transcriptPath string) error {
	return s.beginDelete(ctx, id, transcriptPath, false)
}

// BeginDeletePendingManualTitle permits an explicit DELETE of an unowned
// reserved/ready session only while its interrupted manual rename is still
// recorded. The intent check and deletion fence share one transaction, so a
// completed recovery cannot race this authorization into deleting an ordinary
// unowned session.
func (s *Store) BeginDeletePendingManualTitle(ctx context.Context, id, transcriptPath string) error {
	return s.beginDelete(ctx, id, transcriptPath, true)
}

func (s *Store) beginDelete(ctx context.Context, id, transcriptPath string, requirePendingTitle bool) error {
	relativePath, err := relativeTranscriptPath(s.profileRoot, id, transcriptPath)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize the path check with MarkReady and other deletion fences before
	// reading lifecycle state. This avoids a deferred read-to-write upgrade race.
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET updated_at_ms=updated_at_ms WHERE id=?", id); err != nil {
		return fmt.Errorf("begin session deletion: %w", err)
	}
	var storedRelative string
	var state SessionState
	err = tx.QueryRowContext(ctx, "SELECT relative_path, state FROM sessions WHERE id=?", id).Scan(&storedRelative, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if storedRelative != relativePath {
		return fmt.Errorf("%w: %s cannot begin deletion from a different path", ErrSessionStateConflict, id)
	}
	if requirePendingTitle {
		if state != StateReserved && state != StateReady {
			return fmt.Errorf("%w: %s has no deletable pending title in %s", ErrSessionStateConflict, id, state)
		}
		var intentRelative string
		err := tx.QueryRowContext(ctx, "SELECT relative_path FROM session_title_intents WHERE session_id=?", id).Scan(&intentRelative)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s has no pending manual title", ErrSessionStateConflict, id)
		}
		if err != nil {
			return err
		}
		if intentRelative != relativePath {
			return fmt.Errorf("%w: %s pending title path differs", ErrSessionStateConflict, id)
		}
	}
	if state == StateDeleting {
		if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, id, transcriptPath); err != nil {
			return err
		}
		if err := discardPendingTitleRenameForDelete(ctx, tx, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	if state != StateReserved && state != StateReady && state != StateMissing {
		return fmt.Errorf("%w: %s cannot begin deletion from %s", ErrSessionStateConflict, id, state)
	}
	if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, id, transcriptPath); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET state='deleting', updated_at_ms=?
		WHERE id=? AND relative_path=? AND state IN ('reserved','ready','missing')`,
		time.Now().UnixMilli(), id, relativePath)
	if err != nil {
		return fmt.Errorf("begin session deletion: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("%w: %s cannot begin deletion from %s", ErrSessionStateConflict, id, state)
	}
	if err := discardPendingTitleRenameForDelete(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishDelete leaves a permanent tombstone after the caller has successfully
// removed the entire artifact bundle. This method refuses to finalize while
// the transcript exists or its path is still claimed by another identity; it
// cannot verify every sidecar, so the caller must use the core's artifact sweep
// first.
func (s *Store) FinishDelete(ctx context.Context, id, transcriptPath string) error {
	relativePath, err := relativeTranscriptPath(s.profileRoot, id, transcriptPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(transcriptPath); err == nil {
		return fmt.Errorf("%w: transcript still exists", ErrSessionStateConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect deleted transcript: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET updated_at_ms=updated_at_ms WHERE id=?", id); err != nil {
		return fmt.Errorf("finish session deletion: %w", err)
	}
	var storedRelative string
	var state SessionState
	err = tx.QueryRowContext(ctx, "SELECT relative_path, state FROM sessions WHERE id=?", id).Scan(&storedRelative, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if storedRelative != relativePath {
		return fmt.Errorf("%w: %s cannot finish deletion from a different path", ErrSessionStateConflict, id)
	}
	if state == StateDeleted {
		if err := discardPendingTitleRenameForDelete(ctx, tx, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	if state != StateDeleting {
		return fmt.Errorf("%w: %s cannot finish deletion from %s", ErrSessionStateConflict, id, state)
	}
	if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, id, transcriptPath); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET state='deleted', updated_at_ms=?
		WHERE id=? AND relative_path=? AND state='deleting'`, time.Now().UnixMilli(), id, relativePath)
	if err != nil {
		return fmt.Errorf("finish session deletion: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("%w: %s cannot finish deletion from %s", ErrSessionStateConflict, id, state)
	}
	if err := discardPendingTitleRenameForDelete(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Once explicit deletion is fenced, the title can never be committed or
// recovered by reopening the session. Remove its intent in the same
// transaction as the lifecycle change so inventory cannot remain dirty after
// the artifact sweep and tombstone succeed.
func discardPendingTitleRenameForDelete(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_title_intents WHERE session_id=?", id); err != nil {
		return fmt.Errorf("discard pending session title rename during deletion: %w", err)
	}
	return nil
}
