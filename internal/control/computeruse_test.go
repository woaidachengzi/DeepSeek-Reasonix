package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

func TestComputerUseConsentScopedAndNeverYolo(t *testing.T) {
	ids := make(chan string, 8)
	finished := make(chan struct{}, 1)
	runner := &turnEventGateRunner{started: make(chan struct{}), release: make(chan struct{})}
	dir := t.TempDir()
	c := New(Options{Runner: runner, SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			ids <- e.Approval.ID
		}
		if e.Kind == event.TurnDone {
			finished <- struct{}{}
		}
	})})
	t.Cleanup(c.Close)
	a := &computerUseApprover{c: c}
	target := tool.ComputerUseTarget{PID: 42, Name: "Example", BundleID: "example.app", LaunchTime: 100}
	defer func() {
		close(runner.release)
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("task did not finish")
		}
		if allow, err := a.ApproveComputerUse(t.Context(), target); allow || err == nil {
			t.Error("ended task retained app access")
		}
	}()
	c.SetToolApprovalMode(ToolApprovalYolo)
	c.Submit("test")
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start")
	}
	type result struct {
		allow bool
		err   error
	}
	run := func(ctx context.Context, target tool.ComputerUseTarget) chan result {
		done := make(chan result, 1)
		go func() { allow, err := a.ApproveComputerUse(ctx, target); done <- result{allow, err} }()
		return done
	}
	done := run(t.Context(), target)
	id := waitApprovalID(t, ids)
	select {
	case <-done:
		t.Fatal("Yolo answered app consent")
	default:
	}
	c.Approve(id, true, true, true) // session/persistent reply must remain app/task-local.
	if r := <-done; !r.allow || r.err != nil {
		t.Fatalf("allow: %+v", r)
	}
	if r := <-run(t.Context(), target); !r.allow || r.err != nil {
		t.Fatalf("cached: %+v", r)
	}
	// Native app access must not answer the broader logged-in profile grant.
	browserDone := make(chan result, 1)
	go func() {
		allow, err := a.ApproveBrowserProfile(t.Context(), target, 11)
		browserDone <- result{allow, err}
	}()
	id = waitApprovalID(t, ids)
	c.Approve(id, true, true, true)
	if r := <-browserDone; !r.allow || r.err != nil {
		t.Fatalf("browser grant: %+v", r)
	}
	if allow, err := a.ApproveBrowserProfile(t.Context(), target, 11); !allow || err != nil {
		t.Fatal("same browser window did not reuse task grant", err)
	}
	go func() {
		allow, err := a.ApproveBrowserProfile(t.Context(), target, 12)
		browserDone <- result{allow, err}
	}()
	id = waitApprovalID(t, ids)
	c.Approve(id, false, false, false)
	if r := <-browserDone; r.allow || r.err != nil {
		t.Fatalf("browser grant escaped its native window: %+v", r)
	}
	if !RequiresFreshHumanApprovalTool(BrowserProfileApprovalTool) || allowsFreshSessionGrantTool(BrowserProfileApprovalTool) {
		t.Fatal("browser profile permits a generic remembered grant")
	}
	c.approval.grantSession(BrowserProfileApprovalTool, "*")
	if c.approval.preApprovedForDecision(BrowserProfileApprovalTool, "another", nil, true) {
		t.Fatal("generic grant bypassed browser profile approval")
	}

	target.PID = 43
	done = run(t.Context(), target)
	id = waitApprovalID(t, ids)
	c.Approve(id, false, false, false)
	if r := <-done; r.allow || r.err != nil {
		t.Fatalf("denial: %+v", r)
	}
	if r := <-run(t.Context(), target); r.allow || r.err != nil {
		t.Fatalf("cached denial: %+v", r)
	}
	target.PID = 42
	target.LaunchTime = 200
	ctx, cancel := context.WithCancel(t.Context())
	done = run(ctx, target)
	id = waitApprovalID(t, ids)
	cancel()
	c.Approve(id, true, false, false)
	if r := <-done; r.allow || r.err == nil {
		t.Fatalf("late allow survived cancellation: %+v", r)
	}
	if !RequiresFreshHumanApprovalTool(ComputerUseAccessApprovalTool) || allowsFreshSessionGrantTool(ComputerUseAccessApprovalTool) {
		t.Fatal("wrong fresh approval classification")
	}
	c.approval.grantSession(ComputerUseAccessApprovalTool, "*")
	if c.approval.preApprovedForDecision(ComputerUseAccessApprovalTool, "another", nil, true) {
		t.Fatal("generic grant bypassed app consent")
	}
}
