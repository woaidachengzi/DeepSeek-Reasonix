package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// BeginDelete durably fences an identity before the core removes its artifact
// bundle. Retrying an interrupted deletion is safe: deleting remains deleting.
// The caller must not write this session after this transition succeeds.
func (s *Store) BeginDelete(ctx context.Context, id, transcriptPath string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET state='deleting', updated_at_ms=?
		WHERE id=? AND path=? AND state IN ('reserved','ready','missing')`,
		time.Now().UnixMilli(), id, transcriptPath)
	if err != nil {
		return fmt.Errorf("begin session deletion: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed == 1 {
		return nil
	}
	record, exists, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrSessionNotFound
	}
	if record.Path == transcriptPath && record.State == StateDeleting {
		return nil
	}
	return fmt.Errorf("%w: %s cannot begin deletion from %s", ErrSessionStateConflict, id, record.State)
}

// FinishDelete leaves a permanent tombstone after the caller has successfully
// removed the entire artifact bundle. This method additionally refuses to
// finalize while the transcript itself still exists; it cannot verify every
// sidecar, so the caller must use the core's artifact sweep first.
func (s *Store) FinishDelete(ctx context.Context, id, transcriptPath string) error {
	if _, err := os.Lstat(transcriptPath); err == nil {
		return fmt.Errorf("%w: transcript still exists", ErrSessionStateConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect deleted transcript: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET state='deleted', updated_at_ms=?
		WHERE id=? AND path=? AND state='deleting'`, time.Now().UnixMilli(), id, transcriptPath)
	if err != nil {
		return fmt.Errorf("finish session deletion: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed == 1 {
		return nil
	}
	record, exists, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrSessionNotFound
	}
	if record.Path == transcriptPath && record.State == StateDeleted {
		return nil
	}
	return fmt.Errorf("%w: %s cannot finish deletion from %s", ErrSessionStateConflict, id, record.State)
}
