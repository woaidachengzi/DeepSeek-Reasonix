package main

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
)

func TestNoDeliverableHistoryAfterReopenIsVisibleAndSafe(t *testing.T) {
	session := agent.NewSession("private system")
	session.Add(provider.Message{Role: provider.RoleUser, Origin: provider.MessageOriginUser, Content: "question"})
	session.Add(provider.Message{
		Role: provider.RoleTool, LocalOnly: true, ToolCallID: provider.LocalOnlyToolID,
		Content: "private tool data", ReasoningContent: "private reasoning",
		InterruptedTurn: &provider.InterruptedTurnRecovery{TerminalStatus: "failed", Pending: true,
			FailureDiagnostic: &provider.FailureDiagnostic{Kind: "no_deliverable_output", TraceID: "private trace"}},
	})
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	history := projectBridgeHistory(loaded.Snapshot())
	if len(history) != 2 || history[1].Role != "assistant" || history[1].Content != i18n.M.ProviderErrNoDeliverable || history[1].ForkToken != "" {
		t.Fatalf("failure missing or actionable as a model answer: %+v", history)
	}
	for _, m := range history {
		if strings.Contains(m.Content, "private") {
			t.Fatal("private failure data crossed the display boundary")
		}
	}
}
