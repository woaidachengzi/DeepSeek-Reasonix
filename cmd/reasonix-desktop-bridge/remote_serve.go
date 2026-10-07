package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote"
	"reasonix/internal/remote/bootstrap"
	"reasonix/internal/remote/forward"
	"reasonix/internal/store"
)

// remoteServeRequest is deliberately small: a renderer can choose only a
// configured host and a bounded workspace path. It cannot choose a bind
// address, token, command, or remote port.
type remoteServeRequest struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	TailLines int    `json:"tailLines,omitempty"`
}

type remoteServeView struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Name            string `json:"name"`
	Workspace       string `json:"workspace"`
	State           string `json:"state"`
	LocalURL        string `json:"localUrl,omitempty"`
	Message         string `json:"message,omitempty"`
	Logs            string `json:"logs,omitempty"`
}

// remoteControllerLaunch is consumed only by the native Tauri host. The URL
// carries the Serve bootstrap token and must never reach renderer JavaScript.
type remoteControllerLaunch struct {
	ProtocolVersion int    `json:"protocolVersion"`
	URL             string `json:"url"`
}

const (
	remoteServeTimeout = 45 * time.Second
	remoteServeLogMax  = 500
)

// serveForwardName matches the workspace-derived name used by the Wails
// desktop. It keeps each workspace isolated while retaining only loopback
// listeners in the local bridge process.
func serveForwardName(workspace string) string {
	return "serve-" + store.RemoteWorkspaceSlug(workspace)
}

func newRemoteServeMutexes() *remoteServeMutexes {
	return &remoteServeMutexes{hosts: map[string]*sync.Mutex{}}
}

type remoteServeMutexes struct {
	mu    sync.Mutex
	hosts map[string]*sync.Mutex
}

func (m *remoteServeMutexes) lock(name string) func() {
	m.mu.Lock()
	if m.hosts == nil {
		m.hosts = map[string]*sync.Mutex{}
	}
	lock := m.hosts[name]
	if lock == nil {
		lock = &sync.Mutex{}
		m.hosts[name] = lock
	}
	m.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (b *bridgeServer) remoteServeStatus(w http.ResponseWriter, r *http.Request) {
	input, client, ok := b.remoteServeConnectedClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	state, alive, err := bootstrap.Status(ctx, client, input.Workspace)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "could not read remote Serve status")
		return
	}
	view := remoteServeView{ProtocolVersion: desktopbridge.ProtocolVersion, Name: input.Name, Workspace: input.Workspace, State: "stopped"}
	if alive {
		view.State = "running"
		if localURL, found := remoteServeForwardURL(client, input.Workspace, state.Addr); found {
			view.State, view.LocalURL = "ready", localURL
		} else {
			view.Message = "Serve is running, but this desktop session has no local tunnel. Select Start to create one."
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) startRemoteServe(w http.ResponseWriter, r *http.Request) {
	input, client, ok := b.remoteServeConnectedClient(w, r)
	if !ok {
		return
	}
	unlock := b.remoteServeMu.lock(input.Name)
	defer unlock()
	if !b.remoteServeStillConnected(input.Name, client) {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote connection changed while starting Serve")
		return
	}
	host, err := remoteServeHost(input.Name)
	if err != nil {
		writeProtocolError(w, http.StatusNotFound, "not_found", "saved SSH host not found")
		return
	}
	// The Preview sidecar deliberately does not implement the desktop-held
	// credential proxy yet. Refuse before touching the remote so a host never
	// starts with a configuration that cannot authenticate model calls.
	if host.CredentialProxyEnabled() {
		writeProtocolError(w, http.StatusConflict, "remote_serve_unsupported", "local-proxy credentials are not available in Tauri Preview yet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	result, err := bootstrap.EnsureServe(ctx, client, bootstrap.Options{
		Workspace:   input.Workspace,
		Install:     host.ServeInstallMode(),
		LocalGOOS:   runtime.GOOS,
		LocalGOARCH: runtime.GOARCH,
		MinVersion:  bootstrap.MinServeVersion,
	})
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "could not start remote Serve; verify its install policy and remote Reasonix or npm availability")
		return
	}
	if !b.remoteServeStillConnected(input.Name, client) || r.Context().Err() != nil {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote connection changed while starting Serve")
		return
	}
	bound, err := b.ensureRemoteServeForward(r.Context(), input, client, result.State.Addr)
	if err != nil {
		if !result.Reused {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = bootstrap.Stop(cleanupCtx, client, input.Workspace)
			cleanupCancel()
		}
		writeProtocolError(w, http.StatusConflict, "remote_serve_failed", "could not open a loopback tunnel to remote Serve")
		return
	}
	writeJSON(w, http.StatusOK, remoteServeView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Name:            input.Name,
		Workspace:       input.Workspace,
		State:           "ready",
		LocalURL:        fmt.Sprintf("http://%s/", bound),
	})
}

