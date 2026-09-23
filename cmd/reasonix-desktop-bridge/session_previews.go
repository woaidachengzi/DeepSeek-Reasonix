package main

import (
	"net/http"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

// Preview titles are read without opening or switching the bridge's live
// controller. Only the first user-visible message is projected; assistant and
// tool content must never be sent to the workbench catalog.
type sessionPreview struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
	FirstUser string `json:"firstUser,omitempty"`
}

type sessionPreviewsRequest struct {
	SessionIDs []string `json:"sessionIds"`
}

func (b *bridgeServer) sessionPreviews(w http.ResponseWriter, r *http.Request) {
	var request sessionPreviewsRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil || len(request.SessionIDs) > 50 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid session previews request")
		return
	}
	previews := make([]sessionPreview, 0, len(request.SessionIDs))
	for _, id := range request.SessionIDs {
		path, err := bridgeSessionPath(appconfig.SessionDir(), id)
		if err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid session identifier")
			return
		}
		previews = append(previews, readSessionPreview(path, id))
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "previews": previews})
}

func readSessionPreview(path, id string) sessionPreview {
	result := sessionPreview{SessionID: id}
	if meta, ok, err := agent.LoadBranchMeta(path); err == nil && ok {
		result.Title = strings.TrimSpace(meta.CustomTitle)
	}
	if result.Title != "" {
		return result
	}
	loaded, err := agent.LoadSession(filepath.Clean(path))
	if err != nil {
		return result
	}
	for _, message := range loaded.Snapshot() {
		if message.Role != provider.RoleUser || !bridgeHistoryMessageVisible(message) {
			continue
		}
		content := strings.TrimSpace(bridgeHistoryMessageContent(message))
		if content != "" {
			runes := []rune(content)
			result.FirstUser = string(runes[:min(len(runes), 300)])
			break
		}
	}
	return result
}
