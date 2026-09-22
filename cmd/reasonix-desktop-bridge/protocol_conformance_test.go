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
		{Name: "openSessionRequest", Sample: openSessionRequest{}},
		{Name: "attachFileRequest", Sample: attachFileRequest{}},
		{Name: "attachment", Sample: desktopbridge.AttachmentView{}},
		{Name: "attachmentResponse", Sample: attachmentResponse{}},
		{Name: "workspaceRequest", Sample: workspaceRequest{}},
		{Name: "workspaceEntry", Sample: desktopbridge.WorkspaceEntry{}},
		{Name: "workspaceListResponse", Sample: workspaceListResponse{}},
		{Name: "workspaceFileRequest", Sample: workspaceFileRequest{}},
		{Name: "workspaceFilePreview", Sample: desktopbridge.WorkspaceFilePreview{}},
		{Name: "workspaceFileResponse", Sample: workspaceFileResponse{}},
		{Name: "workspaceChangeView", Sample: desktopbridge.WorkspaceChangeView{}},
		{Name: "workspaceChanges", Sample: desktopbridge.WorkspaceChanges{}},
		{Name: "workspaceChangesResponse", Sample: workspaceChangesResponse{}},
		{Name: "workspaceChangeDetailRequest", Sample: workspaceChangeDetailRequest{}},
		{Name: "workspaceChangeDetail", Sample: desktopbridge.WorkspaceChangeDetail{}},
		{Name: "workspaceChangeDetailResponse", Sample: workspaceChangeDetailResponse{}},
		{Name: "submitRequest", Sample: submitRequest{}},
		{Name: "approvalRequest", Sample: approveRequest{}},
		{Name: "askAnswer", Sample: desktopbridge.AskAnswer{}},
		{Name: "answerQuestionRequest", Sample: answerQuestionRequest{}},
		{Name: "mcpInteractionAnswerRequest", Sample: answerMCPInteractionRequest{}},
		{Name: "deleteSessionResponse", Sample: deleteSessionResponse{}},
		{Name: "historyMessage", Sample: desktopbridge.HistoryMessage{}},
		{Name: "historyResponse", Sample: historyResponse{}},
	}
	if err := protocolgen.CheckGoDTOs(root, definitions); err != nil {
		t.Fatalf("Go DTOs drifted from %s: %v", protocolgen.SchemaPath, err)
	}
}
