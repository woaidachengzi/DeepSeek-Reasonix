package main

import (
	"net/http"

	"reasonix/internal/desktopbridge"
)

type conversationRewindPreviewRequest struct {
	Turn int `json:"turn"`
}

type conversationRewindCommitRequest struct {
	PlanID string `json:"planId"`
}

type conversationRewindUndoRequest struct {
	HeadID string `json:"headId"`
}

type conversationRewindPlanResponse struct {
	ProtocolVersion int                                           `json:"protocolVersion"`
	Plan            desktopbridge.WorkspaceConversationRewindPlan `json:"plan"`
}

type conversationRewindResultResponse struct {
	ProtocolVersion int                                             `json:"protocolVersion"`
	Result          desktopbridge.WorkspaceConversationRewindResult `json:"result"`
}

func (b *bridgeServer) conversationRewindPreview(w http.ResponseWriter, r *http.Request) {
	var request conversationRewindPreviewRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid conversation rewind preview request")
		return
	}
	plan, err := b.runtimes.PrepareConversationRewind(r.PathValue("id"), request.Turn)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to prepare conversation rewind")
		return
	}
	writeJSON(w, http.StatusOK, conversationRewindPlanResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Plan: plan})
}

func (b *bridgeServer) conversationRewindCommit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "conversation rewind requires a request ID")
		return
	}
	var request conversationRewindCommitRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid conversation rewind commit request")
		return
	}
	result, err := b.runtimes.CommitConversationRewind(r.PathValue("id"), request.PlanID)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to commit conversation rewind")
		return
	}
	writeJSON(w, http.StatusOK, conversationRewindResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}

func (b *bridgeServer) conversationRewindUndo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "conversation rewind undo requires a request ID")
		return
	}
	var request conversationRewindUndoRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid conversation rewind undo request")
		return
	}
	result, err := b.runtimes.UndoConversationRewind(r.PathValue("id"), request.HeadID)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to undo conversation rewind")
		return
	}
	writeJSON(w, http.StatusOK, conversationRewindResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}
