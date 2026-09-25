package main

import (
	"errors"
	"net/http"
	"os"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

type pendingSessionTitleRecoveryEntry struct {
	ID            string                       `json:"id"`
	Title         string                       `json:"title"`
	WorkspaceRoot string                       `json:"workspaceRoot,omitempty"`
	State         sessionidentity.SessionState `json:"state"`
}

type pendingSessionTitleRecoveriesResponse struct {
	ProtocolVersion int                                `json:"protocolVersion"`
	Sessions        []pendingSessionTitleRecoveryEntry `json:"sessions"`
}

// pendingSessionTitleRecoveries exposes an explicit reopening path for
// unfinished user renames outside the host's bounded recent-session catalog.
// The request is read-only and never includes a transcript or sidecar path.
func (b *bridgeServer) pendingSessionTitleRecoveries(w http.ResponseWriter, r *http.Request) {
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	info, err := os.Lstat(identityPath)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, pendingSessionTitleRecoveriesResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        []pendingSessionTitleRecoveryEntry{},
		})
		return
	}
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect session title recovery state")
		return
	}
	if !info.Mode().IsRegular() {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "session identity store is not a regular file")
		return
	}
	identities, err := sessionidentity.OpenReadOnly(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open session title recovery state")
		return
	}
	defer func() { _ = identities.Close() }()
	recoveries, err := identities.ListPendingManualTitleRecoveries(r.Context())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read session title recovery state")
		return
	}
	entries := make([]pendingSessionTitleRecoveryEntry, 0, len(recoveries))
	for _, recovery := range recoveries {
		entries = append(entries, pendingSessionTitleRecoveryEntry{
			ID: recovery.ID, Title: recovery.Title, WorkspaceRoot: recovery.WorkspaceRoot, State: recovery.State,
		})
	}
	writeJSON(w, http.StatusOK, pendingSessionTitleRecoveriesResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        entries,
	})
}
