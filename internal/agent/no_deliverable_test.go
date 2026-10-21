package agent

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func noAnswerTurn(text string) []provider.Chunk {
	return []provider.Chunk{
		{Type: provider.ChunkReasoning, Text: text},
		{Type: provider.ChunkUsage, Usage: &provider.Usage{FinishReason: "stop", PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10}},
		{Type: provider.ChunkDone},
	}
}

func TestNoDeliverableRecoveryDoesNotResetAfterTools(t *testing.T) {
	p := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		noAnswerTurn("need to inspect"),
		{toolCallChunk("once", "echo", `{"text":"inspection result"}`), {Type: provider.ChunkDone}},
		noAnswerTurn("still thinking"),
		textTurn("new user turn"),
	}}
	a := New(p, echoRegistry(), NewSession(""), Options{}, event.Discard)
	if err := a.Run(t.Context(), "answer me"); !errors.Is(err, provider.ErrNoDeliverableOutput) {
		t.Fatalf("Run = %v", err)
	}
	if p.call != 3 {
		t.Fatalf("calls = %d; tool progress replenished recovery", p.call)
	}
	prefix := p.requests[0].Messages
	if !reflect.DeepEqual(prefix, p.requests[1].Messages[:len(prefix)]) {
		t.Fatal("recovery rewrote the original prefix")
	}
	tools := 0
	for _, m := range a.Session().Snapshot() {
		if m.Role == provider.RoleTool && m.ToolCallID == "once" {
			tools++
		}
	}
	if tools != 1 {
		t.Fatalf("tool results = %d", tools)
	}
	if err := a.Run(t.Context(), "continue"); err != nil || p.call != 4 {
		t.Fatalf("next user turn: calls=%d err=%v", p.call, err)
	}
}

func TestNoDeliverableFailureSurvivesReopen(t *testing.T) {
	p := &scriptedProvider{name: "p", turns: [][]provider.Chunk{noAnswerTurn("one"), noAnswerTurn("two")}}
	a := New(p, echoRegistry(), NewSession(""), Options{}, event.Discard)
	if err := a.Run(t.Context(), "answer me"); !errors.Is(err, provider.ErrNoDeliverableOutput) {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := a.Session().Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range loaded.Snapshot() {
		if r := m.InterruptedTurn; m.LocalOnly && r != nil && r.FailureDiagnostic != nil && r.FailureDiagnostic.Kind == "no_deliverable_output" {
			if r.TerminalStatus != "failed" {
				t.Fatalf("terminal status = %q", r.TerminalStatus)
			}
			return
		}
	}
	t.Fatal("reopened history lost the terminal failure")
}

func TestNoDeliverableRecoveryHonorsCancellationAndBudget(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "cancel"}[cancel], func(t *testing.T) {
			p := &scriptedProvider{name: "p", turns: [][]provider.Chunk{noAnswerTurn("thinking")}}
			ctx, stop := context.WithCancel(t.Context())
			defer stop()
			opts := Options{}
			if !cancel {
				opts.TaskBudget = TaskBudget{Tokens: 1}
			}
			a := New(p, echoRegistry(), NewSession(""), opts, event.FuncSink(func(e event.Event) {
				if cancel && e.Kind == event.Usage {
					stop()
				}
			}))
			err := a.Run(ctx, "answer me")
			var budget *taskBudgetPause
			if cancel && !errors.Is(err, context.Canceled) || !cancel && !errors.As(err, &budget) {
				t.Fatalf("Run = %v", err)
			}
			if p.call != 1 || sessionHasUserMessageContaining(a.sess.conversation, "visible answer") {
				t.Fatal("continuation bypassed the stop boundary")
			}
		})
	}
}
