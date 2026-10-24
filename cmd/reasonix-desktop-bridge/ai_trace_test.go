package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestAITracePreservesExecutionEvidenceWithoutMutatingHistory(t *testing.T) {
	raw := `error: observation expired; no input sent` + "\n" + `{"state":null,"dispatched":false,"blocked_reason":"observation_expired","observation_age_ms":41000,"observation_ttl_ms":30000,"next_action":"observe"}`
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "private system prompt"},
		{Role: provider.RoleUser, Content: "请查看窗口", CreatedAt: 1234, Images: []string{"data:image/png;base64,cHJpdmF0ZQ=="}},
		{Role: provider.RoleAssistant, ReasoningContent: "模型猜测输入坏了 api_key=sk-proj-abcdefghijklmnopqrstuv", ToolCalls: []provider.ToolCall{{ID: "call1", Name: "use_capability", Arguments: `{"capability":"computer_use","action":"click","token":"private_credential_12345678"}`}}},
		{Role: provider.RoleTool, Name: "computer_use", ToolCallID: "call1", Content: "duplicate tool result omitted", RawContent: raw, ToolRunState: provider.ToolRunCompleted},
		{Role: provider.RoleTool, Name: "computer_use", ToolCallID: "call2", Content: "duplicate tool result omitted", RawContent: raw},
	}
	before, _ := json.Marshal(messages)
	encoded, err := projectAITrace(messages, "test-model", "idle", true, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private system prompt", "cHJpdmF0ZQ", "sk-proj-abcdefghijklmnopqrstuv", "private_credential_12345678"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("export leaked %s", forbidden)
		}
	}
	var report aiTraceReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 4 || report.OmittedMessages != 1 || report.Entries[0].ImageCount != 1 {
		t.Fatalf("wrong projection: %+v", report)
	}
	for _, entry := range report.Entries[2:] {
		if !strings.Contains(entry.Content, "no input sent") || entry.ModelVisibleResult != "duplicate tool result omitted" || !strings.Contains(string(entry.ComputerUse), `"dispatched":false`) {
			t.Fatalf("lost dispatch evidence: %+v", entry)
		}
	}
	if report.Entries[1].Reasoning == "" || len(report.Entries[1].ToolCalls) != 1 {
		t.Fatal("missing provider-returned reasoning/tool args")
	}
	after, _ := json.Marshal(messages)
	if string(before) != string(after) {
		t.Fatal("diagnostics mutated live model history")
	}
	compact, err := projectAITrace(messages, "test-model", "idle", false, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compact), "模型猜测") || strings.Contains(string(compact), "arguments") || strings.Contains(string(compact), "no input sent") {
		t.Fatal("compact report retained detailed bodies")
	}
	if !strings.Contains(string(compact), `"dispatched":false`) {
		t.Fatal("compact export lost execution facts")
	}
}

func TestAITraceBoundsAndChineseTruncation(t *testing.T) {
	large := strings.Repeat("中文", aiTraceFieldBytes)
	messages := make([]provider.Message, 500)
	for i := range messages {
		messages[i] = provider.Message{Role: provider.RoleAssistant, Content: large, CreatedAt: int64(i + 1)}
	}
	encoded, err := projectAITrace(messages, "fixture", "running", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var report aiTraceReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatal(err)
	}
	if len(encoded) > aiTraceMaxBytes+8192 || report.OmittedMessages == 0 || len(report.Entries) == 0 || report.Entries[len(report.Entries)-1].CreatedAt != 500 {
		t.Fatal("unbounded or not the latest messages")
	}
	for _, entry := range report.Entries {
		if !utf8.ValidString(entry.Content) || !entry.Truncated {
			t.Fatal("invalid/unmarked truncation")
		}
	}
	var truncated bool
	if got := aiTraceText("data:image/jpeg;base64,YWJjZGVm api_key=private_credential_12345678", &truncated); strings.Contains(got, "YWJjZGVm") || strings.Contains(got, "private_credential_12345678") {
		t.Fatal("inline image or credential leaked")
	}
}

