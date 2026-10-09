package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote"
	"reasonix/internal/remote/bootstrap"
	"reasonix/internal/remote/controller"
)

// The catalogue/projection stays read-only. Separate scoped Stop/user-message
// routes do not grant approval/takeover authority or a local RuntimeManager.
// All endpoints/tokens stay behind the bridge.
type remoteControllerRequest struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

type remoteControllerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	ReadOnly  bool   `json:"readOnly"`
}

type remoteControllerResponse struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Controller      remoteControllerView `json:"controller"`
}

type remoteControllerSessionsResponse struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Controller      remoteControllerView `json:"controller"`
	Sessions        []controller.Session `json:"sessions"`
}

type remoteControllerCloseResponse struct {
	ProtocolVersion int  `json:"protocolVersion"`
	Closed          bool `json:"closed"`
}

type remoteControllerScope struct{ name, workspace string }
type remoteControllerAttempt struct {
	scope     remoteControllerScope
	ssh       *remote.Client
	transport *ssh.Client
	owner     context.Context
	cancel    context.CancelFunc
}
type remoteControllerConnection struct {
	*remoteControllerAttempt
	view            remoteControllerView
	client          *controller.Client
	eventSlots      chan struct{}
	projectionMu    sync.Mutex
	projectionPages map[string]remoteProjectionPage
}

// The factory seam is Go-only for owned SSH fixtures. No renderer-supplied
// base URL, token, command, local model config or arbitrary session path exists.
type remoteControllerFactory func(context.Context, context.Context, *remote.Client, remoteServeRequest) (*controller.Client, string, error)

const maxRemoteControllers = 16

func (m *previewRemoteSessions) revokePendingControllersLocked(name string) {
	for scope, ticket := range m.controllerAttempts {
		if scope.name == name {
			ticket.cancel()
			delete(m.controllerAttempts, scope)
		}
	}
}

func (m *previewRemoteSessions) controllerCurrentLocked(ticket *remoteControllerAttempt) bool {
	if ticket == nil || m.closed || ticket.owner.Err() != nil || m.clients[ticket.scope.name] != ticket.ssh || ticket.ssh.Status().Status != remote.StatusConnected {
		return false
	}
	transport, err := ticket.ssh.SSH()
	return err == nil && transport == ticket.transport
}

// Called only with m.mu held. Close cancels HTTP work; it never waits on SSH.
func (m *previewRemoteSessions) revokeControllersLocked(name, workspace string) {
	match := func(ticket *remoteControllerAttempt) bool {
		return name == "" || ticket.scope.name == name && (workspace == "" || ticket.scope.workspace == workspace)
	}
	for scope, ticket := range m.controllerAttempts {
		if match(ticket) {
			ticket.cancel()
			delete(m.controllerAttempts, scope)
		}
	}
	for id, connection := range m.controllers {
		if match(connection.remoteControllerAttempt) || name != "" && connection.scope.name == name && connection.view.Workspace == workspace {
			connection.cancel()
			connection.client.Close()
			delete(m.controllers, id)
		}
	}
}

