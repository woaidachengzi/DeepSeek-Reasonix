package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/tool"
)

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
	sink.Emit(event.Event{Kind: event.Text, Text: "discarded partial"})
	sink.Emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{Action: event.StreamAttemptDiscard}})
	sink.Emit(event.Event{Kind: event.Text, Text: "Visible "})
	sink.Emit(event.Event{Kind: event.Text, Text: "answer"})
	sink.Emit(event.Event{Kind: event.Message, Text: "Visible answer"})
	if got := run.snapshot(); got != "Visible answer" {
		t.Fatalf("snapshot = %q, want visible answer only", got)
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
