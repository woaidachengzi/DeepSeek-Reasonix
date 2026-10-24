package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/computeruse"
	"reasonix/internal/tool"
)

func TestCuaAccessCannotStartBeforeHostConsent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx = tool.WithComputerUseLifetime(ctx, ctx)
	c := computerUse{controller: &desktopConsentFixture{}, cua: &computeruse.CuaRuntime{}}
	request := json.RawMessage(`{"action":"observe","pid":42,"window_id":11}`)
	if _, _, err := c.ExecuteWithImages(ctx, request); err == nil {
		t.Fatal("missing approver reached Cua")
	}
	ctx = tool.WithComputerUseApprover(ctx, desktopConsentFunc(func(context.Context, tool.ComputerUseTarget) (bool, error) { return false, nil }))
	if _, _, err := c.ExecuteWithImages(ctx, request); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("declined app reached runtime: %v", err)
	}
	for _, request := range []string{`{"action":"observe","pid":42,"window_id":11,"session":"other"}`, `{"action":"shell_execute","pid":42,"window_id":11}`} {
		if _, _, err := c.ExecuteWithImages(ctx, json.RawMessage(request)); err == nil {
			t.Fatal("untrusted driver operation accepted")
		}
	}
	c.confined = true
	if _, _, err := c.ExecuteWithImages(ctx, json.RawMessage(`{"action":"apps"}`)); err == nil {
		t.Fatal("Cua escaped the workspace sandbox")
	}
}

func TestCuaEffectsDoNotInventDispatchOrTaskSuccess(t *testing.T) {
	c := computerUse{}
	for _, tc := range []struct {
		action, completion  string
		dispatched, unknown bool
	}{
		{`{"effect":"refused","error":{"code":"stale_element_token"}}`, "", false, false},
		{`{"effect":"unverifiable","route":"synthetic_events","delivery":{"delivered_count":1}}`, "", true, false},
		{`{"effect":"unverifiable"}`, "", false, true},
		{`{"effect":"confirmed","evidence":[{"kind":"window_change"}]}`, "", false, true},
		{`null`, "not_started", false, false},
	} {
		reply := computeruse.CuaReply{Action: json.RawMessage(tc.action), Result: json.RawMessage(`{}`), Completion: tc.completion}
		body, _, err := c.cuaResult(t.Context(), computeruse.CuaRequest{Action: "click"}, nil, reply)
		var facts map[string]any
		if err != nil || json.Unmarshal([]byte(body), &facts) != nil || facts["dispatched"] != tc.dispatched || (facts["dispatch_unknown"] == true) != tc.unknown {
			t.Fatalf("wrong Cua evidence: %s %v", body, err)
		}
		if facts["task_success"] != nil {
			t.Fatal("Cua field/window verification became task success")
		}
	}
}

func TestCuaObservationDoesNotDuplicateTreeInSummary(t *testing.T) {
	state := json.RawMessage(`{"window_id":11,"elements":[{"label":"private fixture tree"}]}`)
	body, _, err := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "observe"}, nil, computeruse.CuaReply{Result: state, Text: "private fixture tree"})
	if err != nil || strings.Count(body, "private fixture tree") != 1 {
		t.Fatalf("tree was repeated outside supersedable state: %s %v", body, err)
	}
}

func TestCuaSDKRefusalIsDistinctFromUnknownCompletion(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		reply := computeruse.CuaReply{IsError: true, ErrorCode: "invalid_arguments", Result: json.RawMessage(`{"status":"refused","refusal":{"code":"invalid_arguments"}}`)}
		if unknown {
			reply.Completion = "unknown"
		}
		body, _, err := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "click"}, nil, reply)
		var facts map[string]any
		if err == nil || json.Unmarshal([]byte(body), &facts) != nil || (facts["dispatch_unknown"] == true) != unknown {
			t.Fatalf("refusal and uncertain dispatch were confused: %s %v", body, err)
		}
		if !unknown && facts["blocked_reason"] != "invalid_arguments" {
			t.Fatal("SDK pre-dispatch refusal lost its exact reason")
		}
	}
}
