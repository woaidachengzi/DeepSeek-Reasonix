package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
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
	SnapshotID      string                  `json:"snapshotId"`
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
	rawSnapshotID := query.Get("cursorSnapshot")
	rawTotal := query.Get("cursorTotal")
	if rawPosition != "" || rawID != "" || rawTotal != "" {
		position, err := strconv.Atoi(rawPosition)
		if err != nil || position < 0 || strings.TrimSpace(rawID) == "" {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session page cursor is invalid")
			return
		}
		total := 0
		if rawTotal != "" {
			total, err = strconv.Atoi(rawTotal)
			if err != nil || total < 1 || total > sessionidentity.MaxVisibleSnapshotSize {
				writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session page cursor is invalid")
				return
			}
		}
		cursor = &sessionidentity.Cursor{Position: position, ID: rawID, SnapshotID: rawSnapshotID, Total: total}
	} else if rawSnapshotID != "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session page cursor is invalid")
		return
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
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect the session identity store")
		return
	}
	if !exists {
		if cursor != nil && cursor.SnapshotID != "" {
			writeProtocolError(w, http.StatusConflict, "resync_required", "session directory changed while paging")
			return
		}
		writeJSON(w, http.StatusOK, sessionListResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Sessions: []sessionListEntry{}, SnapshotID: emptyVisibleSnapshotID()})
		return
	}
	defer closeIdentity()

	page, err := identities.ListVisible(r.Context(), limit, cursor, workspaceRoot)
	if err != nil {
		if errors.Is(err, sessionidentity.ErrDirectoryChanged) {
			writeProtocolError(w, http.StatusConflict, "resync_required", "session directory changed while paging")
			return
		}
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessionListResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        sessionListEntries(page.Records),
		NextCursor:      page.NextCursor,
		Total:           page.Total,
		SnapshotID:      page.SnapshotID,
	})
}

func (b *bridgeServer) sessionDirectorySnapshot(w http.ResponseWriter, r *http.Request) {
	workspaceRoot := r.URL.Query().Get("workspaceRoot")
	if len(workspaceRoot) > 4096 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workspace filter is too long")
		return
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect the session identity store")
		return
	}
	if !exists {
		writeJSON(w, http.StatusOK, sessionListResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        []sessionListEntry{},
			SnapshotID:      emptyVisibleSnapshotID(),
		})
		return
	}
	defer closeIdentity()

	snapshot, err := identities.ListVisibleSnapshot(r.Context(), sessionidentity.MaxVisibleSnapshotSize, workspaceRoot)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read the session directory snapshot")
		return
	}
	writeJSON(w, http.StatusOK, sessionListResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sessions:        sessionListEntries(snapshot.Records),
		Total:           snapshot.Total,
		SnapshotID:      snapshot.SnapshotID,
	})
}

func sessionListEntries(records []sessionidentity.Record) []sessionListEntry {
	entries := make([]sessionListEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, sessionListEntry{
			ID: record.ID, Title: record.Title, TitleSource: record.TitleSource,
			WorkspaceRoot: record.WorkspaceRoot, State: record.State, Missing: record.Missing,
			Position: record.Position, UpdatedAtMS: record.UpdatedAtMS,
		})
	}
	return entries
}

func emptyVisibleSnapshotID() string {
	digest := sha256.Sum256([]byte("reasonix-session-directory-v2\x00"))
	return hex.EncodeToString(digest[:])
}
