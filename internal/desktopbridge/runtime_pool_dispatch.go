package desktopbridge

import (
	"context"
	"encoding/json"
)

// Each dispatch captures a slot, then uses its existing ownership lock and
// epoch checks. Navigation never lends one session another session's runtime.
func (p *RuntimePool) PrepareCombinedRewind(sessionID string, turn int) (WorkspaceCombinedRewindPlan, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceCombinedRewindPlan{}, err
	}
	return slot.PrepareCombinedRewind(sessionID, turn)
}

func (p *RuntimePool) CommitCombinedRewind(sessionID, planID string, confirmed bool) (WorkspaceCombinedRewindResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceCombinedRewindResult{}, err
	}
	return slot.CommitCombinedRewind(sessionID, planID, confirmed)
}

func (p *RuntimePool) ForkConversation(sessionID, forkToken string) (LegacyConversationForkResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return LegacyConversationForkResult{}, err
	}
	return slot.ForkConversation(sessionID, forkToken)
}

func (p *RuntimePool) PrepareConversationRewind(sessionID string, turn int) (WorkspaceConversationRewindPlan, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceConversationRewindPlan{}, err
	}
	return slot.PrepareConversationRewind(sessionID, turn)
}

func (p *RuntimePool) CommitConversationRewind(sessionID, planID string) (WorkspaceConversationRewindResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceConversationRewindResult{}, err
	}
	return slot.CommitConversationRewind(sessionID, planID)
}

func (p *RuntimePool) UndoConversationRewind(sessionID, headID string) (WorkspaceConversationRewindResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceConversationRewindResult{}, err
	}
	return slot.UndoConversationRewind(sessionID, headID)
}

func (p *RuntimePool) SetSessionEffort(ctx context.Context, sessionID, model, effort string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.SetSessionEffort(ctx, sessionID, model, effort)
}

func (p *RuntimePool) PrepareLegacyConversationFork(sessionID string, turn int) (WorkspaceConversationRewindPlan, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceConversationRewindPlan{}, err
	}
	return slot.PrepareLegacyConversationFork(sessionID, turn)
}

func (p *RuntimePool) CommitLegacyConversationFork(sessionID, planID string) (LegacyConversationForkResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return LegacyConversationForkResult{}, err
	}
	return slot.CommitLegacyConversationFork(sessionID, planID)
}

func (p *RuntimePool) ObserveLocalReclaim(ctx context.Context, view OwnedCommandView) (*OwnedLocalReclaimObservation, error) {
	slot, err := p.slot(view.Scope.SessionID)
	if err != nil {
		return nil, ErrOwnedRuntimeChanged
	}
	return slot.ObserveLocalReclaim(ctx, view)
}

func (p *RuntimePool) ReadOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt) (json.RawMessage, error) {
	slot, err := p.slot(scope.SessionID)
	if err != nil {
		return nil, ErrOwnedRuntimeChanged
	}
	return slot.ReadOwnedPrompt(ctx, scope, prompt)
}

func (p *RuntimePool) SubmitOwned(ctx context.Context, view OwnedCommandView, input string) error {
	slot, err := p.slot(view.Scope.SessionID)
	if err != nil {
		return ErrOwnedRuntimeChanged
	}
	return slot.SubmitOwned(ctx, view, input)
}

func (p *RuntimePool) ResolveOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt, answer json.RawMessage) error {
	slot, err := p.slot(scope.SessionID)
	if err != nil {
		return ErrOwnedRuntimeChanged
	}
	return slot.ResolveOwnedPrompt(ctx, scope, prompt, answer)
}

func (p *RuntimePool) PinnedFiles(sessionID string) ([]PinnedFile, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, err
	}
	return slot.PinnedFiles(sessionID)
}

func (p *RuntimePool) ChangePinnedFile(sessionID, path string, pin bool) ([]PinnedFile, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, err
	}
	return slot.ChangePinnedFile(sessionID, path, pin)
}

func (p *RuntimePool) SetSessionModel(ctx context.Context, sessionID, modelRef string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.SetSessionModel(ctx, sessionID, modelRef)
}

