package control

import (
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestNoDeliverableTurnFailsAndRemainsInHistory(t *testing.T) {
	isolateControlConfigHome(t)
	p := &scriptedTurns{turns: [][]provider.Chunk{
		{{Type: provider.ChunkReasoning, Text: "thinking"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkReasoning, Text: "still thinking"}, {Type: provider.ChunkDone}},
		textTurn("next answer"),
	}}
	ag := agent.New(p, tool.NewRegistry(), agent.NewSession("system"), agent.Options{}, event.Discard)
	events := make(chan event.Event, 2)
	dir := t.TempDir()
	c := New(Options{Runner: ag, Executor: ag, SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Label: "test", Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			events <- e
		}
	})})
	defer c.Close()
	c.Submit("answer me")
	done := receiveCheckpointTurnDone(t, events)
	err := done.Err
	if !errors.Is(err, provider.ErrNoDeliverableOutput) || p.call != 2 {
		t.Fatalf("Run: calls=%d err=%v", p.call, err)
	}
	if explained := explainError(err); !errors.Is(explained, provider.ErrNoDeliverableOutput) || explained.Error() != i18n.M.ProviderErrNoDeliverable {
		t.Fatalf("localized failure lost identity: %v", explained)
	}
	if done.Diagnostic == nil || done.Diagnostic.Kind != "no_deliverable_output" {
		t.Fatal("terminal event lost the failure classification")
	}
	loaded, err := agent.LoadSession(c.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, m := range loaded.Snapshot() {
		if r := m.InterruptedTurn; m.LocalOnly && r != nil && r.TerminalStatus == "failed" && r.FailureDiagnostic != nil && r.FailureDiagnostic.Kind == "no_deliverable_output" {
			failed = true
		}
	}
	if !failed {
		t.Fatal("reopened controller history lost the failure")
	}
	c.Submit("continue")
	if err := receiveCheckpointTurnDone(t, events).Err; err != nil || p.call != 3 {
		t.Fatalf("continue: calls=%d err=%v", p.call, err)
	}
}
