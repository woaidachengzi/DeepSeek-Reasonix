package desktopbridge

import (
	"context"
	"encoding/json"
)

// SessionManager routes host operations to the exact live session owner.
// Snapshot and CommandSnapshot refer to the selected view; SessionSnapshot
// and inventory methods observe background owners without selecting them.
type SessionManager interface {
	ChangeArchive(sessionID string, archived bool, commit func(bool) error) error
	PrepareCombinedRewind(sessionID string, turn int) (WorkspaceCombinedRewindPlan, error)
	CommitCombinedRewind(sessionID, planID string, confirmed bool) (WorkspaceCombinedRewindResult, error)
	ForkConversation(sessionID, forkToken string) (LegacyConversationForkResult, error)
	PrepareConversationRewind(sessionID string, turn int) (WorkspaceConversationRewindPlan, error)
	CommitConversationRewind(sessionID, planID string) (WorkspaceConversationRewindResult, error)
	UndoConversationRewind(sessionID, headID string) (WorkspaceConversationRewindResult, error)
	SetSessionEffort(ctx context.Context, sessionID, model, effort string) (SessionView, error)
	PrepareLegacyConversationFork(sessionID string, turn int) (WorkspaceConversationRewindPlan, error)
	CommitLegacyConversationFork(sessionID, planID string) (LegacyConversationForkResult, error)
	ObserveLocalReclaim(ctx context.Context, view OwnedCommandView) (*OwnedLocalReclaimObservation, error)
	ReadOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt) (json.RawMessage, error)
	CommandSnapshot() (OwnedCommandView, bool)
	SubmitOwned(ctx context.Context, view OwnedCommandView, input string) error
	ResolveOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt, answer json.RawMessage) error
	PinnedFiles(sessionID string) ([]PinnedFile, error)
	ChangePinnedFile(sessionID, path string, pin bool) ([]PinnedFile, error)
	Open(ctx context.Context, request OpenRequest) (SessionView, error)
	Switch(ctx context.Context, request OpenRequest) (SessionView, error)
	SetSessionModel(ctx context.Context, sessionID, modelRef string) (SessionView, error)
	Snapshot() (SessionView, bool)
	SessionApproval(sessionID, mode string) (string, error)
	BoundShell(sessionID string) (string, bool)
	SessionMetrics(sessionID string) (SessionMetrics, bool)
	SessionBalance(ctx context.Context, sessionID string) (*SessionBalance, bool, error)
	MemoryRecall(sessionID, workspaceRoot string) (MemoryRecallView, bool)
	SetReasoningLanguage(sessionID, language string) bool
	MCPStatus(workspaceRoot string) ([]MCPRuntimeServer, bool)
	MCPRuntimeAction(sessionID, name, action string) (int, error)
	StartMCPOAuth(sessionID, name string) (string, error)
	MCPOAuthStatus(sessionID, flowID string) (MCPAuthFlow, error)
	CancelMCPOAuth(sessionID, flowID string) error
	ClearMCPAuthentication(sessionID, name string) (bool, error)
	History(sessionID string) (HistoryView, error)
	Steer(sessionID, input string) (SessionView, error)
	Continue(ctx context.Context, sessionID string) (SessionView, error)
	Submit(ctx context.Context, sessionID, input string) (SessionView, error)
	AttachFile(sessionID, path string) (AttachmentView, error)
	WorkspaceTarget(sessionID string) (WorkspaceTarget, error)
	Workspace(sessionID, path string) (WorkspaceList, error)
	WorkspaceFilePreview(sessionID, path string) (WorkspaceFilePreview, error)
	WorkspaceChanges(sessionID string) (WorkspaceChanges, error)
	WorkspaceChangeDetail(sessionID, path string) (WorkspaceChangeDetail, error)
	PrepareWorkspaceFileRevert(sessionID, path string) (WorkspaceFileRevertPlan, error)
	CommitWorkspaceFileRevert(sessionID, planID, resolution string) (WorkspaceFileRevertResult, error)
	UndoWorkspaceFileRevert(sessionID, transactionID string) (WorkspaceFileRevertResult, error)
	WorkspaceCheckpoints(sessionID string) ([]WorkspaceCheckpointView, error)
	PrepareCodeRewind(sessionID string, turn int) (WorkspaceCodeRewindPlan, error)
	CommitCodeRewind(sessionID, planID string, confirmPartialCoverage bool) (WorkspaceFileRevertResult, error)
	Cancel(sessionID string) (SessionView, error)
	Approve(sessionID, promptID string, allow bool) (SessionView, error)
	AnswerQuestion(sessionID, promptID string, answers []AskAnswer) (SessionView, error)
	AnswerMCPInteraction(sessionID, promptID, action string, content map[string]any) (SessionView, error)
	ReplayPendingPrompts(sessionID string) (SessionView, error)
	RenameSession(sessionID, title string) (SessionView, error)
	DeleteSession(sessionID string) error
	Shutdown() error
	SessionHeads(sessionID string) ([]SessionHeadView, error)
	SwitchSessionHead(sessionID, headID string) error
	RebuildSettings(ctx context.Context, sessionID string) (SessionView, error)
	ApplySettings(ctx context.Context, sessionID string) (SessionView, error)
	TerminalWorkspace(sessionID string) (TerminalWorkspaceView, error)
	CreateTerminal(ctx context.Context, sessionID, path, shellID string) (TerminalSessionView, error)
	TerminalOutput(sessionID, terminalID string) (TerminalOutputView, error)
	WriteTerminal(sessionID, terminalID string, data []byte) error
	ResizeTerminal(sessionID, terminalID string, columns, rows int) error
	RenameTerminal(sessionID, terminalID, title string) error
	CloseTerminal(sessionID, terminalID string) error
	WorkspaceImage(sessionID, source string) (WorkspaceImageView, error)
	WorkspaceImageWithRemote(ctx context.Context, sessionID, source string, remote func(context.Context, string) WorkspaceImageView) (WorkspaceImageView, error)
	SessionSnapshot(sessionID string) (SessionView, bool)
	Snapshots() []SessionView
	CommandSnapshots() []OwnedCommandView
	CommandSessionSnapshot(sessionID string) (OwnedCommandView, bool)
}

var _ SessionManager = (*RuntimeManager)(nil)
var _ SessionManager = (*RuntimePool)(nil)
