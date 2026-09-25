package main

import (
	"net/http"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

type pendingSessionDeleteEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type pendingSessionDeletesResponse struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	Sessions        []pendingSessionDeleteEntry `json:"sessions"`
}

func (b *bridgeServer) pendingSessionDeletes(w http.ResponseWriter, r *http.Request) {
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	exists, err := sessionidentity.IdentityDatabaseExists(identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect session deletion recovery state")
		return
	}
	if !exists {
		writeJSON(w, http.StatusOK, pendingSessionDeletesResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        []pendingSessionDeleteEntry{},
		})
		return
	}
	identities, err := sessionidentity.OpenReadOnly(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open session deletion recovery state")
		return
	}
	defer func() { _ = identities.Close() }()
	deletions, err := identities.ListPendingDeletes(r.Context())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read session deletion recovery state")
		return
	}
	entries := make([]pendingSessionDeleteEntry, 0, len(deletions))
	for _, deletion := range deletions {
		entries = append(entries, pendingSessionDeleteEntry{ID: deletion.ID, Title: deletion.Title})
	}
	writeJSON(w, http.StatusOK, pendingSessionDeletesResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        entries,
	})
}
