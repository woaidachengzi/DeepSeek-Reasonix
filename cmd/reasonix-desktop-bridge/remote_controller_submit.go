package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionSubmitResponse struct {
	ProtocolVersion int                             `json:"protocolVersion"`
	Controller      remoteControllerView            `json:"controller"`
	Receipt         controller.SessionSubmitReceipt `json:"receipt"`
}

func (b *bridgeServer) remoteControllerSessionSubmit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	ctx, cancel := context.WithTimeout(r.Context(), 23*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input controller.SessionSubmitRequest
	if !controllerHandle(id) || r.URL.RawQuery != "" || decodeJSONBody(w, r, 1<<20, &input) != nil || !controllerSessionPath(input.SessionPath) || !remoteCancelIdentity(input.RuntimeEpoch) || input.Revision == 0 || input.Revision > 9_007_199_254_740_991 ||
		strings.TrimSpace(input.Text) == "" || len(input.Text) > 512<<10 || !utf8.ValidString(input.Text) || strings.ContainsRune(input.Text, 0) {
		writeProtocolError(w, 400, "invalid_request", "select one idle remote session and enter a message")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil || ctx.Err() != nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	// Only this backend SSH owner chooses the client, cookie and endpoint. A
	// connection replacement cannot publish the old send receipt as success.
	receipt, err := connection.client.SubmitSessionTurn(ctx, input.Scope(), input.Text)
	if errors.Is(err, controller.ErrSubmitOutcomeUnknown) || b.remoteSessions.getController(id) != connection || ctx.Err() != nil {
		writeProtocolError(w, 502, "remote_submit_unknown", "remote send outcome is unknown; refresh the selected session and do not automatically retry")
		return
	}
	if errors.Is(err, controller.ErrClosed) {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if errors.Is(err, controller.ErrSessionNotListed) {
		writeProtocolError(w, 404, "not_found", "remote session is no longer listed; refresh remote sessions")
		return
	}
	if errors.Is(err, controller.ErrTurnSubmitChanged) {
		writeProtocolError(w, 409, "conflict", "remote session changed; refresh before sending")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not send remote message; refresh the selected session and verify remote Serve")
		return
	}
	writeJSON(w, 200, remoteControllerSessionSubmitResponse{desktopbridge.ProtocolVersion, connection.view, receipt})
}
