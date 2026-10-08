package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"reasonix/internal/desktopbridge"
)

type terminalCreateRequest struct {
	Path    string `json:"path,omitempty"`
	ShellID string `json:"shellId,omitempty"`
}
type terminalInputRequest struct {
	Data string `json:"data"`
}
type terminalResizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}
type terminalRenameRequest struct {
	Title string `json:"title"`
}
type terminalWorkspaceResponse struct {
	ProtocolVersion int                                 `json:"protocolVersion"`
	Workspace       desktopbridge.TerminalWorkspaceView `json:"workspace"`
}
type terminalSessionResponse struct {
	ProtocolVersion int                               `json:"protocolVersion"`
	Terminal        desktopbridge.TerminalSessionView `json:"terminal"`
}
type terminalOutputResponse struct {
	ProtocolVersion int                              `json:"protocolVersion"`
	Output          desktopbridge.TerminalOutputView `json:"output"`
}
type terminalActionResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	TerminalID      string `json:"terminalId"`
	Accepted        bool   `json:"accepted"`
}

func (b *bridgeServer) terminalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/sessions/{id}/terminal", b.authorized(b.terminalWorkspace))
	mux.HandleFunc("POST /v1/sessions/{id}/terminal", b.authorized(b.idempotent(8<<10, b.terminalCreate)))
	mux.HandleFunc("GET /v1/sessions/{id}/terminal/{terminal}/output", b.authorized(b.terminalOutput))
	mux.HandleFunc("POST /v1/sessions/{id}/terminal/{terminal}/input", b.authorized(b.idempotent(96<<10, b.terminalInput)))
	mux.HandleFunc("POST /v1/sessions/{id}/terminal/{terminal}/resize", b.authorized(b.idempotent(4<<10, b.terminalResize)))
	mux.HandleFunc("PATCH /v1/sessions/{id}/terminal/{terminal}/title", b.authorized(b.idempotent(4<<10, b.terminalRename)))
	mux.HandleFunc("DELETE /v1/sessions/{id}/terminal/{terminal}", b.authorized(b.idempotent(4<<10, b.terminalClose)))
}

func (b *bridgeServer) terminalWorkspace(w http.ResponseWriter, r *http.Request) {
	view, err := b.runtimes.TerminalWorkspace(r.PathValue("id"))
	if err != nil {
		b.terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, terminalWorkspaceResponse{desktopbridge.ProtocolVersion, view})
}

func (b *bridgeServer) terminalCreate(w http.ResponseWriter, r *http.Request) {
	var request terminalCreateRequest
	if r.Header.Get(requestIDHeader) == "" || decodeJSONBody(w, r, 8<<10, &request) != nil || len(request.Path) > 4096 || len(request.ShellID) > 64 {
		b.terminalError(w, desktopbridge.ErrTerminalInput)
		return
	}
	view, err := b.runtimes.CreateTerminal(r.Context(), r.PathValue("id"), request.Path, request.ShellID)
	if err != nil {
		b.terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, terminalSessionResponse{desktopbridge.ProtocolVersion, view})
}

func (b *bridgeServer) terminalOutput(w http.ResponseWriter, r *http.Request) {
	view, err := b.runtimes.TerminalOutput(r.PathValue("id"), r.PathValue("terminal"))
	if err != nil {
		b.terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, terminalOutputResponse{desktopbridge.ProtocolVersion, view})
}

func (b *bridgeServer) terminalInput(w http.ResponseWriter, r *http.Request) {
	var request terminalInputRequest
	if r.Header.Get(requestIDHeader) == "" || decodeJSONBody(w, r, 96<<10, &request) != nil ||
		len(request.Data) > base64.StdEncoding.EncodedLen(desktopbridge.TerminalInputLimit) || strings.ContainsAny(request.Data, "\r\n") {
		b.terminalError(w, desktopbridge.ErrTerminalInput)
		return
	}
	data, err := base64.StdEncoding.Strict().DecodeString(request.Data)
	if err == nil {
		err = b.runtimes.WriteTerminal(r.PathValue("id"), r.PathValue("terminal"), data)
	}
	if err != nil {
		if _, ok := err.(base64.CorruptInputError); ok {
			err = desktopbridge.ErrTerminalInput
		}
		b.terminalError(w, err)
		return
	}
	b.terminalAccepted(w, r, http.StatusAccepted)
}

func (b *bridgeServer) terminalResize(w http.ResponseWriter, r *http.Request) {
	var request terminalResizeRequest
	if decodeJSONBody(w, r, 4<<10, &request) != nil {
		b.terminalError(w, desktopbridge.ErrTerminalInput)
		return
	}
	if err := b.runtimes.ResizeTerminal(r.PathValue("id"), r.PathValue("terminal"), request.Cols, request.Rows); err != nil {
		b.terminalError(w, err)
		return
	}
	b.terminalAccepted(w, r, http.StatusOK)
}

func (b *bridgeServer) terminalRename(w http.ResponseWriter, r *http.Request) {
	var request terminalRenameRequest
	if decodeJSONBody(w, r, 4<<10, &request) != nil {
		b.terminalError(w, desktopbridge.ErrTerminalInput)
		return
	}
	if err := b.runtimes.RenameTerminal(r.PathValue("id"), r.PathValue("terminal"), request.Title); err != nil {
		b.terminalError(w, err)
		return
	}
	b.terminalAccepted(w, r, http.StatusOK)
}

func (b *bridgeServer) terminalClose(w http.ResponseWriter, r *http.Request) {
	if err := b.runtimes.CloseTerminal(r.PathValue("id"), r.PathValue("terminal")); err != nil {
		b.terminalError(w, err)
		return
	}
	b.terminalAccepted(w, r, http.StatusOK)
}

func (b *bridgeServer) terminalAccepted(w http.ResponseWriter, r *http.Request, status int) {
	writeJSON(w, status, terminalActionResponse{desktopbridge.ProtocolVersion, r.PathValue("terminal"), true})
}

func (b *bridgeServer) terminalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, desktopbridge.ErrTerminalInput):
		writeProtocolError(w, http.StatusBadRequest, "terminal_invalid_request", "invalid terminal request; use a relative workspace directory and installed shell")
	case errors.Is(err, desktopbridge.ErrTerminalNotFound):
		writeProtocolError(w, http.StatusNotFound, "terminal_not_found", "terminal is no longer available; refresh the active session")
	case errors.Is(err, desktopbridge.ErrTerminalBusy):
		writeProtocolError(w, http.StatusConflict, "terminal_busy", "terminal limit reached; wait for input to drain or close an unused terminal")
	case errors.Is(err, desktopbridge.ErrTerminalUnavailable):
		writeProtocolError(w, http.StatusServiceUnavailable, "terminal_unavailable", "terminal is unavailable; check shell installation and workspace access")
	default:
		b.writeRuntimeError(w, err, "terminal request failed; reopen the active session")
	}
}
