// Package desktopbridge contains the host-neutral lifecycle primitives used by
// the private Desktop Bridge protocol. It deliberately owns no UI state.
package desktopbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrClosed               = errors.New("desktop bridge runtime manager is closed")
	ErrOpenInProgress       = errors.New("desktop bridge session open is already in progress")
	ErrSessionConflict      = errors.New("desktop bridge already owns a different session")
	ErrInvalidSessionID     = errors.New("desktop bridge session id is required")
	ErrInvalidInput         = errors.New("desktop bridge input is required")
	ErrInvalidAttachment    = errors.New("desktop bridge attachment path is invalid")
	ErrInvalidWorkspacePath = errors.New("desktop bridge workspace path is invalid")
	ErrInvalidTitle         = errors.New("desktop bridge session title is invalid")
	ErrSessionNotFound      = errors.New("desktop bridge session was not found")
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
	Title         string `json:"title,omitempty"`
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

// AskAnswer is the transport-neutral projection of one structured question
// answer. The bridge keeps this small DTO independent from the controller's
// event package so the lifecycle manager remains host-neutral.
type AskAnswer struct {
	QuestionID string   `json:"questionId"`
	Selected   []string `json:"selected"`
}

// AttachmentView is the user-visible result of copying a file into the active
// session workspace. The source path is deliberately never returned.
type AttachmentView struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsImage bool   `json:"isImage"`
}

// WorkspaceEntry is one bounded, workspace-relative directory entry. The
// bridge never returns an absolute path, so the renderer cannot turn a list
// response into a filesystem escape.
type WorkspaceEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// WorkspaceList is one directory level. Callers request another level with
// the returned relative path; the core intentionally avoids recursive scans
// because large repositories are common desktop workspaces.
type WorkspaceList struct {
	Path      string           `json:"path"`
	Entries   []WorkspaceEntry `json:"entries"`
	Truncated bool             `json:"truncated"`
}

// WorkspaceFilePreview is a bounded, workspace-relative text preview. Binary
// or invalid-UTF-8 files never expose their bytes to the renderer; callers can
// still use Path to insert a safe @reference into the composer.
type WorkspaceFilePreview struct {
	Path      string `json:"path"`
	Body      string `json:"body,omitempty"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Error     string `json:"error,omitempty"`
}

// WorkspaceChangeView is one current workspace change. Sources identifies
// whether it comes from Git, the active session checkpoint, or both. Paths
// remain relative to the active workspace.
type WorkspaceChangeView struct {
	Path             string   `json:"path"`
	OldPath          string   `json:"oldPath,omitempty"`
	Sources          []string `json:"sources"`
	GitStatus        string   `json:"gitStatus,omitempty"`
	Turns            []int    `json:"turns,omitempty"`
	LatestPrompt     string   `json:"latestPrompt,omitempty"`
	LatestTime       int64    `json:"latestTime,omitempty"`
	CanSessionRevert bool     `json:"canSessionRevert,omitempty"`
}

type WorkspaceChanges struct {
	Files        []WorkspaceChangeView `json:"files"`
	GitAvailable bool                  `json:"gitAvailable"`
	GitErr       string                `json:"gitErr,omitempty"`
	GitBranch    string                `json:"gitBranch,omitempty"`
}

type WorkspaceChangeDetail struct {
	Diff      string `json:"diff,omitempty"`
	Source    string `json:"source,omitempty"`
	Added     int    `json:"added,omitempty"`
	Removed   int    `json:"removed,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
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
	Title() string
	State() string
	History() []HistoryMessage
	Rename(title string) error
	Delete() error
	AttachFile(path string) (AttachmentView, error)
	ListWorkspace(path string) (WorkspaceList, error)
	ReadWorkspaceFile(path string) (WorkspaceFilePreview, error)
	WorkspaceChanges() WorkspaceChanges
	WorkspaceChangeDetail(path string) (WorkspaceChangeDetail, error)
	Submit(input string)
	Cancel()
	Approve(promptID string, allow bool)
	AnswerQuestion(promptID string, answers []AskAnswer) error
	AnswerMCPInteraction(promptID, action string, content map[string]any) error
	ReplayPendingPrompts()
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
	return m.openWithFactory(ctx, factory, request)
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
	previousRequest := OpenRequest{SessionID: m.view.ID, WorkspaceRoot: m.view.WorkspaceRoot}
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
	runtime, view, err := buildRuntime(ctx, factory, request)
	if err == nil {
		if m.finishOpen(runtime, view) {
			return view, nil
		}
		_ = runtime.Shutdown()
		return SessionView{}, ErrClosed
	}

	// A failed target open must not strand the previous conversation. The old
	// controller was durably closed above, so it is safe to reopen it. A caller
	// cancellation must not cancel this recovery attempt as well.
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, ErrClosed
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	recovered, recoveredView, recoveryErr := buildRuntime(recoveryCtx, factory, previousRequest)
	if recoveryErr != nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, errors.Join(
			fmt.Errorf("open target desktop bridge session: %w", err),
			fmt.Errorf("restore previous desktop bridge session: %w", recoveryErr),
		)
	}
	if !m.finishOpen(recovered, recoveredView) {
		_ = recovered.Shutdown()
		return SessionView{}, ErrClosed
	}
	return SessionView{}, fmt.Errorf("open target desktop bridge session: %w; previous session restored", err)
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

