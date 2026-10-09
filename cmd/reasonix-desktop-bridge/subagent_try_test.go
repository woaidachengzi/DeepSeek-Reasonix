package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	configpkg "reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type previewTryRecordingProvider struct {
	request provider.Request
	calls   int
}

func (p *previewTryRecordingProvider) Name() string { return "owned-fixture" }
func (p *previewTryRecordingProvider) Stream(_ context.Context, request provider.Request) (<-chan provider.Chunk, error) {
	p.request = request
	p.calls++
	chunks := make(chan provider.Chunk, 3)
	chunks <- provider.Chunk{Type: provider.ChunkReasoning, Text: "PRIVATE reasoning"}
	chunks <- provider.Chunk{Type: provider.ChunkText, Text: "Visible answer"}
	chunks <- provider.Chunk{Type: provider.ChunkDone}
	close(chunks)
	return chunks, nil
}

func TestPreviewSubagentUnifiedRunnerIsEphemeralReadOnlyAndVisible(t *testing.T) {
	root := t.TempDir()
	p := &previewTryRecordingProvider{}
	run := &subagentTryRun{}
	ctx := agent.WithParentSession(context.Background(), filepath.Join(root, "parent.jsonl"))
	result, err := runReadOnlySubagentTryWithOptions(ctx, root, "inspect", agent.TaskToolOptions{
		Provider: p, ParentRegistry: readOnlySubagentTryToolRegistry(configpkg.Default(), root, []string{"read_file", "write_file"}), SysPrompt: "EXACT profile prompt", MaxSteps: 12,
	}, subagentTryTextSink{run: run})
	if err != nil || !strings.Contains(result, "Visible answer") || p.calls != 1 {
		t.Fatalf("run: %q %v calls=%d", result, err, p.calls)
	}
	if run.snapshot() != result || strings.Contains(run.snapshot(), "PRIVATE") {
		t.Fatal("preview must expose final answer, not reasoning")
	}
	if len(p.request.Messages) == 0 || p.request.Messages[0].Content != "EXACT profile prompt" {
		t.Fatal("profile prompt changed")
	}
	for _, schema := range p.request.Tools {
		if schema.Name != "read_file" {
			t.Fatalf("unexpected tool grant: %s", schema.Name)
		}
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatalf("ephemeral try created durable artifacts: %v %v", files, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	before := p.calls
	if _, err := runReadOnlySubagentTryWithOptions(cancelled, root, "inspect", agent.TaskToolOptions{Provider: p, ParentRegistry: tool.NewRegistry(), SysPrompt: "prompt"}, event.Discard); err == nil {
		t.Fatal("cancelled try must fail")
	}
	if p.calls != before {
		t.Fatal("cancelled try dispatched provider")
	}
}

func TestRunReadOnlySubagentTryRequiresTaskAndPrompt(t *testing.T) {
	if _, err := runReadOnlySubagentTry(context.Background(), "", previewSubagentProfileInput{SystemPrompt: "prompt"}, " ", event.Discard); err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("missing task error = %v", err)
	}
	if _, err := runReadOnlySubagentTry(context.Background(), "", previewSubagentProfileInput{}, "task", event.Discard); err == nil || !strings.Contains(err.Error(), "system prompt is required") {
		t.Fatalf("missing prompt error = %v", err)
	}
}

func TestSubagentTryTextSinkPublishesOnlyVisibleAnswerAndBoundsOutput(t *testing.T) {
	run := &subagentTryRun{}
	sink := subagentTryTextSink{run: run}
	sink.Emit(event.Event{Kind: event.Reasoning, Text: "private reasoning"})
	sink.Emit(event.Event{Kind: event.ToolProgress, Tool: event.Tool{Name: event.SubagentProgressReasoningName, Output: "private reasoning"}})
	sink.Emit(event.Event{Kind: event.Text, Text: "discarded partial"})
	sink.Emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{Action: event.StreamAttemptDiscard}})
	sink.Emit(event.Event{Kind: event.Text, Text: "Visible "})
	sink.Emit(event.Event{Kind: event.Text, Text: "answer"})
	sink.Emit(event.Event{Kind: event.Message, Text: "Visible answer"})
	if got := run.snapshot(); got != "Visible answer" {
		t.Fatalf("snapshot = %q, want visible answer only", got)
	}
	sink.Emit(event.Event{Kind: event.ToolProgress, Tool: event.Tool{Name: event.SubagentProgressStatusName, Output: "retrying"}})
	sink.Emit(event.Event{Kind: event.ToolProgress, Tool: event.Tool{Name: event.SubagentProgressTextName, Output: "new preview"}})
	if got := run.snapshot(); got != "new preview" {
		t.Fatalf("unified preview: %q", got)
	}
	run.append(strings.Repeat("x", maxSubagentTryResultRunes+1))
	if got := run.snapshot(); !strings.HasSuffix(got, "… output truncated") || len([]rune(got)) > maxSubagentTryResultRunes+20 {
		t.Fatalf("oversized snapshot not bounded or marked truncated: rune count=%d suffix=%q", len([]rune(got)), got[len(got)-30:])
	}
}

func TestSubagentTryStatusEndpointReturnsCurrentOutput(t *testing.T) {
	server := &bridgeServer{subagentTryRun: &subagentTryRun{}}
	server.subagentTryRun.append("partial")
	response := httptest.NewRecorder()
	server.subagentProfileTryStatus(response, httptest.NewRequest("GET", "/v1/settings/subagents/try/status", nil))
	var status previewSubagentTryStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || !status.Running || status.Output != "partial" {
		t.Fatalf("status = %#v, code=%d", status, response.Code)
	}
}

func TestPreviewSubagentTryUsesReadOnlyTools(t *testing.T) {
	registry := readOnlySubagentTryToolRegistry(configpkg.Default(), t.TempDir(), []string{"read_file", "write_file", "grep"})
	for _, name := range []string{"write_file", "edit_file", "multi_edit", "delete_range", "run_skill", "parallel_tasks"} {
		if _, ok := registry.Get(name); ok {
			t.Errorf("Preview try registry must not expose %q; tools=%v", name, registry.Names())
		}
	}
	if _, ok := registry.Get("read_file"); !ok {
		t.Fatalf("allowlisted read_file missing; tools=%v", registry.Names())
	}
	if _, ok := registry.Get("ls"); ok {
		t.Fatalf("non-allowlisted ls should not be exposed; tools=%v", registry.Names())
	}
	readOnlyRegistry := readOnlySubagentTryToolRegistry(configpkg.Default(), t.TempDir(), nil)
	bash, ok := readOnlyRegistry.Get("bash")
	if !ok || !bash.ReadOnly() {
		t.Fatalf("bash must use the read-only wrapper, got tool=%v present=%v", bash, ok)
	}
	_, err := bash.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /tmp/x"}`))
	if _, blocked := tool.BlockedMessage(err); !blocked {
		t.Fatalf("write command should be blocked by Preview try bash: %v", err)
	}
}

func TestSubagentTryEndpointRejectsMissingPromptBeforeStarting(t *testing.T) {
	server := &bridgeServer{}
	request := httptest.NewRequest("POST", "/v1/settings/subagents/try", strings.NewReader(`{"workspaceRoot":"","input":{},"task":"inspect"}`))
	response := httptest.NewRecorder()
	server.trySubagentProfile(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "system prompt") {
		t.Fatalf("response = %d %s, want missing-prompt validation", response.Code, response.Body.String())
	}
	if server.subagentTryRun != nil {
		t.Fatal("invalid try request should not start a run")
	}
}
