package computeruse

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Shared with the Rust worker: both ends must accept/refuse the same model wire
// requests, including an explicitly empty option value and gesture bounds.
func TestCuaBrowserOperationWireContract(t *testing.T) {
	data, err := os.ReadFile("testdata/cua_browser_operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string     `json:"name"`
		Valid   bool       `json:"valid"`
		Request CuaRequest `json:"request"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.Request.Validate(); (err == nil) != tc.Valid {
				t.Fatalf("validation = %v, want valid %v", err, tc.Valid)
			}
			if tc.Valid {
				wire, err := json.Marshal(tc.Request)
				if err != nil {
					t.Fatal(err)
				}
				var decoded CuaRequest
				if err := json.Unmarshal(wire, &decoded); err != nil || decoded.Validate() != nil {
					t.Fatalf("request lost during host serialization: %s %v", wire, err)
				}
				if tc.Request.Value != nil && *tc.Request.Value == "" && (decoded.Value == nil || *decoded.Value != "") {
					t.Fatal("explicit empty option value lost")
				}
			}
		})
	}
	for _, n := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		r := CuaRequest{Action: "browser_pointer", PID: 42, WindowID: 11, TargetID: "b1", TabID: "t1", Ref: "p1:e1", Gesture: "scroll", DeltaY: &n}
		if r.Validate() == nil {
			t.Fatal("nonfinite scroll accepted")
		}
	}
}

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
