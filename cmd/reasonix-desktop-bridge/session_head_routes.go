package main

import (
	"net/http"

	"reasonix/internal/desktopbridge"
)

type sessionHeadsResponse struct {
	ProtocolVersion int                             `json:"protocolVersion"`
	Heads           []desktopbridge.SessionHeadView `json:"heads"`
}

type sessionHeadSwitchRequest struct {
	HeadID string `json:"headId"`
}

type sessionHeadSwitchResponse struct {
	ProtocolVersion int  `json:"protocolVersion"`
	OK              bool `json:"ok"`
}

func (b *bridgeServer) sessionHeads(w http.ResponseWriter, r *http.Request) {
	heads, err := b.runtimes.SessionHeads(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list conversation versions")
		return
	}
	writeJSON(w, http.StatusOK, sessionHeadsResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Heads: heads})
}

func (b *bridgeServer) sessionHeadSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "conversation version switch requires a request ID")
		return
	}
	var request sessionHeadSwitchRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid conversation version switch request")
		return
	}
	if err := b.runtimes.SwitchSessionHead(r.PathValue("id"), request.HeadID); err != nil {
		b.writeRuntimeError(w, err, "unable to switch conversation version")
		return
	}
	writeJSON(w, http.StatusOK, sessionHeadSwitchResponse{ProtocolVersion: desktopbridge.ProtocolVersion, OK: true})
}
