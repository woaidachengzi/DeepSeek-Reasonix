package main

import (
	"net/http"
	"strconv"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
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

type pendingSessionDeletesPageResponse struct {
	ProtocolVersion int                             `json:"protocolVersion"`
	Sessions        []pendingSessionDeletePageEntry `json:"sessions"`
	NextCursor      *pendingSessionDeleteCursor     `json:"nextCursor,omitempty"`
}

type pendingSessionDeletePageEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type pendingSessionDeleteCursor struct {
	ID string `json:"id"`
}

func (b *bridgeServer) pendingSessionDeletes(w http.ResponseWriter, r *http.Request) {
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
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
	defer closeIdentity()
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

// pendingSessionDeletesPage is the bounded continuation route used by current
// Tauri hosts. The legacy no-query recovery route remains for older hosts.
func (b *bridgeServer) pendingSessionDeletesPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := sessionidentity.MaxPendingDeletePageSize
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > sessionidentity.MaxPendingDeletePageSize {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "pending deletion page limit is invalid")
			return
		}
		limit = parsed
	}

	rawID := query.Get("cursorId")
	var cursor *sessionidentity.PendingDeleteCursor
	if query.Has("cursorPosition") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "pending deletion page cursor is invalid")
		return
	}
	if rawID != "" {
		if strings.TrimSpace(rawID) == "" || !sessionpath.ValidID(rawID) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "pending deletion page cursor is invalid")
			return
		}
		cursor = &sessionidentity.PendingDeleteCursor{ID: rawID}
	}

	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect session deletion recovery state")
		return
	}
	if !exists {
		writeJSON(w, http.StatusOK, pendingSessionDeletesPageResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        []pendingSessionDeletePageEntry{},
		})
		return
	}
	defer closeIdentity()
	page, err := identities.ListPendingDeletesPage(r.Context(), limit, cursor)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read the requested deletion recovery page")
		return
	}
	entries := make([]pendingSessionDeletePageEntry, 0, len(page.Sessions))
	for _, deletion := range page.Sessions {
		entries = append(entries, pendingSessionDeletePageEntry{
			ID: deletion.ID, Title: deletion.Title,
		})
	}
	var nextCursor *pendingSessionDeleteCursor
	if page.NextCursor != nil {
		nextCursor = &pendingSessionDeleteCursor{ID: page.NextCursor.ID}
	}
	writeJSON(w, http.StatusOK, pendingSessionDeletesPageResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        entries,
		NextCursor:      nextCursor,
	})
}
