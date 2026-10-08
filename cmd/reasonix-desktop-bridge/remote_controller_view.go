package main

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionViewRequest struct {
	SessionPath string `json:"sessionPath"`
}
type remoteControllerSessionViewResponse struct {
	ProtocolVersion int                    `json:"protocolVersion"`
	Controller      remoteControllerView   `json:"controller"`
	View            controller.SessionView `json:"view"`
}

func controllerSessionPath(path string) bool {
	return path != "" && len(path) <= 32768 && utf8.ValidString(path) && strings.IndexFunc(path, unicode.IsControl) < 0
}

func (b *bridgeServer) remoteControllerSessionView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	var input remoteControllerSessionViewRequest
	if !controllerHandle(id) || decodeJSONBody(w, r, 200<<10, &input) != nil || !controllerSessionPath(input.SessionPath) {
		writeProtocolError(w, 400, "invalid_request", "select one listed remote session")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	view, err := connection.client.SessionView(r.Context(), input.SessionPath)
	if b.remoteSessions.getController(id) != connection || errors.Is(err, controller.ErrClosed) {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if errors.Is(err, controller.ErrSessionNotListed) {
		writeProtocolError(w, 404, "not_found", "remote session is no longer listed; refresh remote sessions")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not read remote session; upgrade remote Serve for session-view v1 with entry IDs and reopen the workspace")
		return
	}
	writeJSON(w, 200, remoteControllerSessionViewResponse{desktopbridge.ProtocolVersion, connection.view, view})
}
