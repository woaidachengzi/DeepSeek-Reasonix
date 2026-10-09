package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestRunProfileSpecExplicitEphemeralNeedsNoStoreAndRejectsContinuation(t *testing.T) {
	root := t.TempDir()
	p := &mockProvider{name: "owned", chunks: []provider.Chunk{{Type: provider.ChunkText, Text: "owned answer"}, {Type: provider.ChunkDone}}}
	runner := NewTaskToolWithOptions(TaskToolOptions{Provider: p, ParentRegistry: tool.NewRegistry(), SysPrompt: "profile prompt", MaxSteps: 3}).
		WithTranscripts(nil, root, "", "").WithScheduler(NewSubagentScheduler(1, 1))
	spec := ProfileExecSpec{Task: TaskSpec{Objective: "inspect"}, Worker: WorkerSpec{Kind: "task", Name: "preview", SystemPrompt: "profile prompt", UseProfilePrompt: true}, Grant: CapabilityGrant{ReadOnly: true, AllowNoTools: true}, Context: ContextRequest{Ephemeral: true}}
	ctx := WithParentSession(context.Background(), filepath.Join(root, "parent.jsonl"))
	answer, err := runner.RunProfileSpec(ctx, spec)
	if err != nil || !strings.Contains(answer, "owned answer") {
		t.Fatalf("ephemeral: %q %v", answer, err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatal("ephemeral spec wrote parent or child transcript")
	}
	before := len(p.requests)
	for _, field := range []string{"continue", "fork"} {
		invalid := spec
		if field == "continue" {
			invalid.Context.ContinueFrom = "persisted-ref"
		} else {
			invalid.Context.ForkFrom = "persisted-ref"
		}
		if _, err := runner.RunProfileSpec(ctx, invalid); err == nil || !strings.Contains(err.Error(), "ephemeral subagent cannot continue") {
			t.Fatalf("continuation: %v", err)
		}
	}
	if len(p.requests) != before {
		t.Fatal("rejected continuation dispatched provider")
	}
}
