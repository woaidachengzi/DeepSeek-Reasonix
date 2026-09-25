package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

// deleteOwnedOrInterruptedSession normally deletes the live, owned runtime.
// After a process restart there is no owned runtime. A deleting identity can
// finish its sweep, and an explicit DELETE can retire a missing session or a
// ready/reserved session with a stranded manual title intent without reopening
// it (which may fail on an ambiguous sidecar).
func (b *bridgeServer) deleteOwnedOrInterruptedSession(ctx context.Context, sessionID string) error {
	err := b.runtimes.DeleteSession(sessionID)
	if !errors.Is(err, desktopbridge.ErrSessionNotFound) {
		return err
	}
	return deleteUnownedRecoverableSession(ctx, sessionID)
}

func deleteUnownedRecoverableSession(ctx context.Context, sessionID string) error {
	identityPath := appconfig.DesktopSessionIdentityPath()
	sessionDir := appconfig.SessionDir()
	if identityPath == "" || sessionDir == "" {
		return desktopbridge.ErrSessionNotFound
	}
	// A lookup must not create a new identity DB or follow an unexpected link.
	info, err := os.Lstat(identityPath)
	if errors.Is(err, os.ErrNotExist) {
		return desktopbridge.ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect session identity for deletion recovery: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("session identity store is not a regular file")
	}
	expectedPath, err := bridgeSessionPath(sessionDir, sessionID)
	if err != nil {
		return desktopbridge.ErrSessionNotFound
	}
	// DELETE must persist the deleting fence and final tombstone. This writable
	// open may migrate an older schema after validation; the separate GET
	// recovery list uses OpenReadOnly and never migrates the identity store.
	identities, err := sessionidentity.Open(ctx, identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		return fmt.Errorf("open session identity for deletion recovery: %w", err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists || record.Path != expectedPath {
		return desktopbridge.ErrSessionNotFound
	}
	if record.State == sessionidentity.StateDeleted {
		// The core sweep may have completed just before the host removed its
		// legacy catalog row. Treat a retry as success without touching files.
		return nil
	}
	if record.State != sessionidentity.StateDeleting && record.State != sessionidentity.StateMissing &&
		record.State != sessionidentity.StateReady && record.State != sessionidentity.StateReserved {
		return desktopbridge.ErrSessionNotFound
	}
	// A missing transcript is not an orphan eligible for ID reuse, but an
	// explicit user delete may still retire the identity permanently. Fence it
	// first so an overlapping open cannot recreate it during cleanup. Re-check
	// uniqueness for deleting retries too: a legacy duplicate owner must not be
	// swept just because an earlier version persisted the fence before crashing.
	beginDelete := identities.BeginDelete
	if record.State == sessionidentity.StateReady || record.State == sessionidentity.StateReserved {
		intent, pending, err := identities.PendingManualTitleRename(ctx, sessionID)
		if err != nil {
			return err
		}
		if !pending || intent.Path != expectedPath {
			return desktopbridge.ErrSessionNotFound
		}
		beginDelete = identities.BeginDeletePendingManualTitle
	}
	if err := beginDelete(ctx, sessionID, expectedPath); err != nil {
		return fmt.Errorf("fence recoverable session deletion: %w", bridgeIdentityConflict(err))
	}
	if err := control.RemoveSessionArtifacts(expectedPath); err != nil {
		return fmt.Errorf("remove recoverable session artifacts: %w", err)
	}
	if err := identities.FinishDelete(ctx, sessionID, expectedPath); err != nil {
		return fmt.Errorf("finish recoverable session deletion: %w", bridgeIdentityConflict(err))
	}
	return nil
}
