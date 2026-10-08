package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

type remoteControllerSessionEvent struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Controller      remoteControllerView `json:"controller"`
	SessionPath     string               `json:"sessionPath"`
	Event           eventwire.Event      `json:"event"`
}

// A separate non-replayable stream, not the local session EventLedger. A native
// consumer must also fence delivery by handle/session/subscription generation;
// bytes already committed to TCP cannot be withdrawn by later SSH revocation.
func (b *bridgeServer) remoteControllerSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	var input remoteControllerSessionViewRequest
	if !controllerHandle(id) || r.URL.RawQuery != "" || r.Header.Get("Last-Event-ID") != "" || decodeJSONBody(w, r, 200<<10, &input) != nil || !controllerSessionPath(input.SessionPath) {
		writeProtocolError(w, 400, "invalid_request", "select one listed remote session without replay parameters")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	select {
	case connection.eventSlots <- struct{}{}:
		defer func() { <-connection.eventSlots }()
	default:
		writeProtocolError(w, 429, "remote_controller_stream_busy", "close an unused remote subscription and reopen the session")
		return
	}
	operation, cancel := context.WithCancel(r.Context())
	defer cancel()
	stream, err := connection.client.SessionEvents(operation, input.SessionPath)
	if b.remoteSessions.getController(id) != connection || errors.Is(err, controller.ErrClosed) {
		if stream != nil {
			stream.Close()
		}
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if errors.Is(err, controller.ErrSessionNotListed) {
		writeProtocolError(w, 404, "not_found", "remote session is no longer listed; refresh remote sessions")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not open remote events; reopen and reconcile the remote session")
		return
	}
	defer stream.Close()
	_, ok := w.(http.Flusher)
	if !ok {
		writeProtocolError(w, 500, "internal", "streaming is unavailable")
		return
	}
	response := http.NewResponseController(w)
	write := func(data []byte) bool {
		if r.Context().Err() != nil || b.remoteSessions.getController(id) != connection {
			return false
		}
		// A suspended native reader must not retain a goroutine or an owner slot
		// forever. ResponseRecorder has no deadlines; actual HTTP does.
		if err := response.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		if _, err := w.Write(data); err != nil {
			return false
		}
		return response.Flush() == nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if !write([]byte(": connected\n\n")) {
		return
	}
	type result struct {
		frame eventwire.Event
		err   error
	}
	frames := make(chan result) // At most one reader-held frame; no replay queue.
	go func() {
		for {
			frame, err := stream.Next()
			select {
			case frames <- result{frame, err}:
			case <-operation.Done():
				return
			case <-connection.owner.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	keepalive := time.NewTicker(10 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-connection.owner.Done():
			return
		case <-keepalive.C:
			if !write([]byte(": ping\n\n")) {
				return
			}
		case next := <-frames:
			if next.err != nil {
				return
			} // EOF/error requires native snapshot reconciliation.
			if next.frame.SessionPath != input.SessionPath {
				return
			}
			data, err := json.Marshal(remoteControllerSessionEvent{desktopbridge.ProtocolVersion, connection.view, input.SessionPath, next.frame})
			if err != nil || len(data) > 9<<20 || !write([]byte(fmt.Sprintf("data: %s\n\n", data))) {
				return
			}
		}
	}
}
