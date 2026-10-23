package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/tool"
)

type bridgeConsentProbe struct{ executed atomic.Bool }

func (*bridgeConsentProbe) Name() string { return "fixture_computer_use" }
func (*bridgeConsentProbe) Description() string {
	return "Test app consent without native observation or input"
}
func (*bridgeConsentProbe) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*bridgeConsentProbe) ReadOnly() bool          { return false }
func (p *bridgeConsentProbe) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	approver := tool.ComputerUseApproverFrom(ctx)
	if approver == nil {
		return "", errors.New("missing app consent host")
	}
	allow, err := approver.ApproveComputerUse(ctx, tool.ComputerUseTarget{PID: 42, Name: "Fixture", BundleID: "fixture.app", LaunchTime: 1})
	if err != nil || !allow {
		return "not approved", err
	}
	p.executed.Store(true)
	return "fixture approved; no native input sent", nil
}

// Exercise the actual cold-open factory, not a controller with manually wired
// services. No native app is observed, activated, or given keyboard input.
func TestColdBridgeOpenRoutesAppConsentEvenInYolo(t *testing.T) {
	terminalLeaseTestProfile(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"consent-probe\",\"type\":\"function\",\"function\":{\"name\":\"fixture_computer_use\",\"arguments\":\"{}\"}}]}}]}\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	config := fmt.Sprintf("default_model = \"fixture/ordinary\"\n[[providers]]\nname = \"fixture\"\nkind = \"openai\"\nbase_url = %q\nmodels = [\"ordinary\"]\nno_proxy = true\n", server.URL+"/v1")
	if err := os.WriteFile(filepath.Join(os.Getenv("REASONIX_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	approvals, done := make(chan event.Event, 4), make(chan struct{}, 4)
	factory := newControllerFactory(nil)
	factory.base.Sink = event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			approvals <- e
		}
		if e.Kind == event.TurnDone {
			done <- struct{}{}
		}
	})
	raw, err := factory.Open(t.Context(), desktopbridge.OpenRequest{SessionID: "cold-consent", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	runtime := raw.(*controllerRuntime)
	t.Cleanup(func() { _ = runtime.Shutdown() })
	probe := &bridgeConsentProbe{}
	registry := tool.NewRegistry()
	registry.Add(probe)
	runtime.controller.Executor().SetTools(registry)
	runtime.controller.SetToolApprovalMode("yolo")
	runtime.Submit("hello")
	select {
	case e := <-approvals:
		if e.Approval.Tool != "computer_use_access" || probe.executed.Load() {
			t.Fatalf("wrong or pre-authorized app consent: %+v", e.Approval)
		}
		runtime.Approve(e.Approval.ID, true)
	case <-done:
		t.Fatal("cold-open task finished without requesting app consent")
	case <-time.After(10 * time.Second):
		t.Fatal("cold-open app consent was not routed")
	}
	select {
	case <-done:
		if !probe.executed.Load() {
			t.Fatal("approved fixture did not execute")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("task did not finish after app approval")
	}
}
