package main

import (
	"context"
	"fmt"
	"os"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func registeredLegacySessionPath(sessionID string) (string, error) {
	if os.Getenv("REASONIX_TAURI_PROFILE_MODE") != "takeover" {
		return "", fmt.Errorf("%w: legacy sessions require the takeover installation", desktopbridge.ErrSessionConflict)
	}
	ctx := context.Background()
	identities, exists, err := sessionidentity.OpenReadOnlyIfExists(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		return "", err
	}
	if !exists {
		return "", desktopbridge.ErrSessionNotFound
	}
	defer identities.Close()
	record, found, err := identities.Get(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", desktopbridge.ErrSessionNotFound
	}
	bound, err := sessionpath.LegacyID(appconfig.SessionProfileRoot(), record.Path)
	if err != nil || bound != sessionID {
		return "", fmt.Errorf("%w: imported legacy path no longer matches its identity", sessionidentity.ErrPathChanged)
	}
	return record.Path, nil
}
