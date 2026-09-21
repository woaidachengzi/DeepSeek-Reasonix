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
		{Name: "submitRequest", Sample: submitRequest{}},
		{Name: "deleteSessionResponse", Sample: deleteSessionResponse{}},
		{Name: "historyMessage", Sample: desktopbridge.HistoryMessage{}},
		{Name: "historyResponse", Sample: historyResponse{}},
	}
	if err := protocolgen.CheckGoDTOs(root, definitions); err != nil {
		t.Fatalf("Go DTOs drifted from %s: %v", protocolgen.SchemaPath, err)
	}
}
