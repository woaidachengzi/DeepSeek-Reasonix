package builtin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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

func TestCuaAppGrantDoesNotAuthorizeBrowserAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx = tool.WithComputerUseLifetime(ctx, ctx)
	ctx = tool.WithComputerUseApprover(ctx, desktopConsentFunc(func(context.Context, tool.ComputerUseTarget) (bool, error) { return true, nil }))
	c := computerUse{controller: &desktopConsentFixture{}, cua: &computeruse.CuaRuntime{}}
	_, _, err := c.ExecuteWithImages(ctx, json.RawMessage(`{"action":"browser_prepare","pid":42,"window_id":11}`))
	if err == nil || !strings.Contains(err.Error(), "separate interactive profile approval") {
		t.Fatalf("app grant authorized profile attachment: %v", err)
	}
}

func TestCuaBrowserDispatchEvidenceAndPartialInput(t *testing.T) {
	for _, tc := range []struct {
		result, action              string
		failed, unknown, dispatched bool
	}{
		{`{}`, `{"effect":"unverifiable","route":"trusted_input","delivery":{"mode":"background"}}`, false, false, true},
		{`{}`, `{"effect":"confirmed","route":"trusted_input"}`, false, true, false},
		{`{"status":"refused","refusal":{"code":"browser_ref_stale"}}`, `null`, true, false, false},
		{`{"status":"refused","refusal":{"code":"browser_input_incomplete","detail":{"delivered_chars":3}}}`, `null`, true, true, false},
		{`{"status":"refused","refusal":{"code":"browser_input_trust_unavailable","detail":{"delivery":"unknown"}}}`, `null`, true, true, false},
	} {
		reply := computeruse.CuaReply{Result: json.RawMessage(tc.result), Action: json.RawMessage(tc.action), IsError: tc.failed}
		body, _, _ := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "browser_click"}, nil, reply)
		var facts map[string]any
		if json.Unmarshal([]byte(body), &facts) != nil || (facts["dispatch_unknown"] == true) != tc.unknown || facts["dispatched"] != tc.dispatched {
			t.Fatalf("wrong browser evidence: %s", body)
		}
		if tc.unknown && facts["blocked_reason"] != nil {
			t.Fatalf("partly delivered input became a safe refusal: %s", body)
		}
	}
	body, _, _ := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "browser_observe", PID: 42, WindowID: 11}, nil, computeruse.CuaReply{Result: json.RawMessage(`{"mode":"snapshot","outline":"private DOM"}`), Text: "private DOM"})
	if strings.Count(body, "private DOM") != 1 || !strings.Contains(body, `"window_id":11`) {
		t.Fatalf("unbounded browser observation: %s", body)
	}
}

// The SDK projects legacy browser refusals into ActionResult, removing the
// original refusal/detail fields. A failed mouse release or focus cleanup can
// therefore look like an ordinary refusal even after an input was accepted.
func TestCuaBrowserProjectedFailuresRemainUncertain(t *testing.T) {
	for _, tc := range []struct {
		action     string
		dispatched bool
	}{
		{`{"effect":"refused","route":"trusted_input","error":{"code":"browser_input_trust_unavailable"}}`, false},
		{`{"effect":"partial","route":"trusted_input","delivery":{"mode":"background","delivered_count":3},"error":{"code":"browser_input_incomplete"}}`, true},
	} {
		raw := json.RawMessage(tc.action)
		reply := computeruse.CuaReply{IsError: true, Action: raw, Result: raw}
		body, _, _ := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "browser_click"}, nil, reply)
		var facts map[string]any
		if json.Unmarshal([]byte(body), &facts) != nil || facts["dispatch_unknown"] != true || facts["dispatched"] != tc.dispatched || facts["blocked_reason"] != nil {
			t.Fatalf("projected browser failure lost uncertainty: %s", body)
		}
	}
	raw := json.RawMessage(`{"status":"refused","refusal":{"code":"browser_input_trust_unavailable"}}`)
	body, _, _ := (computerUse{}).cuaResult(t.Context(), computeruse.CuaRequest{Action: "browser_click"}, nil, computeruse.CuaReply{IsError: true, Result: raw})
	if !strings.Contains(body, `"dispatch_unknown":true`) {
		t.Fatalf("failed mouse release became a safe refusal: %s", body)
	}
}

func TestCuaBrowserFollowUpsUsePublicSchemaAndNoPrivateSession(t *testing.T) {
	request := computeruse.CuaRequest{Action: "browser_type", PID: 42, WindowID: 11, TargetID: "bt-live", TabID: "tab-live"}
	raw := json.RawMessage(`{"effect":"unverifiable","next_call":{"tool":"get_browser_state","arguments":{"session":"reasonix_browser","target_id":"foreign","tab_id":"foreign","script":"ignored"}}}`)
	got := cuaBrowserHints(raw, request)
	if strings.Contains(string(got), "session") || strings.Contains(string(got), "foreign") || strings.Contains(string(got), "script") {
		t.Fatalf("unusable SDK hint reached model: %s", got)
	}
	var result struct {
		Next struct {
			Tool      string                 `json:"tool"`
			Arguments computeruse.CuaRequest `json:"arguments"`
		} `json:"next_call"`
	}
	if json.Unmarshal(got, &result) != nil || result.Next.Tool != "computer_use" || result.Next.Arguments.Action != "browser_observe" || result.Next.Arguments.Validate() != nil {
		t.Fatalf("invalid follow-up: %s", got)
	}
	prep := cuaBrowserHints(json.RawMessage(`{"status":"refused","refusal":{"detail":{"next_call":{"tool":"browser_prepare","arguments":{"session":"private","strategy":{"kind":"existing_profile"}}}}}}`), request)
	if strings.Contains(string(prep), "session") || strings.Contains(string(prep), "strategy") || !strings.Contains(string(prep), `"action":"browser_prepare"`) {
		t.Fatalf("unusable setup hint: %s", prep)
	}
}

func TestCuaBrowserScreenshotUsesExistingPreviewActionContract(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 640, 400))); err != nil {
		t.Fatal(err)
	}
	var preview tool.ComputerUsePreview
	ctx := tool.WithComputerUsePreview(t.Context(), func(p tool.ComputerUsePreview) { preview = p })
	reply := computeruse.CuaReply{Result: json.RawMessage(`{"mode":"snapshot"}`), Images: []computeruse.CuaImage{{MIME: "image/png", Data: base64.StdEncoding.EncodeToString(pngData.Bytes())}}}
	_, images, err := (computerUse{}).cuaResult(ctx, computeruse.CuaRequest{Action: "browser_observe", PID: 42, WindowID: 11}, &computeruse.Target{PID: 42, Name: "Fixture"}, reply)
	if err != nil || len(images) != 1 || preview.Action != "observe" || preview.WindowID != 11 || !strings.HasPrefix(preview.Image, "data:image/jpeg;base64,") {
		t.Fatalf("browser screenshot did not reach the existing PiP contract: %+v %v", preview, err)
	}
}