func (p *RuntimePool) SessionApproval(sessionID, mode string) (string, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return "", err
	}
	return slot.SessionApproval(sessionID, mode)
}

func (p *RuntimePool) BoundShell(sessionID string) (string, bool) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return "", false
	}
	return slot.BoundShell(sessionID)
}

func (p *RuntimePool) SessionMetrics(sessionID string) (SessionMetrics, bool) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionMetrics{}, false
	}
	return slot.SessionMetrics(sessionID)
}

func (p *RuntimePool) SessionBalance(ctx context.Context, sessionID string) (*SessionBalance, bool, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, false, err
	}
	return slot.SessionBalance(ctx, sessionID)
}

func (p *RuntimePool) MemoryRecall(sessionID, workspaceRoot string) (MemoryRecallView, bool) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return MemoryRecallView{}, false
	}
	return slot.MemoryRecall(sessionID, workspaceRoot)
}

func (p *RuntimePool) SetReasoningLanguage(sessionID, language string) bool {
	slot, err := p.slot(sessionID)
	if err != nil {
		return false
	}
	return slot.SetReasoningLanguage(sessionID, language)
}

func (p *RuntimePool) MCPRuntimeAction(sessionID, name, action string) (int, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return 0, err
	}
	return slot.MCPRuntimeAction(sessionID, name, action)
}

func (p *RuntimePool) StartMCPOAuth(sessionID, name string) (string, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return "", err
	}
	return slot.StartMCPOAuth(sessionID, name)
}

func (p *RuntimePool) MCPOAuthStatus(sessionID, flowID string) (MCPAuthFlow, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return MCPAuthFlow{}, err
	}
	return slot.MCPOAuthStatus(sessionID, flowID)
}

func (p *RuntimePool) CancelMCPOAuth(sessionID, flowID string) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.CancelMCPOAuth(sessionID, flowID)
}

func (p *RuntimePool) ClearMCPAuthentication(sessionID, name string) (bool, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return false, err
	}
	return slot.ClearMCPAuthentication(sessionID, name)
}

func (p *RuntimePool) History(sessionID string) (HistoryView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return HistoryView{}, err
	}
	return slot.History(sessionID)
}

func (p *RuntimePool) Submit(ctx context.Context, sessionID, input string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.Submit(ctx, sessionID, input)
}

func (p *RuntimePool) AttachFile(sessionID, path string) (AttachmentView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return AttachmentView{}, err
	}
	return slot.AttachFile(sessionID, path)
}

func (p *RuntimePool) WorkspaceTarget(sessionID string) (WorkspaceTarget, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceTarget{}, err
	}
	return slot.WorkspaceTarget(sessionID)
}

func (p *RuntimePool) Workspace(sessionID, path string) (WorkspaceList, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceList{}, err
	}
	return slot.Workspace(sessionID, path)
}

func (p *RuntimePool) WorkspaceFilePreview(sessionID, path string) (WorkspaceFilePreview, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	return slot.WorkspaceFilePreview(sessionID, path)
}

func (p *RuntimePool) WorkspaceChanges(sessionID string) (WorkspaceChanges, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceChanges{}, err
	}
	return slot.WorkspaceChanges(sessionID)
}

func (p *RuntimePool) WorkspaceChangeDetail(sessionID, path string) (WorkspaceChangeDetail, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceChangeDetail{}, err
	}
	return slot.WorkspaceChangeDetail(sessionID, path)
}

func (p *RuntimePool) PrepareWorkspaceFileRevert(sessionID, path string) (WorkspaceFileRevertPlan, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceFileRevertPlan{}, err
	}
	return slot.PrepareWorkspaceFileRevert(sessionID, path)
}

func (p *RuntimePool) CommitWorkspaceFileRevert(sessionID, planID, resolution string) (WorkspaceFileRevertResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceFileRevertResult{}, err
	}
	return slot.CommitWorkspaceFileRevert(sessionID, planID, resolution)
}

