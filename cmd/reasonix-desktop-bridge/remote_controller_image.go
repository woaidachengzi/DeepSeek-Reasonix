package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionImageRequest struct {
	SessionPath string `json:"sessionPath"`
	Source      string `json:"source"`
}
type remoteControllerSessionImageResponse struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	Controller      remoteControllerView        `json:"controller"`
	View            controller.SessionImageView `json:"view"`
}

var remoteControllerImageSlots = make(chan struct{}, 2)

func (b *bridgeServer) remoteControllerSessionImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	if !controllerHandle(id) {
		writeProtocolError(w, 400, "invalid_request", "select a remote session image")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 32*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	select {
	case remoteControllerImageSlots <- struct{}{}:
		defer func() { <-remoteControllerImageSlots }()
	case <-ctx.Done():
		writeProtocolError(w, 409, "conflict", "remote image request expired; reopen the remote workspace")
		return
	}
	var input remoteControllerSessionImageRequest
	if decodeJSONBody(w, r, 24<<20, &input) != nil || input.SessionPath == "" || len(input.SessionPath) > 32768 || !utf8.ValidString(input.SessionPath) || strings.IndexFunc(input.SessionPath, unicode.IsControl) >= 0 || strings.TrimSpace(input.Source) == "" || len(input.Source) > controller.SessionImageSourceLimit || !utf8.ValidString(input.Source) || strings.ContainsRune(input.Source, 0) {
		writeProtocolError(w, 400, "invalid_request", "select a listed remote session image")
		return
	}
	if ctx.Err() != nil || b.remoteSessions.getController(id) != connection {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	// Workspace is resolved/owned by backend SSH bootstrap. It is deliberately
	// absent from the inbound DTO; a remote path never reaches RuntimeManager.
	view, err := connection.client.SessionImage(ctx, connection.view.Workspace, input.SessionPath, input.Source)
	if b.remoteSessions.getController(id) != connection || errors.Is(err, controller.ErrClosed) || ctx.Err() != nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if errors.Is(err, controller.ErrSessionNotListed) {
		writeProtocolError(w, 404, "not_found", "remote session is no longer listed; refresh remote sessions")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not preview remote image; upgrade remote Serve for session-image v1 and verify its session workspace")
		return
	}
	writeJSON(w, 200, remoteControllerSessionImageResponse{desktopbridge.ProtocolVersion, connection.view, view})
}
