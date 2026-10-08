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
	ErrClosed                   = errors.New("desktop bridge runtime manager is closed")
	ErrOpenInProgress           = errors.New("desktop bridge session open is already in progress")
	ErrSessionConflict          = errors.New("desktop bridge already owns a different session")
	ErrInvalidSessionID         = errors.New("desktop bridge session id is required")
	ErrInvalidInput             = errors.New("desktop bridge input is required")
	ErrInvalidAttachment        = errors.New("desktop bridge attachment path is invalid")
	ErrInvalidWorkspacePath     = errors.New("desktop bridge workspace path is invalid")
	ErrWorkspaceFileNotFound    = errors.New("desktop bridge workspace file was not found")
	ErrWorkspaceFileAmbiguous   = errors.New("desktop bridge workspace file reference is ambiguous")
	ErrWorkspaceFileUnavailable = errors.New("desktop bridge workspace file is unavailable")
	ErrInvalidTitle             = errors.New("desktop bridge session title is invalid")
	ErrSessionNotFound          = errors.New("desktop bridge session was not found")
	ErrSessionModelSwitch       = errors.New("desktop bridge session model could not be switched")
	ErrSessionModelRecover      = errors.New("desktop bridge session model switch recovery failed")
)

// OpenRequest identifies the one local session the first bridge release owns.
// The caller is responsible for resolving a user-facing ID to a stable core
// session path before constructing the real Runtime.
type OpenRequest struct {
	SessionID     string
	WorkspaceRoot string
	// ModelRef is an optional per-session override. Empty follows effective config.
	ModelRef string
	Effort   *string
}

// SessionView is transport-safe runtime metadata. It intentionally excludes
// history and provider details; those gain their own versioned contracts.
type SessionView struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Title         string `json:"title,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	ModelRef      string `json:"modelRef,omitempty"`
	Effort        string `json:"effort,omitempty"`
	State         string `json:"state"`
}

// WorkspaceTarget is the live runtime's directory, separate from project
// catalog metadata: a Global conversation has no project assignment.
type WorkspaceTarget struct {
	SessionID     string `json:"sessionId"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

type RuntimeWorkspaceProvider interface {
	LocalWorkspace() (string, error)
}

// HistoryMessage is the deliberately small, display-safe transcript projection
// the bridge may return to a desktop host. It must never contain provider
// reasoning, tool arguments/results, system prompts, image data, or other
// persistence metadata.
type HistoryMessage struct {
	Role           string            `json:"role"`
	Content        string            `json:"content"`
	Truncated      bool              `json:"truncated,omitempty"`
	WorkDurationMs int64             `json:"workDurationMs,omitempty"`
	CreatedAtMs    int64             `json:"createdAtMs,omitempty"`
	TurnUsage      *HistoryTurnUsage `json:"turnUsage,omitempty"`
}

