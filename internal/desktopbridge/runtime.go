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
	ErrInvalidInput     = errors.New("desktop bridge input is required")
	ErrSessionNotFound  = errors.New("desktop bridge session was not found")
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

// HistoryMessage is the deliberately small, display-safe transcript projection
// the bridge may return to a desktop host. It must never contain provider
// reasoning, tool arguments/results, system prompts, image data, or other
// persistence metadata.
type HistoryMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// HistoryView is the latest bounded page of display-safe messages. StartIndex
// and TotalMessages refer to the projected (not raw provider) transcript.
type HistoryView struct {
	Session       SessionView      `json:"session"`
	Messages      []HistoryMessage `json:"messages"`
	StartIndex    int              `json:"startIndex"`
	TotalMessages int              `json:"totalMessages"`
}

const maxHistoryMessages = 200

// Runtime is the minimal core lifecycle surface needed by the first bridge
// vertical slices. Implementations must make Shutdown durable before releasing
// their core resources.
type Runtime interface {
	SessionPath() string
	State() string
	History() []HistoryMessage
	Submit(input string)
	Cancel()
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
	if !validSessionID(request.SessionID) {
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

// Switch makes an explicit, durable handoff between two bridge sessions.
// The first bridge version owns only one controller, so allowing a second
// open to silently replace it would lose in-flight state. Only an idle
// runtime may be switched: it is snapshotted and closed before the next core
// controller is constructed. A running or paused turn remains selected until
// the user cancels or completes it.
func (m *RuntimeManager) Switch(ctx context.Context, request OpenRequest) (SessionView, error) {
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	if !validSessionID(request.SessionID) {
		return SessionView{}, ErrInvalidSessionID
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil {
		m.mu.Unlock()
		return m.Open(ctx, request)
	}
	if m.opening {
		m.mu.Unlock()
		return SessionView{}, ErrOpenInProgress
	}
	if m.view.ID == request.SessionID && m.view.WorkspaceRoot == request.WorkspaceRoot {
		view := m.view
		m.mu.Unlock()
		return view, nil
	}
	if state := m.runtime.State(); state != "idle" {
		active := m.view.ID
		m.mu.Unlock()
		return SessionView{}, fmt.Errorf("%w: active session %q is %s", ErrSessionConflict, active, state)
	}
	previous := m.runtime
	factory := m.factory
	if factory == nil {
		m.mu.Unlock()
		return SessionView{}, errors.New("desktop bridge runtime factory is not configured")
	}
	m.runtime = nil
	m.view = SessionView{}
	m.opening = true
	m.mu.Unlock()

	if err := previous.Shutdown(); err != nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, fmt.Errorf("close active desktop bridge session: %w", err)
	}
	return m.openWithFactory(ctx, factory, request)
}

// validSessionID keeps the bridge's public ID safe for hosts that derive a
// deterministic session filename. The Rust host already applies this rule;
// enforcing it here keeps direct loopback callers from widening that boundary.
func validSessionID(sessionID string) bool {
	if sessionID == "" || len(sessionID) > 128 {
		return false
	}
	for _, byte := range []byte(sessionID) {
		if !(byte >= 'a' && byte <= 'z') && !(byte >= 'A' && byte <= 'Z') && !(byte >= '0' && byte <= '9') && byte != '-' && byte != '_' {
			return false
		}
	}
	return true
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

// History returns the newest bounded page of the bridge-owned transcript. The
// Runtime owns projection from its richer local model, so this package remains
// independent of controller and provider implementation details.
func (m *RuntimeManager) History(sessionID string) (HistoryView, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return HistoryView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return HistoryView{}, ErrSessionNotFound
	}
	messages := m.runtime.History()
	total := len(messages)
	start := 0
	if total > maxHistoryMessages {
		start = total - maxHistoryMessages
	}
	page := append([]HistoryMessage(nil), messages[start:]...)
	// The protocol models messages as a JSON array. Keep the empty case as []
	// rather than nil so Go's JSON encoder does not emit `null` (Rust expects a
	// sequence when decoding BridgeHistoryResponse).
	if page == nil {
		page = []HistoryMessage{}
	}
	view := m.view
	view.State = m.runtime.State()
	return HistoryView{
		Session:       view,
		Messages:      page,
		StartIndex:    start,
		TotalMessages: total,
	}, nil
}

// Submit starts a turn on the bridge-owned runtime. It intentionally returns
// after admission rather than waiting for Agent work; progress is delivered by
// the event transport added on top of this lifecycle layer.
func (m *RuntimeManager) Submit(sessionID, input string) (SessionView, error) {
	if strings.TrimSpace(input) == "" {
		return SessionView{}, ErrInvalidInput
	}
	return m.withRuntime(sessionID, func(runtime Runtime) {
		runtime.Submit(input)
	})
}

// Cancel asks the bridge-owned runtime to stop foreground work. It is safe to
// call while idle; the core decides whether there is work that can be stopped.
func (m *RuntimeManager) Cancel(sessionID string) (SessionView, error) {
	return m.withRuntime(sessionID, func(runtime Runtime) {
		runtime.Cancel()
	})
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

func (m *RuntimeManager) openWithFactory(ctx context.Context, factory RuntimeFactory, request OpenRequest) (SessionView, error) {
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

func (m *RuntimeManager) withRuntime(sessionID string, action func(Runtime)) (SessionView, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return SessionView{}, ErrSessionNotFound
	}
	action(m.runtime)
	view := m.view
	view.State = m.runtime.State()
	return view, nil
}
