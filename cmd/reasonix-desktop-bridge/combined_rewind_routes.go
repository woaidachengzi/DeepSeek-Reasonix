package main

import (
	"net/http"

	"reasonix/internal/desktopbridge"
)

type combinedRewindPreviewRequest struct {
	Turn int `json:"turn"`
}

type combinedRewindCommitRequest struct {
	PlanID                 string `json:"planId"`
	ConfirmPartialCoverage bool   `json:"confirmPartialCoverage"`
}

type combinedRewindPlanResponse struct {
	ProtocolVersion int                                       `json:"protocolVersion"`
	Plan            desktopbridge.WorkspaceCombinedRewindPlan `json:"plan"`
}

type combinedRewindResultResponse struct {
	ProtocolVersion int                                         `json:"protocolVersion"`
	Result          desktopbridge.WorkspaceCombinedRewindResult `json:"result"`
}

func (b *bridgeServer) combinedRewindPreview(w http.ResponseWriter, r *http.Request) {
	var request combinedRewindPreviewRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid combined rewind preview request")
		return
	}
	plan, err := b.runtimes.PrepareCombinedRewind(r.PathValue("id"), request.Turn)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to prepare combined rewind")
		return
	}
	writeJSON(w, http.StatusOK, combinedRewindPlanResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Plan: plan})
}

func (b *bridgeServer) combinedRewindCommit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "combined rewind requires a request ID")
		return
	}
	var request combinedRewindCommitRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid combined rewind commit request")
		return
	}
	result, err := b.runtimes.CommitCombinedRewind(r.PathValue("id"), request.PlanID, request.ConfirmPartialCoverage)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to commit combined rewind")
		return
	}
	writeJSON(w, http.StatusOK, combinedRewindResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}