func (b *bridgeServer) stopRemoteServe(w http.ResponseWriter, r *http.Request) {
	input, client, ok := b.remoteServeConnectedClient(w, r)
	if !ok {
		return
	}
	unlock := b.remoteServeMu.lock(input.Name)
	defer unlock()
	if !b.remoteServeStillConnected(input.Name, client) || r.Context().Err() != nil {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote connection changed while stopping Serve")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	if err := bootstrap.Stop(ctx, client, input.Workspace); err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "could not stop remote Serve")
		return
	}
	_ = client.Forwards().Remove(serveForwardName(input.Workspace))
	writeJSON(w, http.StatusOK, remoteServeView{ProtocolVersion: desktopbridge.ProtocolVersion, Name: input.Name, Workspace: input.Workspace, State: "stopped"})
}

// openRemoteController follows the Wails remote-window entry: ensure a Serve,
// publish a loopback tunnel, then return a fragment-token URL exclusively to
// the native host. Serve bootstraps an HttpOnly cookie and clears the fragment.
func (b *bridgeServer) openRemoteController(w http.ResponseWriter, r *http.Request) {
	input, client, ok := b.remoteServeConnectedClient(w, r)
	if !ok {
		return
	}
	unlock := b.remoteServeMu.lock(input.Name)
	defer unlock()
	if !b.remoteServeStillConnected(input.Name, client) {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote connection changed while opening controller")
		return
	}
	host, err := remoteServeHost(input.Name)
	if err != nil {
		writeProtocolError(w, http.StatusNotFound, "not_found", "saved SSH host not found")
		return
	}
	if host.CredentialProxyEnabled() {
		writeProtocolError(w, http.StatusConflict, "remote_serve_unsupported", "local-proxy credentials are not available in Tauri Preview yet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	result, err := bootstrap.EnsureServe(ctx, client, bootstrap.Options{
		Workspace:   input.Workspace,
		Install:     host.ServeInstallMode(),
		LocalGOOS:   runtime.GOOS,
		LocalGOARCH: runtime.GOARCH,
		MinVersion:  bootstrap.MinServeVersion,
	})
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "could not start remote Serve; verify its install policy and remote Reasonix or npm availability")
		return
	}
	if !b.remoteServeStillConnected(input.Name, client) || r.Context().Err() != nil {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote connection changed while opening controller")
		return
	}
	bound, err := b.ensureRemoteServeForward(r.Context(), input, client, result.State.Addr)
	if err != nil {
		if !result.Reused {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = bootstrap.Stop(cleanupCtx, client, input.Workspace)
			cleanupCancel()
		}
		writeProtocolError(w, http.StatusConflict, "remote_serve_failed", "could not open a loopback tunnel to remote Serve")
		return
	}
	controllerURL, err := remoteControllerURL(bound, result.Token)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "remote Serve did not provide a controller token")
		return
	}
	writeJSON(w, http.StatusOK, remoteControllerLaunch{ProtocolVersion: desktopbridge.ProtocolVersion, URL: controllerURL})
}

