package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/protocolgen"
)

// The bridge server owns the wire format, so its hand-written DTOs are the Go
// half of the contract that the generated TypeScript and Rust mirrors follow.
func TestGoDTOsMatchTheWireSchema(t *testing.T) {
	root := filepath.Join("..", "..")
	definitions := []protocolgen.Definition{
		{Name: "session", Sample: desktopbridge.SessionView{}},
		{Name: "event", Sample: desktopbridge.Event{}},
		{Name: "health", Sample: healthResponse{}},
		{Name: "providerSummary", Sample: providerSummaryEntry{}},
		{Name: "providerReasoningSummary", Sample: providerReasoningSummary{}},
		{Name: "providerSummaryResponse", Sample: providerSummaryResponse{ReasoningLanguage: "auto", CompactRatioPercent: 80}},
		{Name: "setModelRoleRequest", Sample: setModelRoleRequest{}},
		{Name: "setSessionModelRequest", Sample: setSessionModelRequest{}},
		{Name: "setAgentPreferenceRequest", Sample: setAgentPreferenceRequest{CompactRatioPercent: 80}},
		{Name: "openSessionRequest", Sample: openSessionRequest{}},
		{Name: "attachFileRequest", Sample: attachFileRequest{}},
		{Name: "attachment", Sample: desktopbridge.AttachmentView{}},
		{Name: "attachmentResponse", Sample: attachmentResponse{}},
		{Name: "workspaceRequest", Sample: workspaceRequest{}},
		{Name: "workspaceEntry", Sample: desktopbridge.WorkspaceEntry{}},
		{Name: "workspaceListResponse", Sample: workspaceListResponse{}},
		{Name: "workspaceTargetResponse", Sample: workspaceTargetResponse{}},
		{Name: "workspaceFileRequest", Sample: workspaceFileRequest{}},
		{Name: "workspaceFilePreview", Sample: desktopbridge.WorkspaceFilePreview{}},
		{Name: "workspaceFileResponse", Sample: workspaceFileResponse{}},
		{Name: "workspaceChangeView", Sample: desktopbridge.WorkspaceChangeView{}},
		{Name: "workspaceChanges", Sample: desktopbridge.WorkspaceChanges{}},
		{Name: "workspaceChangesResponse", Sample: workspaceChangesResponse{}},
		{Name: "workspaceChangeDetailRequest", Sample: workspaceChangeDetailRequest{}},
		{Name: "workspaceChangeDetail", Sample: desktopbridge.WorkspaceChangeDetail{}},
		{Name: "workspaceChangeDetailResponse", Sample: workspaceChangeDetailResponse{}},
		{Name: "workspaceFileRevertPlan", Sample: desktopbridge.WorkspaceFileRevertPlan{}},
		{Name: "workspaceFileRevertPlanResponse", Sample: workspaceFileRevertPlanResponse{}},
		{Name: "workspaceFileRevertCommitRequest", Sample: workspaceFileRevertCommitRequest{}},
		{Name: "workspaceFileRevertUndoRequest", Sample: workspaceFileRevertUndoRequest{}},
		{Name: "workspaceFileRevertResult", Sample: desktopbridge.WorkspaceFileRevertResult{}},
		{Name: "workspaceFileRevertResultResponse", Sample: workspaceFileRevertResultResponse{}},
		{Name: "workspaceCheckpointView", Sample: desktopbridge.WorkspaceCheckpointView{}},
		{Name: "workspaceCheckpointsResponse", Sample: workspaceCheckpointsResponse{}},
		{Name: "codeRewindPreviewRequest", Sample: codeRewindPreviewRequest{}},
		{Name: "workspaceCodeRewindPlan", Sample: desktopbridge.WorkspaceCodeRewindPlan{}},
		{Name: "codeRewindPlanResponse", Sample: codeRewindPlanResponse{}},
		{Name: "codeRewindCommitRequest", Sample: codeRewindCommitRequest{}},
		{Name: "conversationRewindPreviewRequest", Sample: conversationRewindPreviewRequest{}},
		{Name: "workspaceConversationRewindPlan", Sample: desktopbridge.WorkspaceConversationRewindPlan{}},
		{Name: "conversationRewindPlanResponse", Sample: conversationRewindPlanResponse{}},
		{Name: "conversationRewindCommitRequest", Sample: conversationRewindCommitRequest{}},
		{Name: "conversationRewindUndoRequest", Sample: conversationRewindUndoRequest{}},
		{Name: "workspaceConversationRewindResult", Sample: desktopbridge.WorkspaceConversationRewindResult{}},
		{Name: "conversationRewindResultResponse", Sample: conversationRewindResultResponse{}},
		{Name: "sessionHeadView", Sample: desktopbridge.SessionHeadView{}},
		{Name: "sessionHeadsResponse", Sample: sessionHeadsResponse{}},
		{Name: "sessionHeadSwitchRequest", Sample: sessionHeadSwitchRequest{}},
		{Name: "sessionHeadSwitchResponse", Sample: sessionHeadSwitchResponse{}},
		{Name: "combinedRewindPreviewRequest", Sample: combinedRewindPreviewRequest{}},
		{Name: "workspaceCombinedRewindPlan", Sample: desktopbridge.WorkspaceCombinedRewindPlan{}},
		{Name: "combinedRewindPlanResponse", Sample: combinedRewindPlanResponse{}},
		{Name: "combinedRewindCommitRequest", Sample: combinedRewindCommitRequest{}},
		{Name: "workspaceCombinedRewindResult", Sample: desktopbridge.WorkspaceCombinedRewindResult{}},
		{Name: "combinedRewindResultResponse", Sample: combinedRewindResultResponse{}},
		{Name: "legacyConversationForkResult", Sample: desktopbridge.LegacyConversationForkResult{}},
		{Name: "legacyConversationForkResultResponse", Sample: legacyConversationForkResultResponse{}},
		{Name: "submitRequest", Sample: submitRequest{}},
		{Name: "approvalRequest", Sample: approveRequest{}},
		{Name: "askAnswer", Sample: desktopbridge.AskAnswer{}},
		{Name: "answerQuestionRequest", Sample: answerQuestionRequest{}},
		{Name: "mcpInteractionAnswerRequest", Sample: answerMCPInteractionRequest{}},
		{Name: "deleteSessionResponse", Sample: deleteSessionResponse{}},
		{Name: "historyTurnUsage", Sample: desktopbridge.HistoryTurnUsage{}},
		{Name: "historyMessage", Sample: desktopbridge.HistoryMessage{}},
		{Name: "historyResponse", Sample: historyResponse{}},
	}
	if err := protocolgen.CheckGoDTOs(root, definitions); err != nil {
		t.Fatalf("Go DTOs drifted from %s: %v", protocolgen.SchemaPath, err)
	}
}
