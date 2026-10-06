package main

import (
	"fmt"
	"net"
	"net/http"
	"reasonix/internal/remote"
	"reasonix/internal/remote/forward"
	"strings"
)

// Preview follows Wails' session-scoped local forwards. The bind address is
// chosen here; renderer input cannot expose a listener to the LAN or add -R.
type remoteForwardRequest struct {
	Name       string `json:"name"`
	Action     string `json:"action"`
	ID         string `json:"id,omitempty"`
	LocalPort  int    `json:"localPort,omitempty"`
	RemoteHost string `json:"remoteHost,omitempty"`
	RemotePort int    `json:"remotePort,omitempty"`
}
type remoteForwardView struct {
	ID            string `json:"id"`
	LocalAddress  string `json:"localAddress"`
	RemoteAddress string `json:"remoteAddress"`
	Active        bool   `json:"active"`
	Failed        bool   `json:"failed"`
}
type remoteForwardsView struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Forwards        []remoteForwardView `json:"forwards"`
}

func (b *bridgeServer) remoteForwards(w http.ResponseWriter, r *http.Request) {
	var input remoteForwardRequest
	if err := decodeJSONBody(w, r, 4096, &input); err != nil {
		writeProtocolError(w, 400, "invalid_request", "invalid forward request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.ID = strings.TrimSpace(input.ID)
	input.RemoteHost = strings.TrimSpace(input.RemoteHost)
	if !previewProviderName.MatchString(input.Name) || (input.Action != "list" && input.Action != "add" && input.Action != "remove") {
		writeProtocolError(w, 400, "invalid_request", "invalid forward request")
		return
	}
	if input.Action != "list" && (!previewProviderName.MatchString(input.ID) || len(input.ID) > 64) {
		writeProtocolError(w, 400, "invalid_request", "forward ID must use letters, digits, dots, underscores or dashes")
		return
	}
	if input.Action == "add" && (input.LocalPort < 1 || input.LocalPort > 65535 || input.RemotePort < 1 || input.RemotePort > 65535 || input.RemoteHost == "" || len(input.RemoteHost) > 253 || strings.ContainsAny(input.RemoteHost, " /\\\t\r\n\x00")) {
		writeProtocolError(w, 400, "invalid_request", "forward requires a host and ports between 1 and 65535")
		return
	}
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil || client.Status().Status != remote.StatusConnected {
		writeProtocolError(w, 409, "conflict", "remote host is not connected")
		return
	}
	id := "user:" + input.ID
	switch input.Action {
	case "add":
		_, err := client.Forwards().Add(forward.Spec{Name: id, Direction: forward.Local, BindAddr: net.JoinHostPort("127.0.0.1", fmt.Sprint(input.LocalPort)), TargetAddr: net.JoinHostPort(input.RemoteHost, fmt.Sprint(input.RemotePort))})
		if err != nil {
			writeProtocolError(w, 409, "forward_failed", "could not add forward; check its ID and whether the local port is already in use")
			return
		}
		current, _ := b.remoteSessions.get(input.Name)
		if current != client || r.Context().Err() != nil {
			_ = client.Forwards().Remove(id)
			writeProtocolError(w, 409, "conflict", "remote connection changed while adding forward")
			return
		}
	case "remove":
		if err := client.Forwards().Remove(id); err != nil {
			writeProtocolError(w, 409, "forward_failed", "forward no longer exists; refresh the list")
			return
		}
	}
	view := remoteForwardsView{ProtocolVersion: 1, Forwards: []remoteForwardView{}}
	for _, entry := range client.Forwards().List() {
		if !strings.HasPrefix(entry.Spec.Name, "user:") {
			continue
		}
		view.Forwards = append(view.Forwards, remoteForwardView{ID: strings.TrimPrefix(entry.Spec.Name, "user:"), LocalAddress: entry.BoundAddr, RemoteAddress: entry.Spec.TargetAddr, Active: entry.Up, Failed: entry.LastErr != nil})
	}
	writeJSON(w, http.StatusOK, view)
}