// HistoryTurnUsage contains only local numeric accounting for one user turn.
// Reasoning is a subset of output tokens, not an extra charge added to total.
type HistoryTurnUsage struct {
	InputTokens     int64  `json:"inputTokens"`
	OutputTokens    int64  `json:"outputTokens"`
	TotalTokens     int64  `json:"totalTokens"`
	RequestCount    int64  `json:"requestCount"`
	ReasoningTokens int64  `json:"reasoningTokens"`
	CacheHitTokens  *int64 `json:"cacheHitTokens,omitempty"`
	CacheMissTokens *int64 `json:"cacheMissTokens,omitempty"`
	Estimated       bool   `json:"estimated"`
	Complete        bool   `json:"complete"`
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

type WorkspaceFileRevertPlan struct {
	PlanID         string   `json:"planId,omitempty"`
	Path           string   `json:"path"`
	CanFiles       bool     `json:"canFiles"`
	DisabledReason string   `json:"disabledReason,omitempty"`
	Conflicts      []string `json:"conflicts,omitempty"`
	Legacy         bool     `json:"legacy,omitempty"`
}

type WorkspaceFileRevertResult struct {
	OK            bool     `json:"ok"`
	TransactionID string   `json:"transactionId,omitempty"`
	UndoAvailable bool     `json:"undoAvailable"`
	WrittenCount  int      `json:"writtenCount"`
	DeletedCount  int      `json:"deletedCount"`
	Error         string   `json:"error,omitempty"`
	Conflicts     []string `json:"conflicts,omitempty"`
}

type WorkspaceCheckpointView struct {
	Turn          int    `json:"turn"`
	Prompt        string `json:"prompt"`
	Time          int64  `json:"time"`
	TurnFileCount int    `json:"turnFileCount"`
}

type WorkspaceCodeRewindPlan struct {
	PlanID                       string   `json:"planId,omitempty"`
	Turn                         int      `json:"turn"`
	CanFiles                     bool     `json:"canFiles"`
	FileCount                    int      `json:"fileCount"`
	Files                        []string `json:"files"`
	FilesTruncated               bool     `json:"filesTruncated"`
	Coverage                     string   `json:"coverage"`
	CoverageGaps                 []string `json:"coverageGaps"`
	RequiresCoverageConfirmation bool     `json:"requiresCoverageConfirmation"`
	DisabledReason               string   `json:"disabledReason,omitempty"`
	Conflicts                    []string `json:"conflicts"`
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
	PrepareWorkspaceFileRevert(path string) (WorkspaceFileRevertPlan, error)
	CommitWorkspaceFileRevert(planID, resolution string) (WorkspaceFileRevertResult, error)
	UndoWorkspaceFileRevert(transactionID string) (WorkspaceFileRevertResult, error)
	WorkspaceCheckpoints() []WorkspaceCheckpointView
	PrepareCodeRewind(turn int) (WorkspaceCodeRewindPlan, error)
	CommitCodeRewind(planID string, confirmPartialCoverage bool) (WorkspaceFileRevertResult, error)
	Submit(input string)
	Cancel()
	Approve(promptID string, allow bool)
	AnswerQuestion(promptID string, answers []AskAnswer) error
	AnswerMCPInteraction(promptID, action string, content map[string]any) error
	ReplayPendingPrompts()
	Shutdown() error
}

// RuntimeModelProvider exposes the current controller model when it can be
// identified. It is optional so host-neutral test runtimes remain lightweight.
type RuntimeModelProvider interface {
	ModelRef() string
}

// RuntimeShellProvider exposes only the interpreter identity bound to the
// active controller generation. The executable path is kept out of this live
// session view; the settings inventory reports host paths separately.
type RuntimeShellProvider interface {
	BoundShell() string
}

// MCPRuntimeTool is display-safe metadata from the active session's MCP Host.
// It deliberately excludes tool schemas, arguments, results, and credentials.
type MCPRuntimeTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type MCPRuntimeServer struct {
	Name              string           `json:"name"`
	Status            string           `json:"status"`
	ToolCount         int              `json:"toolCount,omitempty"`
	Tools             []MCPRuntimeTool `json:"tools,omitempty"`
	ProtocolVersion   string           `json:"protocolVersion,omitempty"`
	SessionState      string           `json:"sessionState,omitempty"`
	ReconnectAttempts int              `json:"reconnectAttempts,omitempty"`
	ErrorKind         string           `json:"errorKind,omitempty"`
}

// MCPRuntimeStatusProvider is optional, so host-neutral fake runtimes and
// non-MCP runtimes retain the small base Runtime contract.
type MCPRuntimeStatusProvider interface {
	MCPRuntimeStatus() []MCPRuntimeServer
}

// MCPRuntimeActionProvider exposes explicit current-session connect/disconnect
// actions without widening the base Runtime contract used by headless hosts.
type MCPRuntimeActionProvider interface {
	MCPRuntimeAction(name, action string) (int, error)
}

// MCPOAuthProvider runs browser-based authorization asynchronously for the
// exact active session. OAuth tokens remain in the core's private state store.
type MCPOAuthProvider interface {
	StartMCPOAuth(name string) (string, error)
	MCPOAuthStatus(flowID string) (MCPAuthFlow, error)
	CancelMCPOAuth(flowID string) error
}

// MCPCredentialProvider clears only the selected server's Reasonix OAuth state
// and configured static credentials, then removes that server from the active
// session's Host.
type MCPCredentialProvider interface {
	ClearMCPAuthentication(name string) (bool, error)
}

// MCPAuthFlow is the renderer-safe projection of a browser authorization flow.
// It never contains authorization URLs, tokens, or provider error text.
type MCPAuthFlow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// SessionMetrics contains only current-session status values backed by the
// active controller. Zero contextWindowTokens means no reliable gauge yet.
type SessionMetrics struct {
	ContextUsedTokens       int `json:"contextUsedTokens"`
	ContextWindowTokens     int `json:"contextWindowTokens"`
	CompactThresholdPercent int `json:"compactThresholdPercent"`
	CacheHitTokens          int `json:"cacheHitTokens"`
	CacheMissTokens         int `json:"cacheMissTokens"`
}

// SessionMetricsProvider is optional for runtimes that can read these values.
type SessionMetricsProvider interface {
	SessionMetrics() SessionMetrics
}

// SessionBalance is an optional wallet readout from the active runtime. It
// contains only display-safe fields and never includes the endpoint or key.
type SessionBalance struct {
	Available bool   `json:"available"`
	Display   string `json:"display"`
}

// SessionBalanceProvider is optional because many runtimes/providers do not
// expose a wallet balance.
type SessionBalanceProvider interface {
	SessionBalance(context.Context) (*SessionBalance, error)
}

// MemoryRecallHit is the safe settings-panel projection of one automatically
// recalled memory. It intentionally omits local filesystem paths.
type MemoryRecallHit struct {
	ID        string  `json:"id"`
	Revision  int     `json:"revision"`
	Name      string  `json:"name"`
	Title     string  `json:"title,omitempty"`
	Type      string  `json:"type"`
	Scope     string  `json:"scope"`
	Score     float64 `json:"score"`
	Freshness string  `json:"freshness"`
	Reason    string  `json:"reason"`
	Snippet   string  `json:"snippet"`
}

// MemoryRecallView reports the latest automatic recall from the active
// in-memory controller. This is a live-session view, not persisted history.
type MemoryRecallView struct {
	Query      string            `json:"query"`
	Hits       []MemoryRecallHit `json:"hits"`
	Omitted    int               `json:"omitted"`
	CharBudget int               `json:"charBudget"`
	UsedChars  int               `json:"usedChars"`
	Suppressed string            `json:"suppressed,omitempty"`
}

// MemoryRecallProvider is optional for runtimes that own memory recall state.
type MemoryRecallProvider interface {
	MemoryRecall() MemoryRecallView
}

// ReasoningLanguageProvider applies the configured visible-reasoning language
// to an already-running controller.
type ReasoningLanguageProvider interface {
	SetReasoningLanguage(string)
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

// RuntimeOpenAdmission performs read-only policy validation before Switch
// releases the current controller. Archive errors must preserve that owner.
type RuntimeOpenAdmission interface{ ValidateOpen(OpenRequest) error }

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
	if admission, ok := m.factory.(RuntimeOpenAdmission); ok {
		if err := admission.ValidateOpen(request); err != nil {
			m.mu.Unlock()
			return SessionView{}, err
		}
	}
	if m.runtime != nil {
		view := m.view
		state := m.runtime.State()
		m.mu.Unlock()
		if state == "deleting" {
			return SessionView{}, fmt.Errorf("%w: session deletion is in progress", ErrSessionConflict)
		}
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
	if admission, ok := m.factory.(RuntimeOpenAdmission); ok {
		if err := admission.ValidateOpen(request); err != nil {
			m.mu.Unlock()
			return SessionView{}, err
		}
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
		if m.runtime.State() == "deleting" {
			m.mu.Unlock()
			return SessionView{}, fmt.Errorf("%w: session deletion is in progress", ErrSessionConflict)
		}
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

// SetSessionModel rebuilds the active controller for the same transcript with
// an explicit model override. The old controller is durably shut down first;
// if constructing the replacement fails, the previous model is reopened.
func (m *RuntimeManager) SetSessionModel(ctx context.Context, sessionID, modelRef string) (SessionView, error) {
	sessionID = strings.TrimSpace(sessionID)
	modelRef = strings.TrimSpace(modelRef)
	if sessionID == "" || modelRef == "" {
		return SessionView{}, ErrInvalidInput
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil || m.view.ID != sessionID {
		m.mu.Unlock()
		return SessionView{}, ErrSessionNotFound
	}
	if m.opening {
		m.mu.Unlock()
		return SessionView{}, ErrOpenInProgress
	}
	if state := m.runtime.State(); state != "idle" {
		m.mu.Unlock()
		return SessionView{}, fmt.Errorf("%w: active session %q is %s", ErrSessionConflict, sessionID, state)
	}
	previous := m.runtime
	previousRequest := OpenRequest{SessionID: m.view.ID, WorkspaceRoot: m.view.WorkspaceRoot, ModelRef: m.view.ModelRef}
	if m.view.Effort != "" {
		effort := m.view.Effort
		previousRequest.Effort = &effort
	}
	if previousRequest.ModelRef == "" {
		if provider, ok := previous.(RuntimeModelProvider); ok {
			previousRequest.ModelRef = strings.TrimSpace(provider.ModelRef())
		}
	}
	if previousRequest.ModelRef == modelRef {
		view := m.view
		m.mu.Unlock()
		return view, nil
	}
	factory := m.factory
	if factory == nil {
		m.mu.Unlock()
		return SessionView{}, errors.New("desktop bridge runtime factory is not configured")
	}
	request := previousRequest
	request.ModelRef = modelRef
	request.Effort = nil // A model switch starts with that model's own default.
	m.runtime = nil
	m.view = SessionView{}
	m.opening = true
	m.mu.Unlock()

	if err := previous.Shutdown(); err != nil {
		m.finishOpen(nil, SessionView{})
		return SessionView{}, fmt.Errorf("%w: close active session: %v", ErrSessionModelRecover, err)
	}
	runtime, view, err := buildRuntime(ctx, factory, request)
	if err == nil {
		if m.finishOpen(runtime, view) {
			return view, nil
		}
		_ = runtime.Shutdown()
		return SessionView{}, ErrClosed
	}

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
			ErrSessionModelRecover,
			fmt.Errorf("switch active session model: %w", err),
			fmt.Errorf("restore previous session model: %w", recoveryErr),
		)
	}
	if !m.finishOpen(recovered, recoveredView) {
		_ = recovered.Shutdown()
		return SessionView{}, ErrClosed
	}
	return SessionView{}, errors.Join(ErrSessionModelSwitch, fmt.Errorf("switch active session model: %w; previous model restored", err))
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

// RuntimeApprovalProvider exposes the active controller posture, separate from
// desktop defaults and permission/sandbox policy.
type RuntimeApprovalProvider interface {
	ApprovalMode() string
	SetApprovalMode(string) error
}

// SessionApproval serializes ownership, idle-state validation and persistence.
// An empty mode is a read; a write never drains a pending approval prompt.
func (m *RuntimeManager) SessionApproval(sessionID, mode string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", ErrClosed
	}
	if m.runtime == nil || m.view.ID != sessionID || sessionID == "" {
		return "", ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeApprovalProvider)
	if !ok {
		return "", fmt.Errorf("approval mode is unavailable")
	}
	if mode != "" {
		if mode != "ask" && mode != "auto" && mode != "yolo" {
			return "", fmt.Errorf("invalid approval mode")
		}
		if m.opening || m.runtime.State() != "idle" {
			return "", ErrSessionConflict
		}
		if err := provider.SetApprovalMode(mode); err != nil {
			return "", err
		}
	}
	return provider.ApprovalMode(), nil
}

// BoundShell reports the interpreter captured by the exact active session's
// controller. It never falls back to another session or to current config.
func (m *RuntimeManager) BoundShell(sessionID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtime == nil || m.view.ID != sessionID {
		return "", false
	}
	provider, ok := m.runtime.(RuntimeShellProvider)
	if !ok {
		return "", false
	}
	shell := provider.BoundShell()
	return shell, shell != ""
}

// SessionMetrics returns metrics only for the exact active session ID.
func (m *RuntimeManager) SessionMetrics(sessionID string) (SessionMetrics, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtime == nil || m.view.ID != sessionID {
		return SessionMetrics{}, false
	}
	provider, ok := m.runtime.(SessionMetricsProvider)
	if !ok {
		return SessionMetrics{}, false
	}
	return provider.SessionMetrics(), true
}

// SessionBalance queries the active controller only for its exact session ID.
// The runtime call happens outside the manager lock because it can perform IO.
func (m *RuntimeManager) SessionBalance(ctx context.Context, sessionID string) (*SessionBalance, bool, error) {
	m.mu.Lock()
	if m.closed || m.runtime == nil || m.view.ID != sessionID {
		m.mu.Unlock()
		return nil, false, nil
	}
	provider, ok := m.runtime.(SessionBalanceProvider)
	m.mu.Unlock()
	if !ok {
		return nil, true, nil
	}
	balance, err := provider.SessionBalance(ctx)
	return balance, true, err
}

// MemoryRecall returns the active session's latest recall only when both its
// identity and workspace match. This prevents settings for another project
// from displaying unrelated session activity.
func (m *RuntimeManager) MemoryRecall(sessionID, workspaceRoot string) (MemoryRecallView, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtime == nil || m.view.ID != sessionID || m.view.WorkspaceRoot != strings.TrimSpace(workspaceRoot) {
		return MemoryRecallView{}, false
	}
	provider, ok := m.runtime.(MemoryRecallProvider)
	if !ok {
		return MemoryRecallView{}, false
	}
	return provider.MemoryRecall(), true
}

// SetReasoningLanguage updates only the matching active session when its core
// runtime supports live language changes.
func (m *RuntimeManager) SetReasoningLanguage(sessionID, language string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtime == nil || m.view.ID != sessionID {
		return false
	}
	provider, ok := m.runtime.(ReasoningLanguageProvider)
	if !ok {
		return false
	}
	provider.SetReasoningLanguage(language)
	return true
}

// MCPStatus reads only the current session's Host and only for its workspace.
// A different selected project must never inherit the active session's status.
func (m *RuntimeManager) MCPStatus(workspaceRoot string) ([]MCPRuntimeServer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtime == nil || m.view.WorkspaceRoot != strings.TrimSpace(workspaceRoot) {
		return nil, false
	}
	provider, ok := m.runtime.(MCPRuntimeStatusProvider)
	if !ok {
		return nil, false
	}
	return provider.MCPRuntimeStatus(), true
}

// MCPRuntimeAction applies a connect or disconnect to the exact active session.
// The active session identity check prevents a delayed settings action from
// mutating a different conversation after navigation.
func (m *RuntimeManager) MCPRuntimeAction(sessionID, name, action string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
		return 0, ErrInvalidInput
	}
	if action != "connect" && action != "disconnect" {
		return 0, ErrInvalidInput
	}
	var count int
	_, err := m.withRuntimeError(sessionID, func(runtime Runtime) error {
		if runtime.State() != "idle" {
			return fmt.Errorf("%w: MCP connection changes require an idle session", ErrSessionConflict)
		}
		provider, ok := runtime.(MCPRuntimeActionProvider)
		if !ok {
			return fmt.Errorf("%w: MCP runtime actions are unavailable", ErrSessionConflict)
		}
		var actionErr error
		count, actionErr = provider.MCPRuntimeAction(name, action)
		return actionErr
	})
	return count, err
}

// StartMCPOAuth begins authorization for an idle exact session. The provider
// returns immediately; the browser flow runs in its own bounded context.
func (m *RuntimeManager) StartMCPOAuth(sessionID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
		return "", ErrInvalidInput
	}
	var flowID string
	_, err := m.withRuntimeError(sessionID, func(runtime Runtime) error {
		if runtime.State() != "idle" {
			return fmt.Errorf("%w: MCP authorization requires an idle session", ErrSessionConflict)
		}
		provider, ok := runtime.(MCPOAuthProvider)
		if !ok {
			return fmt.Errorf("%w: MCP authorization is unavailable", ErrSessionConflict)
		}
		var startErr error
		flowID, startErr = provider.StartMCPOAuth(name)
		return startErr
	})
	return flowID, err
}

// MCPOAuthStatus and CancelMCPOAuth bind polling and cancellation to the same
// active session identity used to start the flow.
func (m *RuntimeManager) MCPOAuthStatus(sessionID, flowID string) (MCPAuthFlow, error) {
	var result MCPAuthFlow
	_, err := m.withRuntimeError(sessionID, func(runtime Runtime) error {
		provider, ok := runtime.(MCPOAuthProvider)
		if !ok {
			return fmt.Errorf("%w: MCP authorization is unavailable", ErrSessionConflict)
		}
		var statusErr error
		result, statusErr = provider.MCPOAuthStatus(flowID)
		return statusErr
	})
	return result, err
}

func (m *RuntimeManager) CancelMCPOAuth(sessionID, flowID string) error {
	_, err := m.withRuntimeError(sessionID, func(runtime Runtime) error {
		provider, ok := runtime.(MCPOAuthProvider)
		if !ok {
			return fmt.Errorf("%w: MCP authorization is unavailable", ErrSessionConflict)
		}
		return provider.CancelMCPOAuth(flowID)
	})
	return err
}

func (m *RuntimeManager) ClearMCPAuthentication(sessionID, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
		return false, ErrInvalidInput
	}
	var changed bool
	_, err := m.withRuntimeError(sessionID, func(runtime Runtime) error {
		if runtime.State() != "idle" {
			return fmt.Errorf("%w: clearing MCP credentials requires an idle session", ErrSessionConflict)
		}
		provider, ok := runtime.(MCPCredentialProvider)
		if !ok {
			return fmt.Errorf("%w: MCP credential management is unavailable", ErrSessionConflict)
		}
		var clearErr error
		changed, clearErr = provider.ClearMCPAuthentication(name)
		return clearErr
	})
	return changed, err
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
// the event transport added on top of this lifecycle layer. Saved settings are
// applied first, so a key written after the controller froze it still reaches
// the provider.
func (m *RuntimeManager) Submit(ctx context.Context, sessionID, input string) (SessionView, error) {
	if strings.TrimSpace(input) == "" {
		return SessionView{}, ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.rebuildSettingsLocked(ctx, strings.TrimSpace(sessionID)); err != nil {
		return SessionView{}, err
	}
	m.runtime.Submit(input)
	view := m.view
	view.State = m.runtime.State()
	return view, nil
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

// WorkspaceTarget resolves the live runtime under the ownership lock so a
// concurrent switch or shutdown cannot substitute another session's directory.
func (m *RuntimeManager) WorkspaceTarget(sessionID string) (WorkspaceTarget, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceTarget{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceTarget{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeWorkspaceProvider)
	if !ok {
		return WorkspaceTarget{}, ErrInvalidWorkspacePath
	}
	root, err := provider.LocalWorkspace()
	if err != nil {
		return WorkspaceTarget{}, err
	}
	if strings.TrimSpace(root) == "" {
		return WorkspaceTarget{}, ErrInvalidWorkspacePath
	}
	return WorkspaceTarget{SessionID: m.view.ID, WorkspaceRoot: root}, nil
}

// Workspace lists one directory in the owned session workspace under the same
// lock as session switching and shutdown.
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

func (m *RuntimeManager) PrepareWorkspaceFileRevert(sessionID, path string) (WorkspaceFileRevertPlan, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceFileRevertPlan{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceFileRevertPlan{}, ErrSessionNotFound
	}
	return m.runtime.PrepareWorkspaceFileRevert(path)
}

func (m *RuntimeManager) CommitWorkspaceFileRevert(sessionID, planID, resolution string) (WorkspaceFileRevertResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceFileRevertResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceFileRevertResult{}, ErrSessionNotFound
	}
	return m.runtime.CommitWorkspaceFileRevert(planID, resolution)
}

func (m *RuntimeManager) UndoWorkspaceFileRevert(sessionID, transactionID string) (WorkspaceFileRevertResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceFileRevertResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceFileRevertResult{}, ErrSessionNotFound
	}
	return m.runtime.UndoWorkspaceFileRevert(transactionID)
}

func (m *RuntimeManager) WorkspaceCheckpoints(sessionID string) ([]WorkspaceCheckpointView, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return nil, ErrSessionNotFound
	}
	return m.runtime.WorkspaceCheckpoints(), nil
}

func (m *RuntimeManager) PrepareCodeRewind(sessionID string, turn int) (WorkspaceCodeRewindPlan, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceCodeRewindPlan{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceCodeRewindPlan{}, ErrSessionNotFound
	}
	return m.runtime.PrepareCodeRewind(turn)
}

func (m *RuntimeManager) CommitCodeRewind(sessionID, planID string, confirmPartialCoverage bool) (WorkspaceFileRevertResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceFileRevertResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceFileRevertResult{}, ErrSessionNotFound
	}
	return m.runtime.CommitCodeRewind(planID, confirmPartialCoverage)
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
	if state := m.runtime.State(); state != "idle" && state != "deleting" {
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
	view, err := runtimeView(runtime, request)
	if err != nil && runtime != nil {
		_ = runtime.Shutdown()
	}
	return runtime, view, err
}

func runtimeView(runtime Runtime, request OpenRequest) (SessionView, error) {
	if runtime == nil {
		return SessionView{}, errors.New("desktop bridge runtime factory returned nil runtime")
	}
	path := strings.TrimSpace(runtime.SessionPath())
	if path == "" {
		return SessionView{}, errors.New("desktop bridge runtime has no session path")
	}
	view := SessionView{
		ID: request.SessionID, Path: path, Title: runtime.Title(),
		WorkspaceRoot: request.WorkspaceRoot, ModelRef: request.ModelRef, State: runtime.State(),
	}
	if provider, ok := runtime.(RuntimeModelProvider); ok {
		if modelRef := strings.TrimSpace(provider.ModelRef()); modelRef != "" {
			view.ModelRef = modelRef
		}
	}
	if provider, ok := runtime.(RuntimeEffortProvider); ok {
		view.Effort = provider.Effort()
	}
	return view, nil
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
	if m.runtime.State() == "deleting" {
		return SessionView{}, fmt.Errorf("%w: session deletion is in progress", ErrSessionConflict)
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
	if m.runtime.State() == "deleting" {
		return SessionView{}, fmt.Errorf("%w: session deletion is in progress", ErrSessionConflict)
	}
	if err := action(m.runtime); err != nil {
		return SessionView{}, err
	}
	view := m.view
	view.State = m.runtime.State()
	return view, nil
}
