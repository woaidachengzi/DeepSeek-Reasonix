package main

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

type firstMessageTitle struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

type backfillSessionTitlesRequest struct {
	Titles []firstMessageTitle `json:"titles"`
}

type backfillSessionTitlesResponse struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Titles          []firstMessageTitle `json:"titles"`
}

// backfillSessionTitles copies only first-message fallback titles into the
// identity directory. Explicit or previously generated titles are immutable
// through this migration path.
func (b *bridgeServer) backfillSessionTitles(w http.ResponseWriter, r *http.Request) {
	var request backfillSessionTitlesRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		return
	}
	if len(request.Titles) > 50 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "too many session titles")
		return
	}
	if len(request.Titles) == 0 {
		writeJSON(w, http.StatusOK, backfillSessionTitlesResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Titles:          []firstMessageTitle{},
		})
		return
	}
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if sessionDir == "" || identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	seen := make(map[string]struct{}, len(request.Titles))
	for _, item := range request.Titles {
		if _, err := sessionpath.TranscriptPath(sessionDir, item.SessionID); err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session title contains an invalid identifier")
			return
		}
		if _, duplicate := seen[item.SessionID]; duplicate {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session title contains a duplicate identifier")
			return
		}
		if strings.TrimSpace(item.Title) == "" || utf8.RuneCountInString(item.Title) > 120 ||
			strings.IndexFunc(item.Title, unicode.IsControl) >= 0 {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "session title is invalid")
			return
		}
		seen[item.SessionID] = struct{}{}
	}
	identities, err := sessionidentity.Open(r.Context(), identityPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open the session identity store")
		return
	}
	defer func() { _ = identities.Close() }()
	resolved := make([]firstMessageTitle, 0, len(request.Titles))
	for _, item := range request.Titles {
		record, exists, err := identities.Get(r.Context(), item.SessionID)
		if err != nil {
			b.writeRuntimeError(w, err, "unable to read the session identity store")
			return
		}
		if !exists || record.State == sessionidentity.StateDeleting || record.State == sessionidentity.StateDeleted {
			continue
		}
		if record.Title == "" && record.TitleSource == sessionidentity.TitleFallback {
			if err := identities.SetTitle(r.Context(), item.SessionID, record.TitleRevision, item.Title, sessionidentity.TitleFirstMessage); err != nil {
				b.writeRuntimeError(w, err, "unable to update a session identity title")
				return
			}
			record.Title = item.Title
		}
		if record.Title != "" {
			resolved = append(resolved, firstMessageTitle{SessionID: item.SessionID, Title: record.Title})
		}
	}
	writeJSON(w, http.StatusOK, backfillSessionTitlesResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Titles:          resolved,
	})
}
