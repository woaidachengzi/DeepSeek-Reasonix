package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/computeruse"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

type desktopConsentFixture struct{ captures int }

func (f *desktopConsentFixture) Target(context.Context, int) (computeruse.Target, error) {
	return computeruse.Target{PID: 42, Name: "Native name", BundleID: "example.app", LaunchTime: 100}, nil
}
func (f *desktopConsentFixture) Execute(context.Context, computeruse.Request) (computeruse.Result, error) {
	return computeruse.Result{}, nil
}
func (f *desktopConsentFixture) ExecuteForTarget(ctx context.Context, _ computeruse.Request, _ computeruse.Target) (computeruse.Result, error) {
	if ctx.Err() != nil {
		return computeruse.Result{}, ctx.Err()
	}
	f.captures++
	return computeruse.Result{State: json.RawMessage(`{}`)}, nil
}

type desktopConsentFunc func(context.Context, tool.ComputerUseTarget) (bool, error)

func (f desktopConsentFunc) ApproveComputerUse(ctx context.Context, t tool.ComputerUseTarget) (bool, error) {
	return f(ctx, t)
}

func TestComputerUseRequiresHumanBeforeCapture(t *testing.T) {
	for _, allow := range []bool{false, true} {
		f := &desktopConsentFixture{}
		c := computerUse{controller: f}
		args := json.RawMessage(`{"action":"observe","pid":42}`)
		if _, _, err := c.ExecuteWithImages(t.Context(), args); err == nil || f.captures != 0 {
			t.Fatal("missing approver captured screen")
		} else if _, blocked := tool.BlockedMessage(err); !blocked {
			t.Fatal("missing host must be a refusal, not a failed input attempt")
		}
		ctx := tool.WithComputerUseApprover(t.Context(), desktopConsentFunc(func(_ context.Context, target tool.ComputerUseTarget) (bool, error) {
			if target.Name != "Native name" || f.captures != 0 {
				t.Fatal("prompt did not precede capture with native identity")
			}
			return allow, nil
		}))
		_, _, err := c.ExecuteWithImages(ctx, args)
		if allow && (err != nil || f.captures != 1) {
			t.Fatalf("approved capture failed: %v", err)
		}
		if !allow && (err == nil || f.captures != 0) {
			t.Fatal("denial captured screen")
		}
	}
}

func TestComputerUseHonorsWorkspaceSandbox(t *testing.T) {
	c := newComputerUse(true)
	if c.ReadOnly() || c.ProviderVisible(t.Context()) {
		t.Fatal("desktop tool escaped sandbox/read-only classification")
	}
	if _, _, err := c.ExecuteWithImages(t.Context(), json.RawMessage(`{"action":"apps"}`)); err == nil {
		t.Fatal("sandboxed direct execution succeeded")
	}
	if computeruse.Available() {
		for _, candidate := range (Workspace{Dir: t.TempDir(), Bash: sandbox.Spec{Mode: "enforce"}}).Tools("computer_use") {
			bound, ok := candidate.(computerUse)
			if !ok || bound.ProviderVisible(t.Context()) {
				t.Fatal("workspace did not bind the desktop sandbox boundary")
			}
		}
		var found bool
		for _, candidate := range (Workspace{Dir: t.TempDir()}).Tools("computer_use") {
			found = true
			if _, ok := candidate.(tool.ImageTool); !ok {
				t.Fatal("computer tool lost screenshot image channel")
			}
		}
		if !found {
			t.Fatal("native desktop tool not registered")
		}
	}
}

func TestComputerUseErrorsNeverRetry(t *testing.T) {
	c := newComputerUse(false)
	_, _, err := c.ExecuteWithImages(t.Context(), json.RawMessage(`{"action":"click","pid":1}`))
	classified, ok := err.(interface{ RetryableToolError() bool })
	if !ok || classified.RetryableToolError() {
		t.Fatal("desktop errors must opt out of automatic tool replay")
	}
}
