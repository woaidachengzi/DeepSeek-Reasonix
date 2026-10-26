package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/computeruse"
	"reasonix/internal/tool"
)

type foregroundConsentFixture struct {
	allow bool
	calls int
	after func()
}

func (*foregroundConsentFixture) ApproveComputerUse(context.Context, tool.ComputerUseTarget) (bool, error) {
	return true, nil
}
func (f *foregroundConsentFixture) ApproveComputerUseForeground(_ context.Context, target tool.ComputerUseTarget, window int, action string) (bool, error) {
	f.calls++
	if target.PID != 42 || window != 11 || action != "key" {
		panic("foreground prompt lost exact target/action")
	}
	if f.after != nil {
		f.after()
	}
	return f.allow, nil
}

func TestCuaForegroundCannotUseAppConsentOrLateApproval(t *testing.T) {
	request := json.RawMessage(`{"action":"key","pid":42,"window_id":11,"key":"tab","delivery_mode":"foreground"}`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx = tool.WithComputerUseLifetime(ctx, ctx)
	c := computerUse{controller: &desktopConsentFixture{}, cua: &computeruse.CuaRuntime{}}
	appOnly := tool.WithComputerUseApprover(ctx, desktopConsentFunc(func(context.Context, tool.ComputerUseTarget) (bool, error) { return true, nil }))
	if _, _, err := c.ExecuteWithImages(appOnly, request); err == nil || !strings.Contains(err.Error(), "single-action confirmation") {
		t.Fatalf("app grant bypassed foreground gate: %v", err)
	}
	a := &foregroundConsentFixture{}
	declined := tool.WithComputerUseApprover(ctx, a)
	if _, _, err := c.ExecuteWithImages(declined, request); err == nil || !strings.Contains(err.Error(), "kept this window in background") || a.calls != 1 {
		t.Fatalf("denied foreground action reached worker: %v", err)
	}
	a.allow, a.after = true, cancel
	if _, _, err := c.ExecuteWithImages(declined, request); err == nil || !strings.Contains(err.Error(), "changed before Cua dispatch") {
		t.Fatalf("late foreground approval survived task stop: %v", err)
	}
}

func TestCuaModelCannotForgeForegroundHostApproval(t *testing.T) {
	c := computerUse{cua: &computeruse.CuaRuntime{}}
	_, _, err := c.ExecuteWithImages(t.Context(), json.RawMessage(`{"action":"key","pid":42,"window_id":11,"key":"tab","foreground_approved":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("model supplied private host approval: %v", err)
	}
}
