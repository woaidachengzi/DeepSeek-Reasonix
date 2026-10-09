package main

import (
	"errors"
	"net/http"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionProjectionResponse struct {
	ProtocolVersion int                          `json:"protocolVersion"`
	Controller      remoteControllerView         `json:"controller"`
	Projection      controller.SessionProjection `json:"projection"`
	NextPage        string                       `json:"nextPage,omitempty"`
}

type remoteControllerProjectionRequest struct {
	SessionPath  string `json:"sessionPath"`
	Continuation string `json:"continuation,omitempty"`
}

type remoteProjectionPage struct {
	path     string
	deadline time.Time
	cursor   controller.ProjectionCursor
	next     string
}

const maxRemoteProjectionPages = 64

func (c *remoteControllerConnection) projectionPage(handle, path string, now time.Time) (remoteProjectionPage, bool) {
	c.projectionMu.Lock()
	defer c.projectionMu.Unlock()
	for id, page := range c.projectionPages {
		if !now.Before(page.deadline) {
			delete(c.projectionPages, id)
		}
	}
	page, ok := c.projectionPages[handle]
	return page, ok && page.path == path
}

func (c *remoteControllerConnection) retainProjectionPage(page remoteProjectionPage, parent string) (string, error) {
	handle, err := randomID()
	if err != nil {
		return "", err
	}
	c.projectionMu.Lock()
	defer c.projectionMu.Unlock()
	if c.projectionPages == nil {
		c.projectionPages = make(map[string]remoteProjectionPage)
	}
	now := time.Now()
	for id, cached := range c.projectionPages {
		if !now.Before(cached.deadline) {
			delete(c.projectionPages, id)
		}
	}
	if !now.Before(page.deadline) {
		return "", controller.ErrProjectionReconcile
	}
	if parent != "" {
		previous, ok := c.projectionPages[parent]
		if !ok || previous.path != page.path || !previous.deadline.Equal(page.deadline) {
			return "", controller.ErrProjectionReconcile
		}
		if previous.next != "" {
			return previous.next, nil
		}
	}
	if len(c.projectionPages) >= maxRemoteProjectionPages {
		return "", controller.ErrProjectionReconcile
	}
	if _, exists := c.projectionPages[handle]; exists {
		return "", controller.ErrProjectionReconcile
	}
	c.projectionPages[handle] = page
	if parent != "" {
		previous := c.projectionPages[parent]
		previous.next = handle
		c.projectionPages[parent] = previous
	}
	return handle, nil
}

// This is a bounded display cut only. It does not adopt a saved
// session, replay a model request, or authorize an approval. The caller must
// establish the live subscription before reading a cut; receipt is not ready.
// Continuation resolves host-held metadata, never caller-supplied history,
// server page tokens, URL or cursor. Original deadlines are never extended.
func (b *bridgeServer) remoteControllerSessionProjection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	var input remoteControllerProjectionRequest
	if !controllerHandle(id) || r.URL.RawQuery != "" || r.Header.Get("Last-Event-ID") != "" || decodeJSONBody(w, r, 200<<10, &input) != nil || !controllerSessionPath(input.SessionPath) || (input.Continuation != "" && !controllerHandle(input.Continuation)) {
		writeProtocolError(w, 400, "invalid_request", "select one listed remote session without replay parameters")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	deadline := time.Now().Add(2 * time.Minute)
	var projection controller.SessionProjection
	var err error
	if input.Continuation == "" {
		projection, err = connection.client.SessionProjection(r.Context(), input.SessionPath)
	} else {
		page, ok := connection.projectionPage(input.Continuation, input.SessionPath, time.Now())
		if !ok {
			writeProtocolError(w, 409, "projection_reconcile", "remote snapshot changed or expired; reconcile the selected session")
			return
		}
		deadline = page.deadline
		projection, err = connection.client.SessionProjectionNext(r.Context(), input.SessionPath, page.cursor)
	}
	if b.remoteSessions.getController(id) != connection || errors.Is(err, controller.ErrClosed) {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if errors.Is(err, controller.ErrSessionNotListed) {
		writeProtocolError(w, 404, "not_found", "remote session is no longer listed; refresh remote sessions")
		return
	}
	if errors.Is(err, controller.ErrProjectionReconcile) {
		writeProtocolError(w, 409, "projection_reconcile", "remote snapshot changed or expired; reconcile the selected session")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not read remote snapshot; verify remote Serve projection support and reconcile the selected session")
		return
	}
	var next string
	if projection.Replay.HasMore {
		cursor, cursorErr := connection.client.ProjectionCursor(projection)
		if cursorErr == nil {
			next, cursorErr = connection.retainProjectionPage(remoteProjectionPage{path: input.SessionPath, deadline: deadline, cursor: cursor}, input.Continuation)
		}
		if cursorErr != nil {
			writeProtocolError(w, 409, "projection_reconcile", "remote snapshot continuation unavailable; reconcile the selected session")
			return
		}
	}
	if b.remoteSessions.getController(id) != connection || r.Context().Err() != nil || !time.Now().Before(deadline) {
		writeProtocolError(w, 409, "projection_reconcile", "remote snapshot changed or expired; reconcile the selected session")
		return
	}
	projection.PageToken = "" // Serve continuation never leaves the host.
	writeJSON(w, 200, remoteControllerSessionProjectionResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Controller: connection.view, Projection: projection, NextPage: next})
}