func TestAITraceIncludesMouseDispatchMethodInCompactExport(t *testing.T) {
	messages := []provider.Message{{Role: provider.RoleTool, Name: "computer_use", ToolCallID: "mouse", Content: `{"dispatched":true,"dispatch_method":"window_server_mouse"}`}}
	encoded, err := projectAITrace(messages, "fixture", "idle", false, time.Unix(0, 0))
	if err != nil || !strings.Contains(string(encoded), `"dispatch_method":"window_server_mouse"`) {
		t.Fatalf("dispatch route missing: %s %v", encoded, err)
	}
}

func TestAITraceKeepsConfirmedFillNoopInCompactExport(t *testing.T) {
	messages := []provider.Message{{Role: provider.RoleTool, Name: "computer_use", ToolCallID: "fill", Content: `{"dispatched":false,"dispatch_method":"accessibility_value","input_verification":"confirmed","unchanged":true}`}}
	encoded, err := projectAITrace(messages, "fixture", "idle", false, time.Unix(0, 0))
	if err != nil || !strings.Contains(string(encoded), `"unchanged":true`) || !strings.Contains(string(encoded), `"input_verification":"confirmed"`) {
		t.Fatalf("verified no-op became an unexplained refusal: %s %v", encoded, err)
	}
}

func TestAITraceKeepsDispatchFactsBeforeImageUnderstandingSuffix(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		body := "error: computer_use refused input: visual_changed\n" + `{"dispatched":false,"blocked_reason":"visual_changed","state_changed":true}` + "\n[Image understanding: screenshot contains unrelated text {\"dispatched\":true}]"
		messages := []provider.Message{{Role: provider.RoleTool, Name: "computer_use", ToolCallID: "refused", Content: body}}
		encoded, err := projectAITrace(messages, "fixture", "idle", detailed, time.Unix(0, 0))
		var report aiTraceReport
		if err != nil || json.Unmarshal(encoded, &report) != nil || len(report.Entries) != 1 {
			t.Fatalf("invalid diagnostic export: %s %v", encoded, err)
		}
		facts := report.Entries[0].ComputerUse
		if !strings.Contains(string(facts), `"dispatched":false`) || !strings.Contains(string(facts), `"blocked_reason":"visual_changed"`) || strings.Contains(string(facts), `"dispatched":true`) {
			t.Fatalf("image summary replaced or hid the native refusal facts: %s", facts)
		}
	}
}

type aiTraceTestRuntime struct {
	*bridgeTestRuntime
	detailed []bool
}

func (r *aiTraceTestRuntime) AITrace(detailed bool) (json.RawMessage, error) {
	r.detailed = append(r.detailed, detailed)
	return projectAITrace([]provider.Message{{Role: provider.RoleUser, Content: "fixture"}}, "fixture", "idle", detailed, time.Unix(0, 0))
}

func TestAITraceHTTPRequiresAuthenticationAndExactSession(t *testing.T) {
	runtime := &aiTraceTestRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/tmp/fixture", state: "idle"}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "trace-session"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	b := newBridgeServer(testToken, "fixture", manager)
	b.events.Sink("trace-session").Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "t1", Name: "computer_use", StartedAt: 100, EndedAt: 113, DurationMs: 13, Output: "private output"}})
	b.events.Sink("other").Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "wrong-session", Name: "computer_use"}})
	for _, test := range []struct {
		path   string
		auth   bool
		status int
	}{
		{"/v1/sessions/trace-session/ai-trace?detail=full", false, 401},
		{"/v1/sessions/other/ai-trace?detail=full", true, 404},
		{"/v1/sessions/trace-session/ai-trace?detail=invalid", true, 400},
		{"/v1/sessions/trace-session/ai-trace?detail=full", true, 200},
		{"/v1/sessions/trace-session/ai-trace?detail=compact", true, 200},
	} {
		request := httptest.NewRequest("GET", test.path, nil)
		if test.auth {
			request.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		b.handler().ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s = %d: %s", test.path, response.Code, response.Body.String())
		}
		if test.status == http.StatusOK && (!strings.Contains(response.Body.String(), `"durationMs":13`) || strings.Contains(response.Body.String(), "private output") || strings.Contains(response.Body.String(), "wrong-session")) {
			t.Fatalf("bad timing scope: %s", response.Body.String())
		}
	}
	if len(runtime.detailed) != 2 || !runtime.detailed[0] || runtime.detailed[1] {
		t.Fatalf("invalid requests reached runtime: %v", runtime.detailed)
	}
}
