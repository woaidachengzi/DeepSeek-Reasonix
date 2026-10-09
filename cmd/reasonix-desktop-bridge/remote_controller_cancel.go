package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionCancelResponse struct {
	ProtocolVersion int                             `json:"protocolVersion"`
	Controller      remoteControllerView            `json:"controller"`
	Receipt         controller.SessionCancelReceipt `json:"receipt"`
}

func (b *bridgeServer) remoteControllerSessionCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var scope controller.SessionCancelScope
	if !controllerHandle(id) || r.URL.RawQuery != "" || decodeJSONBody(w, r, 40<<10, &scope) != nil || !controllerSessionPath(scope.SessionPath) || !remoteCancelIdentity(scope.RuntimeEpoch) || !remoteCancelIdentity(scope.TurnID) {
		writeProtocolError(w, 400, "invalid_request", "select one current remote turn to stop")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil || ctx.Err() != nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	// Only the backend-owned SSH connection chooses the HTTP client, endpoint,
	// cookie and owner lifetime. No local foreground cancellation is involved.
	receipt, err := connection.client.CancelSessionTurn(ctx, scope)
	if errors.Is(err, controller.ErrCancelOutcomeUnknown) || b.remoteSessions.getController(id) != connection || ctx.Err() != nil {
		writeProtocolError(w, 502, "remote_cancel_unknown", "remote stop outcome is unknown; refresh the selected session and do not automatically retry")
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
	if errors.Is(err, controller.ErrTurnCancelChanged) {
		writeProtocolError(w, 409, "conflict", "remote turn changed; refresh before stopping")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not stop remote turn; refresh the selected session and verify remote Serve")
		return
	}
	writeJSON(w, 200, remoteControllerSessionCancelResponse{desktopbridge.ProtocolVersion, connection.view, receipt})
}

func remoteCancelIdentity(value string) bool {
	return len(value) <= 4096 && controllerSessionPath(value)
}