// AttachFile copies a user-selected file into the owned session's workspace.
// Holding the manager lock prevents shutdown or session replacement from
// racing an in-progress copy.
func (m *RuntimeManager) AttachFile(sessionID, path string) (AttachmentView, error) {
	sessionID = strings.TrimSpace(sessionID)
	if strings.TrimSpace(path) == "" {
		return AttachmentView{}, ErrInvalidAttachment
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return AttachmentView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return AttachmentView{}, ErrSessionNotFound
	}
	return m.runtime.AttachFile(path)
}

// Workspace lists one directory in the owned session workspace. Keeping the
// manager lock across the call prevents a session switch or shutdown from
// racing the runtime's workspace root.
func (m *RuntimeManager) Workspace(sessionID, path string) (WorkspaceList, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceList{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceList{}, ErrSessionNotFound
	}
	return m.runtime.ListWorkspace(path)
}

// WorkspaceFilePreview reads a bounded, display-safe preview from the owned
// session workspace. Keeping the manager lock across the call prevents a
// session switch or shutdown from racing the path validation and read.
func (m *RuntimeManager) WorkspaceFilePreview(sessionID, path string) (WorkspaceFilePreview, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceFilePreview{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceFilePreview{}, ErrSessionNotFound
	}
	return m.runtime.ReadWorkspaceFile(path)
}

func (m *RuntimeManager) WorkspaceChanges(sessionID string) (WorkspaceChanges, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceChanges{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceChanges{}, ErrSessionNotFound
	}
	return m.runtime.WorkspaceChanges(), nil
}

func (m *RuntimeManager) WorkspaceChangeDetail(sessionID, path string) (WorkspaceChangeDetail, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceChangeDetail{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceChangeDetail{}, ErrSessionNotFound
	}
	return m.runtime.WorkspaceChangeDetail(path)
}

// Cancel asks the bridge-owned runtime to stop foreground work. It is safe to
// call while idle; the core decides whether there is work that can be stopped.
func (m *RuntimeManager) Cancel(sessionID string) (SessionView, error) {
	return m.withRuntime(sessionID, func(runtime Runtime) {
		runtime.Cancel()
	})
}

// Approve resolves one pending tool permission. The core treats an unknown or
// already answered ID as a no-op, which makes a retried button safe.
func (m *RuntimeManager) Approve(sessionID, promptID string, allow bool) (SessionView, error) {
	if strings.TrimSpace(promptID) == "" {
		return SessionView{}, ErrInvalidInput
	}
	return m.withRuntime(sessionID, func(runtime Runtime) {
		runtime.Approve(promptID, allow)
	})
}

