package main

import (
	"errors"
	"net/http"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

type syncSessionCatalogRequest struct {
	Sessions []sessionidentity.WorkbenchOrderEntry `json:"sessions"`
}

type syncSessionCatalogResponse struct {
	ProtocolVersion int `json:"protocolVersion"`
	Synced          int `json:"synced"`
}

func (b *bridgeServer) syncSessionCatalog(w http.ResponseWriter, r *http.Request) {
	var request syncSessionCatalogRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		return
	}
	if len(request.Sessions) > 50 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session catalog contains too many entries")
		return
	}
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(identityPath) == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	seen := make(map[string]struct{}, len(request.Sessions))
	for _, entry := range request.Sessions {
		if _, err := sessionpath.TranscriptPath(sessionDir, entry.ID); err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session catalog contains an invalid identifier")
			return
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session catalog contains duplicate identifiers")
			return
		}
		if len(entry.WorkspaceRoot) > 4096 {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session workspace path is too long")
			return
		}
		seen[entry.ID] = struct{}{}
	}
	identities, err := sessionidentity.Open(r.Context(), identityPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open the session identity store")
		return
	}
	defer func() { _ = identities.Close() }()
	synced, err := identities.SyncWorkbenchOrder(r.Context(), sessionDir, request.Sessions)
	if err != nil {
		if errors.Is(err, sessionidentity.ErrPathChanged) {
			writeProtocolError(w, http.StatusConflict, "session_conflict", "session catalog identity does not match")
			return
		}
		b.writeRuntimeError(w, err, "unable to sync the session catalog")
		return
	}
	writeJSON(w, http.StatusOK, syncSessionCatalogResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Synced:          synced,
	})
}
