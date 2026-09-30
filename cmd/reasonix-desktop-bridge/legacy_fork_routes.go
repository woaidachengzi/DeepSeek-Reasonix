package main

import (
	"net/http"

	"reasonix/internal/desktopbridge"
)

type legacyConversationForkResultResponse struct {
	ProtocolVersion int                                        `json:"protocolVersion"`
	Result          desktopbridge.LegacyConversationForkResult `json:"result"`
}

func (b *bridgeServer) legacyForkPreview(w http.ResponseWriter, r *http.Request) {
	var request conversationRewindPreviewRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid legacy fork preview request")
		return
	}
	plan, err := b.runtimes.PrepareLegacyConversationFork(r.PathValue("id"), request.Turn)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to prepare legacy conversation fork")
		return
	}
	writeJSON(w, http.StatusOK, conversationRewindPlanResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Plan: plan})
}

func (b *bridgeServer) legacyForkCommit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "legacy fork requires a request ID")
		return
	}
	var request conversationRewindCommitRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid legacy fork commit request")
		return
	}
	result, err := b.runtimes.CommitLegacyConversationFork(r.PathValue("id"), request.PlanID)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to create legacy conversation fork")
		return
	}
	writeJSON(w, http.StatusOK, legacyConversationForkResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}