func (p *RuntimePool) UndoWorkspaceFileRevert(sessionID, transactionID string) (WorkspaceFileRevertResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceFileRevertResult{}, err
	}
	return slot.UndoWorkspaceFileRevert(sessionID, transactionID)
}

func (p *RuntimePool) WorkspaceCheckpoints(sessionID string) ([]WorkspaceCheckpointView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, err
	}
	return slot.WorkspaceCheckpoints(sessionID)
}

func (p *RuntimePool) PrepareCodeRewind(sessionID string, turn int) (WorkspaceCodeRewindPlan, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceCodeRewindPlan{}, err
	}
	return slot.PrepareCodeRewind(sessionID, turn)
}

func (p *RuntimePool) CommitCodeRewind(sessionID, planID string, confirmPartialCoverage bool) (WorkspaceFileRevertResult, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceFileRevertResult{}, err
	}
	return slot.CommitCodeRewind(sessionID, planID, confirmPartialCoverage)
}

func (p *RuntimePool) Cancel(sessionID string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.Cancel(sessionID)
}

func (p *RuntimePool) Approve(sessionID, promptID string, allow bool) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.Approve(sessionID, promptID, allow)
}

func (p *RuntimePool) AnswerQuestion(sessionID, promptID string, answers []AskAnswer) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.AnswerQuestion(sessionID, promptID, answers)
}

func (p *RuntimePool) AnswerMCPInteraction(sessionID, promptID, action string, content map[string]any) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.AnswerMCPInteraction(sessionID, promptID, action, content)
}

func (p *RuntimePool) ReplayPendingPrompts(sessionID string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.ReplayPendingPrompts(sessionID)
}

func (p *RuntimePool) RenameSession(sessionID, title string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.RenameSession(sessionID, title)
}

func (p *RuntimePool) SessionHeads(sessionID string) ([]SessionHeadView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, err
	}
	return slot.SessionHeads(sessionID)
}

func (p *RuntimePool) SwitchSessionHead(sessionID, headID string) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.SwitchSessionHead(sessionID, headID)
}

func (p *RuntimePool) RebuildSettings(ctx context.Context, sessionID string) (SessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	return slot.RebuildSettings(ctx, sessionID)
}

func (p *RuntimePool) TerminalWorkspace(sessionID string) (TerminalWorkspaceView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return TerminalWorkspaceView{}, err
	}
	return slot.TerminalWorkspace(sessionID)
}

func (p *RuntimePool) CreateTerminal(ctx context.Context, sessionID, path, shellID string) (TerminalSessionView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return TerminalSessionView{}, err
	}
	return slot.CreateTerminal(ctx, sessionID, path, shellID)
}

func (p *RuntimePool) TerminalOutput(sessionID, terminalID string) (TerminalOutputView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return TerminalOutputView{}, err
	}
	return slot.TerminalOutput(sessionID, terminalID)
}

func (p *RuntimePool) WriteTerminal(sessionID, terminalID string, data []byte) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.WriteTerminal(sessionID, terminalID, data)
}

func (p *RuntimePool) ResizeTerminal(sessionID, terminalID string, columns, rows int) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.ResizeTerminal(sessionID, terminalID, columns, rows)
}

func (p *RuntimePool) RenameTerminal(sessionID, terminalID, title string) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.RenameTerminal(sessionID, terminalID, title)
}

func (p *RuntimePool) CloseTerminal(sessionID, terminalID string) error {
	slot, err := p.slot(sessionID)
	if err != nil {
		return err
	}
	return slot.CloseTerminal(sessionID, terminalID)
}

func (p *RuntimePool) WorkspaceImage(sessionID, source string) (WorkspaceImageView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceImageView{}, err
	}
	return slot.WorkspaceImage(sessionID, source)
}

func (p *RuntimePool) WorkspaceImageWithRemote(ctx context.Context, sessionID, source string, remote func(context.Context, string) WorkspaceImageView) (WorkspaceImageView, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return WorkspaceImageView{}, err
	}
	return slot.WorkspaceImageWithRemote(ctx, sessionID, source, remote)
}
