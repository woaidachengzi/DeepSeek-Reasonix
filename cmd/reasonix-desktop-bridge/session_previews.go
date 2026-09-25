package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
	sessionstore "reasonix/internal/store"
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
	sessionDir := appconfig.SessionDir()
	profileRoot := appconfig.SessionProfileRoot()
	previewDirectorySafe, err := previewDirectoryWithinProfile(profileRoot, sessionDir)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "session_preview_unavailable", "session preview is unavailable")
		return
	}
	for _, id := range request.SessionIDs {
		path, err := bridgeSessionPath(sessionDir, id)
		if err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid session identifier")
			return
		}
		preview := sessionPreview{SessionID: id}
		if previewDirectorySafe {
			preview = readSessionPreview(path, id)
		}
		previews = append(previews, preview)
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "previews": previews})
}

func previewDirectoryWithinProfile(profileRoot, sessionDir string) (bool, error) {
	root, err := filepath.EvalSymlinks(profileRoot)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	resolvedSessionDir, err := filepath.EvalSymlinks(sessionDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(root, resolvedSessionDir)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func readSessionPreview(path, id string) sessionPreview {
	result := sessionPreview{SessionID: id}
	transcriptInfo, err := os.Lstat(path)
	if err != nil || !transcriptInfo.Mode().IsRegular() {
		return result
	}
	metaPath := sessionstore.SessionMeta(path)
	if metaInfo, err := os.Lstat(metaPath); err == nil && metaInfo.Mode().IsRegular() {
		if meta, ok, err := agent.LoadBranchMeta(path); err == nil && ok {
			result.Title = strings.TrimSpace(meta.CustomTitle)
		}
		if result.Title == "" {
			// Avoid replaying a large transcript for the common case. The cached
			// first user projection can be a synthetic session-context message;
			// fall through to full parsing then so it never becomes the title.
			if preview, _, ok := agent.SessionPreviewCached(path); ok &&
				!strings.HasPrefix(strings.TrimSpace(preview), "<session-context") {
				result.FirstUser = preview
				return result
			}
		}
	}
	if result.Title != "" {
		return result
	}
	if eventInfo, err := os.Lstat(sessionstore.SessionEventLog(path)); err == nil {
		if !eventInfo.Mode().IsRegular() {
			return result
		}
	} else if !os.IsNotExist(err) {
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
