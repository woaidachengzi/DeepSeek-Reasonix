package computeruse

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCuaBrowserArgumentsAndHostOnlyApproval(t *testing.T) {
	for _, raw := range []string{
		`{"action":"browser_observe","pid":42,"window_id":11}`,
		`{"action":"browser_observe","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","query":"收件人","include_screenshot":true}`,
		`{"action":"browser_click","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1"}`,
		`{"action":"browser_type","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1","replace":true}`,
	} {
		var r CuaRequest
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("valid browser request: %s: %v", raw, err)
		}
	}
	for _, r := range []CuaRequest{
		{Action: "browser_click", PID: 42, WindowID: 11},
		{Action: "browser_observe", PID: 42, WindowID: 11, TargetID: "other"},
		{Action: "browser_prepare", PID: 42, WindowID: 11, TargetID: "b1", TabID: "t1"},
		{Action: "browser_observe", PID: 42, WindowID: 11, ElementToken: "native"},
		{Action: "browser_type", PID: 42, WindowID: 11, TargetID: "b1", TabID: "t1", Ref: "p1:e1"},
		{Action: "observe", PID: 42, WindowID: 11, TargetID: "b1", TabID: "t1"},
	} {
		if r.Validate() == nil {
			t.Fatalf("unsafe browser request: %+v", r)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runtime := cuaFixture("echo")
	if _, err := runtime.Call(ctx, ctx, CuaRequest{Action: "browser_prepare", PID: 42, WindowID: 11}, nil); err == nil || runtime.process != nil {
		t.Fatal("unapproved profile attachment started a worker", err)
	}
}
