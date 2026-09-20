// Package desktopbridge contains the host-neutral lifecycle primitives used by
// the private Desktop Bridge protocol. It deliberately owns no UI state.
package desktopbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrClosed           = errors.New("desktop bridge runtime manager is closed")
	ErrOpenInProgress   = errors.New("desktop bridge session open is already in progress")
	ErrSessionConflict  = errors.New("desktop bridge already owns a different session")
	ErrInvalidSessionID = errors.New("desktop bridge session id is required")
)

// OpenRequest identifies the one local session the first bridge release owns.
// The caller is responsible for resolving a user-facing ID to a stable core
// session path before constructing the real Runtime.
type OpenRequest struct {
	SessionID     string
	WorkspaceRoot string
}

// SessionView is transport-safe runtime metadata. It intentionally excludes
// history and provider details; those gain their own versioned contracts.
type SessionView struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	State         string `json:"state"`
}

// Runtime is the minimal core lifecycle surface needed before command and
// transcript APIs are added. Implementations must make Shutdown durable before
// releasing their core resources.
type Runtime interface {
	SessionPath() string
	State() string
	Shutdown() error
}

// RuntimeFactory builds a core runtime only after an authenticated open request.
// Bridge startup and health checks never invoke it.
type RuntimeFactory interface {
	Open(context.Context, OpenRequest) (Runtime, error)
}

// RuntimeFactoryFunc adapts a function to RuntimeFactory.
type RuntimeFactoryFunc func(context.Context, OpenRequest) (Runtime, error)

func (f RuntimeFactoryFunc) Open(ctx context.Context, request OpenRequest) (Runtime, error) {
	return f(ctx, request)
}

// RuntimeManager serializes ownership of the first bridge session. It does not
// allow a second session to replace a live controller implicitly; the future
// multi-tab manager must make that transfer explicit and testable.
type RuntimeManager struct {
	factory RuntimeFactory

	mu      sync.Mutex
	runtime Runtime
	view    SessionView
	opening bool
	closed  bool
}

func NewRuntimeManager(factory RuntimeFactory) *RuntimeManager {
	return &RuntimeManager{factory: factory}
}

func (m *RuntimeManager) Open(ctx context.Context, request OpenRequest) (SessionView, error) {
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	if request.SessionID == "" {
		return SessionView{}, ErrInvalidSessionID
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return SessionView{}, ErrClosed
	}
	if m.runtime != nil {
		view := m.view
		m.mu.Unlock()
		if view.ID == request.SessionID && view.WorkspaceRoot == request.WorkspaceRoot {
			return view, nil
		}
		return SessionView{}, fmt.Errorf("%w: active=%q requested=%q", ErrSessionConflict, view.ID, request.SessionID)
	}
	if m.opening {
		m.mu.Unlock()
		return SessionView{}, ErrOpenInProgress
	}
	if m.factory == nil {
		m.mu.Unlock()
		return SessionView{}, errors.New("desktop bridge runtime factory is not configured")
	}
	m.opening = true
	factory := m.factory
	m.mu.Unlock()

	runtime, err := factory.Open(ctx, request)
	if err != nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, err
	}
	if runtime == nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, errors.New("desktop bridge runtime factory returned nil runtime")
	}
	path := strings.TrimSpace(runtime.SessionPath())
	if path == "" {
		_ = runtime.Shutdown()
		m.finishOpen(nil, SessionView{})
		return SessionView{}, errors.New("desktop bridge runtime has no session path")
	}
	view := SessionView{
		ID:            request.SessionID,
		Path:          path,
		WorkspaceRoot: request.WorkspaceRoot,
		State:         runtime.State(),
	}
	if m.finishOpen(runtime, view) {
		return view, nil
	}
	_ = runtime.Shutdown()
	return SessionView{}, ErrClosed
}

// Snapshot returns current bridge-owned metadata without writing session data.
func (m *RuntimeManager) Snapshot() (SessionView, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtime == nil {
		return SessionView{}, false
	}
	view := m.view
	view.State = m.runtime.State()
	return view, true
}

// Shutdown makes the owned core durable and then releases it. It is idempotent.
func (m *RuntimeManager) Shutdown() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	runtime := m.runtime
	m.runtime = nil
	m.view = SessionView{}
	m.mu.Unlock()
	if runtime == nil {
		return nil
	}
	return runtime.Shutdown()
}

func (m *RuntimeManager) finishOpen(runtime Runtime, view SessionView) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opening = false
	if m.closed {
		return false
	}
	if runtime != nil {
		m.runtime = runtime
		m.view = view
	}
	return true
}