func (m *previewRemoteSessions) beginController(scope remoteControllerScope, client *remote.Client) (*remoteControllerAttempt, *remoteControllerConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.clients[scope.name] != client || client.Status().Status != remote.StatusConnected {
		return nil, nil, controller.ErrClosed
	}
	transport, err := client.SSH()
	if err != nil {
		return nil, nil, controller.ErrClosed
	}
	for id, connection := range m.controllers {
		if !m.controllerCurrentLocked(connection.remoteControllerAttempt) || connection.client.Closed() {
			connection.cancel()
			connection.client.Close()
			delete(m.controllers, id)
			continue
		}
		if connection.scope == scope && connection.ssh == client && connection.transport == transport {
			return nil, connection, nil
		}
	}
	for pendingScope, ticket := range m.controllerAttempts {
		if ticket.owner.Err() != nil || pendingScope == scope {
			ticket.cancel()
			delete(m.controllerAttempts, pendingScope)
		}
	}
	if len(m.controllers)+len(m.controllerAttempts) >= maxRemoteControllers {
		return nil, nil, errors.New("controller limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ticket := &remoteControllerAttempt{scope: scope, ssh: client, transport: transport, owner: ctx, cancel: cancel}
	m.controllerAttempts[scope] = ticket
	return ticket, nil, nil
}

func (m *previewRemoteSessions) finishController(ticket *remoteControllerAttempt, published bool) {
	m.mu.Lock()
	if m.controllerAttempts[ticket.scope] == ticket {
		delete(m.controllerAttempts, ticket.scope)
	}
	m.mu.Unlock()
	if !published {
		ticket.cancel()
	}
}

func (m *previewRemoteSessions) publishController(ticket *remoteControllerAttempt, candidate *controller.Client, workspace string) (*remoteControllerConnection, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.controllerAttempts[ticket.scope] != ticket || !m.controllerCurrentLocked(ticket) || candidate.Closed() {
		return nil, false
	}
	id, err := randomID()
	if err != nil {
		return nil, false
	}
	connection := &remoteControllerConnection{remoteControllerAttempt: ticket, client: candidate, eventSlots: make(chan struct{}, 2), view: remoteControllerView{ID: id, Name: ticket.scope.name, Workspace: workspace, ReadOnly: true}}
	m.controllers[connection.view.ID] = connection
	delete(m.controllerAttempts, ticket.scope)
	return connection, true
}

func (m *previewRemoteSessions) getController(id string) *remoteControllerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	connection := m.controllers[id]
	if connection != nil && (!m.controllerCurrentLocked(connection.remoteControllerAttempt) || connection.client.Closed()) {
		connection.cancel()
		connection.client.Close()
		delete(m.controllers, id)
		return nil
	}
	return connection
}

func (m *previewRemoteSessions) closeController(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if connection := m.controllers[id]; connection != nil {
		connection.cancel()
		connection.client.Close()
		delete(m.controllers, id)
	}
}

func (b *bridgeServer) attachRemoteController(w http.ResponseWriter, r *http.Request) {
	var input remoteControllerRequest
	if decodeJSONBody(w, r, 12<<10, &input) != nil {
		writeProtocolError(w, 400, "invalid_request", "invalid remote controller request")
		return
	}
	input.Name, input.Workspace = strings.TrimSpace(input.Name), strings.TrimSpace(input.Workspace)
	if !previewProviderName.MatchString(input.Name) || !controllerDisplayPath(input.Workspace) {
		writeProtocolError(w, 400, "invalid_request", "remote controller requires a saved host and workspace")
		return
	}
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil {
		writeProtocolError(w, 409, "conflict", "remote host is not connected; reconnect the saved SSH host")
		return
	}
	ticket, existing, err := b.remoteSessions.beginController(remoteControllerScope{input.Name, input.Workspace}, client)
	if err != nil {
		writeProtocolError(w, 409, "conflict", "remote controller is unavailable; reconnect the host or close unused remote controllers")
		return
	}
	if existing != nil {
		writeJSON(w, 200, remoteControllerResponse{desktopbridge.ProtocolVersion, existing.view})
		return
	}
	published := false
	defer func() { b.remoteSessions.finishController(ticket, published) }()
	// No Client call inside a status callback: only revoke this immutable owner.
	unsubscribe := client.Subscribe(func(event remote.StatusEvent) {
		if event.Status != remote.StatusConnected {
			ticket.cancel()
		}
	})
	context.AfterFunc(ticket.owner, unsubscribe)
	ctx, cancel := context.WithTimeout(r.Context(), remoteServeTimeout)
	defer cancel()
	stop := context.AfterFunc(ticket.owner, cancel)
	defer stop()
	factory := b.remoteControllerFactory
	if factory == nil {
		factory = b.makeRemoteController
	}
	candidate, workspace, err := factory(ticket.owner, ctx, client, remoteServeRequest{Name: input.Name, Workspace: input.Workspace})
	if err != nil || candidate == nil {
		if candidate != nil {
			candidate.Close()
		}
		if ticket.owner.Err() != nil || r.Context().Err() != nil {
			writeProtocolError(w, 409, "conflict", "remote connection changed; reopen the remote workspace")
			return
		}
		writeProtocolError(w, 502, "remote_controller_failed", "could not authenticate remote controller; verify remote Serve and reconnect the saved SSH host")
		return
	}
	if ctx.Err() != nil || !controllerDisplayPath(workspace) {
		candidate.Close()
		writeProtocolError(w, 409, "conflict", "remote connection changed; reopen the remote workspace")
		return
	}
	connection, ok := b.remoteSessions.publishController(ticket, candidate, workspace)
	if !ok {
		candidate.Close()
		writeProtocolError(w, 409, "conflict", "remote connection changed; reopen the remote workspace")
		return
	}
	published = true
	writeJSON(w, 200, remoteControllerResponse{desktopbridge.ProtocolVersion, connection.view})
}

func controllerDisplayPath(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func controllerHandle(value string) bool {
	if len(value) != 22 {
		return false
	}
	bytes, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(bytes) == 16 && base64.RawURLEncoding.EncodeToString(bytes) == value
}

func (b *bridgeServer) remoteControllerSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	if !controllerHandle(id) {
		writeProtocolError(w, 400, "invalid_request", "invalid controller handle")
		return
	}
	connection := b.remoteSessions.getController(id)
	if connection == nil {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	entries, err := connection.client.Sessions(r.Context())
	if b.remoteSessions.getController(id) != connection || errors.Is(err, controller.ErrClosed) {
		writeProtocolError(w, 409, "conflict", "remote controller changed; reopen the remote workspace")
		return
	}
	if err != nil {
		writeProtocolError(w, 502, "remote_controller_failed", "could not read remote sessions; verify remote Serve and reopen the remote workspace")
		return
	}
	writeJSON(w, 200, remoteControllerSessionsResponse{desktopbridge.ProtocolVersion, connection.view, entries})
}

func (b *bridgeServer) closeRemoteController(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("controllerID")
	if !controllerHandle(id) {
		writeProtocolError(w, 400, "invalid_request", "invalid controller handle")
		return
	}
	b.remoteSessions.closeController(id)
	writeJSON(w, 200, remoteControllerCloseResponse{desktopbridge.ProtocolVersion, true})
}

func (b *bridgeServer) makeRemoteController(owner, operation context.Context, client *remote.Client, input remoteServeRequest) (*controller.Client, string, error) {
	unlock, err := b.remoteServeMu.lockContext(operation, input.Name)
	if err != nil {
		return nil, "", controller.ErrClosed
	}
	defer unlock()
	if operation.Err() != nil || !b.remoteServeStillConnected(input.Name, client) {
		return nil, "", controller.ErrClosed
	}
	host, err := remoteServeHost(input.Name)
	if err != nil || host.CredentialProxyEnabled() {
		return nil, "", controller.ErrUnavailable
	}
	result, err := bootstrap.EnsureServe(operation, client, bootstrap.Options{Workspace: input.Workspace, Install: host.ServeInstallMode(), LocalGOOS: runtime.GOOS, LocalGOARCH: runtime.GOARCH, MinVersion: bootstrap.MinServeVersion})
	if err != nil {
		return nil, "", controller.ErrUnavailable
	}
	bound, err := b.ensureRemoteServeForward(operation, input, client, result.State.Addr)
	if err != nil {
		if !result.Reused {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = bootstrap.Stop(cleanup, client, input.Workspace)
			cancel()
		}
		return nil, "", controller.ErrUnavailable
	}
	candidate, err := controller.Connect(owner, operation, "http://"+bound, result.Token)
	return candidate, result.State.Workspace, err
}
