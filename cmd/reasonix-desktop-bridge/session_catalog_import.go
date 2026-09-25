package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

type legacyCatalogEntry struct {
	SessionID     string  `json:"sessionId"`
	Title         *string `json:"title"`
	WorkspaceRoot *string `json:"workspaceRoot"`
}

type importLegacyCatalogRequest struct {
	Sessions []legacyCatalogEntry `json:"sessions"`
}

type importLegacyCatalogResponse struct {
	ProtocolVersion int `json:"protocolVersion"`
	Accepted        int `json:"accepted"`
}

func (b *bridgeServer) importLegacyCatalog(w http.ResponseWriter, r *http.Request) {
	var request importLegacyCatalogRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid legacy session catalog request")
		return
	}
	if len(request.Sessions) > 50 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy catalog contains too many sessions")
		return
	}
	if len(request.Sessions) == 0 {
		writeJSON(w, http.StatusOK, importLegacyCatalogResponse{ProtocolVersion: desktopbridge.ProtocolVersion})
		return
	}
	sessionDir := appconfig.SessionDir()
	identityPath := appconfig.DesktopSessionIdentityPath()
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(identityPath) == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	candidates := make([]sessionidentity.Candidate, 0, len(request.Sessions))
	for position, entry := range request.Sessions {
		if entry.Title != nil && !validCatalogTitle(*entry.Title) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy catalog contains an invalid title")
			return
		}
		if entry.WorkspaceRoot != nil && !validCatalogWorkspace(*entry.WorkspaceRoot) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy catalog contains an invalid workspace path")
			return
		}
		path, err := sessionpath.TranscriptPath(sessionDir, entry.SessionID)
		if err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy catalog contains an invalid session identifier")
			return
		}
		workspaceRoot, title := "", ""
		if entry.WorkspaceRoot != nil {
			workspaceRoot = strings.TrimSpace(*entry.WorkspaceRoot)
			if workspaceRoot != "" {
				workspaceRoot, err = filepath.Abs(workspaceRoot)
				if err != nil {
					writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy catalog contains an invalid workspace path")
					return
				}
			}
		}
		if entry.Title != nil {
			title = *entry.Title
		}
		candidates = append(candidates, sessionidentity.Candidate{
			ID: entry.SessionID, Path: path, WorkspaceRoot: workspaceRoot,
			Title: title, Position: position,
		})
	}
	identities, err := sessionidentity.Open(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open the session identity store")
		return
	}
	defer func() { _ = identities.Close() }()
	if err := identities.ImportLegacyCatalog(r.Context(), sessionDir, candidates); err != nil {
		writeProtocolError(w, http.StatusConflict, "session_conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, importLegacyCatalogResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Accepted:        len(candidates),
	})
}

// Match the host catalog's metadata bounds before a request can open or
// mutate the identity store. An absent/empty legacy title is still valid.
func validCatalogTitle(title string) bool {
	return utf8.ValidString(title) && utf8.RuneCountInString(title) <= 120 &&
		strings.IndexFunc(title, unicode.IsControl) < 0
}

func validCatalogWorkspace(root string) bool {
	return utf8.ValidString(root) && len(root) <= 4096 &&
		strings.IndexFunc(root, unicode.IsControl) < 0
}