// AnswerQuestion durably records an ask-tool answer before the blocked turn is
// released. The runtime owns validation against the pending prompt.
func (m *RuntimeManager) AnswerQuestion(sessionID, promptID string, answers []AskAnswer) (SessionView, error) {
	if strings.TrimSpace(promptID) == "" {
		return SessionView{}, ErrInvalidInput
	}
	return m.withRuntimeError(sessionID, func(runtime Runtime) error {
		return runtime.AnswerQuestion(promptID, answers)
	})
}

// AnswerMCPInteraction durably records an MCP elicitation action before the
// plugin call resumes. Form values are carried only in memory for this call.
func (m *RuntimeManager) AnswerMCPInteraction(sessionID, promptID, action string, content map[string]any) (SessionView, error) {
	if strings.TrimSpace(promptID) == "" || strings.TrimSpace(action) == "" {
		return SessionView{}, ErrInvalidInput
	}
	return m.withRuntimeError(sessionID, func(runtime Runtime) error {
		return runtime.AnswerMCPInteraction(promptID, action, content)
	})
}

// ReplayPendingPrompts re-emits a prompt that survived a bridge reconnect, so
// a newly attached desktop host can rebuild its actionable card.
func (m *RuntimeManager) ReplayPendingPrompts(sessionID string) (SessionView, error) {
	return m.withRuntime(sessionID, func(runtime Runtime) {
		runtime.ReplayPendingPrompts()
	})
}

// RenameSession persists a user-selected display title in the core session
// metadata. The controller must be idle so title writes cannot race a turn.
func (m *RuntimeManager) RenameSession(sessionID, title string) (SessionView, error) {
	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 120 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return SessionView{}, ErrInvalidTitle
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return SessionView{}, ErrSessionNotFound
	}
	if state := m.runtime.State(); state != "idle" {
		return SessionView{}, fmt.Errorf("%w: cannot rename a %s session", ErrSessionConflict, state)
	}
	if err := m.runtime.Rename(title); err != nil {
		return SessionView{}, err
	}
	m.view.Title = m.runtime.Title()
	view := m.view
	view.State = m.runtime.State()
	return view, nil
}

// DeleteSession removes the session this manager owns, together with every
// durable artifact the core records for it. Only the owned session may be
// deleted: the manager holds the one controller, so an unowned path could be
// running under a different host or absent from this profile entirely. The
// operation is idempotent — a session whose files are already gone still
// reports success, so a retried request cannot turn a completed delete into a
// 404. After the sweep the controller is released without another durable
// snapshot, because that snapshot would recreate the file just deleted.
func (m *RuntimeManager) DeleteSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return ErrSessionNotFound
	}
	if state := m.runtime.State(); state != "idle" {
		return fmt.Errorf("%w: cannot delete a %s session", ErrSessionConflict, state)
	}
	if err := m.runtime.Delete(); err != nil {
		return err
	}
	m.runtime = nil
	m.view = SessionView{}
	return nil
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
	runtime, view, err := buildRuntime(ctx, factory, request)
	if err != nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, err
	}
	if m.finishOpen(runtime, view) {
		return view, nil
	}
	_ = runtime.Shutdown()
	return SessionView{}, ErrClosed
}

func buildRuntime(ctx context.Context, factory RuntimeFactory, request OpenRequest) (Runtime, SessionView, error) {
	runtime, err := factory.Open(ctx, request)
	if err != nil {
		return nil, SessionView{}, err
	}
	if runtime == nil {
		return nil, SessionView{}, errors.New("desktop bridge runtime factory returned nil runtime")
	}
	path := strings.TrimSpace(runtime.SessionPath())
	if path == "" {
		_ = runtime.Shutdown()
		return nil, SessionView{}, errors.New("desktop bridge runtime has no session path")
	}
	view := SessionView{
		ID:            request.SessionID,
		Path:          path,
		Title:         runtime.Title(),
		WorkspaceRoot: request.WorkspaceRoot,
		State:         runtime.State(),
	}
	return runtime, view, nil
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

func (m *RuntimeManager) withRuntimeError(sessionID string, action func(Runtime) error) (SessionView, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return SessionView{}, ErrSessionNotFound
	}
	if err := action(m.runtime); err != nil {
		return SessionView{}, err
	}
	view := m.view
	view.State = m.runtime.State()
	return view, nil
}
