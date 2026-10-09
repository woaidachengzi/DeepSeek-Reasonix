package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

// Keep the bridge wire DTO flat for schema conformance. Answer is validated
// by the shared discriminated decoder before selecting any remote connection.
type remoteControllerSessionPromptRequest struct {
	SessionPath        string          `json:"sessionPath"`
	RuntimeEpoch       string          `json:"runtimeEpoch"`
	TurnID             string          `json:"turnId"`
	PromptID           string          `json:"promptId"`
	PromptRuntimeEpoch string          `json:"promptRuntimeEpoch"`
	Kind               string          `json:"kind"`
	Answer             json.RawMessage `json:"answer"`
}

func (r remoteControllerSessionPromptRequest) shared() controller.SessionPromptRequest {
	return controller.SessionPromptRequest{SessionPromptScope: controller.SessionPromptScope{
		SessionPath: r.SessionPath, RuntimeEpoch: r.RuntimeEpoch, TurnID: r.TurnID,
		PromptID: r.PromptID, PromptRuntimeEpoch: r.PromptRuntimeEpoch, Kind: r.Kind,
	}, Answer: r.Answer}
}

type remoteControllerSessionPromptResponse struct {
	ProtocolVersion int                                  `json:"protocolVersion"`
	Controller      remoteControllerView                 `json:"controller"`
	Receipt         remoteControllerSessionPromptReceipt `json:"receipt"`
}

type remoteControllerSessionPromptReceipt struct {
	ProtocolVersion    int    `json:"protocolVersion"`
	SessionPath        string `json:"sessionPath"`
	RuntimeEpoch       string `json:"runtimeEpoch"`
	TurnID             string `json:"turnId"`
	PromptID           string `json:"promptId"`
	PromptRuntimeEpoch string `json:"promptRuntimeEpoch"`
	Kind               string `json:"kind"`
	Resolved           bool   `json:"resolved"`
}

func (b *bridgeServer) remoteControllerSessionPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	ctx, cancel := context.WithTimeout(r.Context(), 23*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input remoteControllerSessionPromptRequest
	if !controllerHandle(id) || r.URL.RawQuery != "" || decodeJSONBody(w, r, 256<<10, &input) != nil || !controllerSessionPath(input.SessionPath) {
		writeProtocolError(w, 400, "invalid_request", "select one current remote prompt and a valid decision")
		return
	}
	message := input.shared()
	if _, err := controller.DecodeSessionPromptAnswer(message); err != nil {
		writeProtocolError(w, 400, "invalid_request", "select one current remote prompt and a valid decision")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil || ctx.Err() != nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	// Saved SSH owner chooses the client/cookie/endpoint. Exactly one dispatch;
	// a revoked handle cannot publish a completed old decision as new success.
	receipt, err := connection.client.ResolveSessionPrompt(ctx, message)
	if errors.Is(err, controller.ErrPromptOutcomeUnknown) || b.remoteSessions.getController(id) != connection || ctx.Err() != nil {
		writeProtocolError(w, 502, "remote_prompt_unknown", "remote decision outcome is unknown; refresh the selected session and do not automatically retry")
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
	if errors.Is(err, controller.ErrPromptChanged) {
		writeProtocolError(w, 409, "conflict", "remote prompt changed; refresh before answering")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not answer remote prompt; refresh the selected session and verify remote Serve")
		return
	}
	writeJSON(w, 200, remoteControllerSessionPromptResponse{desktopbridge.ProtocolVersion, connection.view, remoteControllerSessionPromptReceipt{
		ProtocolVersion: receipt.ProtocolVersion, SessionPath: receipt.SessionPath, RuntimeEpoch: receipt.RuntimeEpoch,
		TurnID: receipt.TurnID, PromptID: receipt.PromptID, PromptRuntimeEpoch: receipt.PromptRuntimeEpoch, Kind: receipt.Kind, Resolved: receipt.Resolved,
	}})
}
