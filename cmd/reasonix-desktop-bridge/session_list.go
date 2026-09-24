package main

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

type sessionListEntry struct {
	ID            string                       `json:"id"`
	Title         string                       `json:"title"`
	TitleSource   sessionidentity.TitleSource  `json:"titleSource"`
	WorkspaceRoot string                       `json:"workspaceRoot,omitempty"`
	State         sessionidentity.SessionState `json:"state"`
	Missing       bool                         `json:"missing"`
	Position      int                          `json:"position"`
	UpdatedAtMS   int64                        `json:"updatedAtMs"`
}

type sessionListResponse struct {
	ProtocolVersion int                     `json:"protocolVersion"`
	Sessions        []sessionListEntry      `json:"sessions"`
	NextCursor      *sessionidentity.Cursor `json:"nextCursor,omitempty"`
	Total           int                     `json:"total"`
}

func (b *bridgeServer) sessionList(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := sessionidentity.MaxVisiblePageSize
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > sessionidentity.MaxVisiblePageSize {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session page limit is invalid")
			return
		}
		limit = parsed
	}

	var cursor *sessionidentity.Cursor
	rawPosition, rawID := query.Get("cursorPosition"), query.Get("cursorId")
	if rawPosition != "" || rawID != "" {
		position, err := strconv.Atoi(rawPosition)
		if err != nil || position < 0 || strings.TrimSpace(rawID) == "" {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session page cursor is invalid")
			return
		}
		cursor = &sessionidentity.Cursor{Position: position, ID: rawID}
	}
	workspaceRoot := query.Get("workspaceRoot")
	if len(workspaceRoot) > 4096 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workspace filter is too long")
		return
	}

	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	info, err := os.Lstat(identityPath)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, sessionListResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Sessions: []sessionListEntry{}})
		return
	}
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect the session identity store")
		return
	}
	if !info.Mode().IsRegular() {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "session identity store is not a regular file")
		return
	}
	identities, err := sessionidentity.OpenReadOnly(r.Context(), identityPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open the session identity store")
		return
	}
	defer func() { _ = identities.Close() }()

	page, err := identities.ListVisible(r.Context(), limit, cursor, workspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	entries := make([]sessionListEntry, 0, len(page.Records))
	for _, record := range page.Records {
		entries = append(entries, sessionListEntry{
			ID: record.ID, Title: record.Title, TitleSource: record.TitleSource,
			WorkspaceRoot: record.WorkspaceRoot, State: record.State, Missing: record.Missing,
			Position: record.Position, UpdatedAtMS: record.UpdatedAtMS,
		})
	}
	writeJSON(w, http.StatusOK, sessionListResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        entries,
		NextCursor:      page.NextCursor,
		Total:           page.Total,
	})
}