func (b *bridgeServer) remoteServeLogs(w http.ResponseWriter, r *http.Request) {
	input, client, ok := b.remoteServeConnectedClient(w, r)
	if !ok {
		return
	}
	lines := input.TailLines
	if lines == 0 {
		lines = 100
	}
	if lines < 1 || lines > remoteServeLogMax {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "tail lines must be between 1 and 500")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	var logs strings.Builder
	if err := bootstrap.LogsBounded(ctx, client, input.Workspace, lines, 256<<10, &logs); err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_serve_failed", "could not read remote Serve logs")
		return
	}
	writeJSON(w, http.StatusOK, remoteServeView{ProtocolVersion: desktopbridge.ProtocolVersion, Name: input.Name, Workspace: input.Workspace, State: "logs", Logs: logs.String()})
}

func (b *bridgeServer) remoteServeConnectedClient(w http.ResponseWriter, r *http.Request) (remoteServeRequest, *remote.Client, bool) {
	var input remoteServeRequest
	if err := decodeJSONBody(w, r, 12<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote Serve request")
		return remoteServeRequest{}, nil, false
	}
	input.Name, input.Workspace = strings.TrimSpace(input.Name), strings.TrimSpace(input.Workspace)
	if !previewProviderName.MatchString(input.Name) || input.Workspace == "" || len(input.Workspace) > 4096 || strings.ContainsRune(input.Workspace, '\x00') {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "remote Serve requires a configured host and workspace")
		return remoteServeRequest{}, nil, false
	}
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil || client.Status().Status != remote.StatusConnected {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote host is not connected")
		return remoteServeRequest{}, nil, false
	}
	return input, client, true
}

func (b *bridgeServer) remoteServeStillConnected(name string, client *remote.Client) bool {
	current, _ := b.remoteSessions.get(name)
	return current == client && client.Status().Status == remote.StatusConnected
}

func remoteServeHost(name string) (configpkg.RemoteHostEntry, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return configpkg.RemoteHostEntry{}, err
	}
	host, ok := cfg.RemoteHost(name)
	if !ok {
		return configpkg.RemoteHostEntry{}, fmt.Errorf("host %q is not configured", name)
	}
	return host, nil
}

func remoteServeForwardURL(client *remote.Client, workspace, addr string) (string, bool) {
	for _, entry := range client.Forwards().List() {
		if entry.Spec.Name == serveForwardName(workspace) && entry.Spec.TargetAddr == addr && entry.Up && entry.BoundAddr != "" {
			return fmt.Sprintf("http://%s/", entry.BoundAddr), true
		}
	}
	return "", false
}

func (b *bridgeServer) ensureRemoteServeForward(ctx context.Context, input remoteServeRequest, client *remote.Client, addr string) (string, error) {
	if !b.remoteServeStillConnected(input.Name, client) || ctx.Err() != nil {
		return "", fmt.Errorf("remote connection changed")
	}
	// Keep an existing browser/SSE connection alive when Serve is reused.
	for _, entry := range client.Forwards().List() {
		if entry.Spec.Name == serveForwardName(input.Workspace) && entry.Spec.TargetAddr == addr && entry.Up && entry.BoundAddr != "" {
			return entry.BoundAddr, nil
		}
	}
	bound, err := client.Forwards().Replace(forward.Spec{
		Name: serveForwardName(input.Workspace), Direction: forward.Local, BindAddr: "127.0.0.1:0", TargetAddr: addr,
	})
	if err != nil {
		return "", err
	}
	if !b.remoteServeStillConnected(input.Name, client) || ctx.Err() != nil {
		_ = client.Forwards().Remove(serveForwardName(input.Workspace))
		return "", fmt.Errorf("remote connection changed")
	}
	return bound, nil
}

func remoteControllerURL(bound, token string) (string, error) {
	if len(token) != 64 || strings.IndexFunc(token, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F')
	}) >= 0 {
		return "", fmt.Errorf("missing token")
	}
	host, port, err := net.SplitHostPort(bound)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || port == "" {
		return "", fmt.Errorf("invalid loopback endpoint")
	}
	endpoint := &url.URL{Scheme: "http", Host: bound, Path: "/"}
	if endpoint.Hostname() == "" || endpoint.Port() == "" {
		return "", fmt.Errorf("invalid loopback endpoint")
	}
	query := url.Values{}
	query.Set("token", token)
	endpoint.Fragment = query.Encode()
	return endpoint.String(), nil
}
