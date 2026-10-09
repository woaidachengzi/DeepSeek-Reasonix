package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/protocolgen"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/remote/controller"
)

func TestRemotePromptSchemaDiscriminatesDecisionKinds(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", protocolgen.SchemaPath))
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil)
	const resource = "https://reasonix.local/schemas/desktop-bridge/v1.json"
	if err := compiler.AddResource(resource, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(resource + "#/$defs/remoteControllerSessionPromptRequest")
	if err != nil {
		t.Fatal(err)
	}
	root, err := compiler.Compile(resource)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind, answer string
		valid        bool
	}{
		{"ask", `{"questions":[]}`, true},
		{"approval", `{"allow":false}`, true},
		{"approval", `{"allow":true,"session":true}`, true},
		{"plan", `{"action":"exit_plan"}`, true},
		{"recovery", `{"action":"revise","feedback":"adjust"}`, true},
		{"mcp", `{"action":"accept","content":{"answer":true}}`, true},
		{"mcp", `{"action":"cancel"}`, true},
		{"ask", `{"allow":true}`, false},
		{"approval", `{"allow":true,"action":"accept"}`, false},
		{"approval", `{"allow":false,"persist":true}`, false},
		{"approval", `{"allow":true,"session":true,"persist":true}`, false},
		{"plan", `{"action":"continue"}`, false},
		{"recovery", `{"action":"exit_plan"}`, false},
		{"mcp", `{"action":"decline","content":{}}`, false},
	} {
		var answer any
		if json.Unmarshal([]byte(test.answer), &answer) != nil {
			t.Fatal("bad fixture")
		}
		value := map[string]any{"sessionPath": "/remote/session.jsonl", "runtimeEpoch": "instance", "turnId": "turn", "promptId": "prompt", "promptRuntimeEpoch": "", "kind": test.kind, "answer": answer}
		if err := schema.Validate(value); (err == nil) != test.valid {
			t.Fatalf("schema kind %s answer %s valid=%v: %v", test.kind, test.answer, test.valid, err)
		}
		if test.valid {
			if err := root.Validate(value); err != nil {
				t.Fatalf("root schema omitted prompt request: %v", err)
			}
		}
	}
}

// The bridge server owns the wire format, so its hand-written DTOs are the Go
// half of the contract that the generated TypeScript and Rust mirrors follow.
func TestGoDTOsMatchTheWireSchema(t *testing.T) {
	root := filepath.Join("..", "..")
	definitions := []protocolgen.Definition{
		{Name: "remoteControllerSessionPromptRequest", Sample: remoteControllerSessionPromptRequest{}},
		{Name: "remoteControllerSessionPromptReceipt", Sample: remoteControllerSessionPromptReceipt{}},
		{Name: "remoteControllerSessionPromptResponse", Sample: remoteControllerSessionPromptResponse{}},
		{Name: "remoteControllerPromptQuestionAnswer", Sample: controller.SessionPromptQuestionAnswer{}},
		{Name: "remoteControllerSessionSubmitRequest", Sample: controller.SessionSubmitRequest{}},
		{Name: "remoteControllerSessionSubmitReceipt", Sample: controller.SessionSubmitReceipt{}},
		{Name: "remoteControllerSessionSubmitResponse", Sample: remoteControllerSessionSubmitResponse{}},
		{Name: "remoteControllerSessionCancelRequest", Sample: controller.SessionCancelScope{}},
		{Name: "remoteControllerSessionCancelReceipt", Sample: controller.SessionCancelReceipt{}},
		{Name: "remoteControllerSessionCancelResponse", Sample: remoteControllerSessionCancelResponse{}},
		{Name: "remoteControllerProjectionRequest", Sample: remoteControllerProjectionRequest{}},
		{Name: "remoteControllerProjectionReplay", Sample: controller.ProjectionReplay{}},
		{Name: "remoteControllerSessionProjection", Sample: controller.SessionProjection{}},
		{Name: "remoteControllerSessionProjectionResponse", Sample: remoteControllerSessionProjectionResponse{}},
		{Name: "remoteControllerSessionViewRequest", Sample: remoteControllerSessionViewRequest{}},
		{Name: "remoteControllerSessionImageRequest", Sample: remoteControllerSessionImageRequest{}},
		{Name: "remoteControllerSessionImageResponse", Sample: remoteControllerSessionImageResponse{}},
		{Name: "remoteControllerSessionImageView", Sample: controller.SessionImageView{}},
		{Name: "remoteControllerImage", Sample: controller.SessionImage{}},
		{Name: "remoteControllerSessionViewResponse", Sample: remoteControllerSessionViewResponse{}},
		{Name: "remoteControllerSessionEvent", Sample: remoteControllerSessionEvent{}},
		{Name: "remoteControllerSessionView", Sample: controller.SessionView{}},
		{Name: "remoteControllerHistoryMessage", Sample: controller.HistoryMessage{}},
		{Name: "remoteControllerHistoryToolCall", Sample: controller.HistoryToolCall{}},
		{Name: "remoteControllerHistorySearch", Sample: controller.HistorySearch{}},
		{Name: "remoteControllerHistorySearchHit", Sample: provider.ServerSearchHit{}},
		{Name: "remoteControllerProtocolRecovery", Sample: provider.ProtocolRecoveryAction{}},
		{Name: "remoteControllerRuntimeState", Sample: event.RuntimeStateSnapshot{}},
		{Name: "remoteControllerRequest", Sample: remoteControllerRequest{}},
		{Name: "remoteControllerView", Sample: remoteControllerView{}},
		{Name: "remoteControllerSession", Sample: controller.Session{}},
		{Name: "remoteControllerResponse", Sample: remoteControllerResponse{}},
		{Name: "remoteControllerSessionsResponse", Sample: remoteControllerSessionsResponse{}},
		{Name: "remoteControllerCloseResponse", Sample: remoteControllerCloseResponse{}},
		{Name: "terminalCreateRequest", Sample: terminalCreateRequest{}},
		{Name: "terminalInputRequest", Sample: terminalInputRequest{}},
		{Name: "terminalResizeRequest", Sample: terminalResizeRequest{}},
		{Name: "terminalRenameRequest", Sample: terminalRenameRequest{}},
		{Name: "terminalShellView", Sample: desktopbridge.TerminalShellView{}},
		{Name: "terminalSessionView", Sample: desktopbridge.TerminalSessionView{}},
		{Name: "terminalWorkspaceView", Sample: desktopbridge.TerminalWorkspaceView{}},
		{Name: "terminalOutputView", Sample: desktopbridge.TerminalOutputView{}},
		{Name: "terminalExitView", Sample: desktopbridge.TerminalExitView{}},
		{Name: "terminalWorkspaceResponse", Sample: terminalWorkspaceResponse{}},
		{Name: "terminalSessionResponse", Sample: terminalSessionResponse{}},
		{Name: "terminalOutputResponse", Sample: terminalOutputResponse{}},
		{Name: "terminalActionResponse", Sample: terminalActionResponse{}},
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
