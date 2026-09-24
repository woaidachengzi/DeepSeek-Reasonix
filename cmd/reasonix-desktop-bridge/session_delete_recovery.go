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
// After a process restart there is no owned runtime, but an identity fenced as
// deleting must still be able to finish its artifact sweep without reopening
// the session (which would risk creating a fresh transcript).
func (b *bridgeServer) deleteOwnedOrInterruptedSession(ctx context.Context, sessionID string) error {
	err := b.runtimes.DeleteSession(sessionID)
	if !errors.Is(err, desktopbridge.ErrSessionNotFound) {
		return err
	}
	return retryInterruptedSessionDelete(ctx, sessionID)
}

func retryInterruptedSessionDelete(ctx context.Context, sessionID string) error {
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
	identities, err := sessionidentity.Open(ctx, identityPath)
	if err != nil {
		return fmt.Errorf("open session identity for deletion recovery: %w", err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists || record.Path != expectedPath || record.State != sessionidentity.StateDeleting {
		return desktopbridge.ErrSessionNotFound
	}
	if err := control.RemoveSessionArtifacts(expectedPath); err != nil {
		return fmt.Errorf("resume session artifact deletion: %w", err)
	}
	if err := identities.FinishDelete(ctx, sessionID, expectedPath); err != nil {
		return fmt.Errorf("finish interrupted session deletion: %w", err)
	}
	return nil
}
