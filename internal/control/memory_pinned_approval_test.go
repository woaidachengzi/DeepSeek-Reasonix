package control

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/memory"
)

func TestPinnedProjectMemoryRequiresManualDecision(t *testing.T) {
	for _, approve := range []bool{false, true} {
		t.Run(map[bool]string{false: "decline", true: "allow"}[approve], func(t *testing.T) {
			store := memory.Store{Dir: t.TempDir()}
			requests := make(chan event.Approval, 1)
			c := New(Options{Memory: &memory.Set{Store: store}, Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.ApprovalRequest {
					requests <- e.Approval
				}
			})})
			args := json.RawMessage(`{"name":"release-target","description":"Project release target","type":"project","activation":"pinned","body":"Release from main-v2."}`)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			type result struct {
				allow bool
				err   error
			}
			done := make(chan result, 1)
			go func() {
				allow, _, err := gateApprover{c}.Approve(ctx, memoryRememberTool, "", args)
				done <- result{allow, err}
			}()
			var request event.Approval
			select {
			case request = <-requests:
			case got := <-done:
				t.Fatalf("pinned memory bypassed human confirmation: %+v", got)
			case <-time.After(5 * time.Second):
				t.Fatal("pinned memory did not request confirmation")
			}
			if c.ClaimAutoMemoryWrite(args) || len(store.List()) != 0 {
				t.Fatal("unapproved pinned memory obtained an automatic write claim")
			}
			c.Approve(request.ID, approve, false, false)
			select {
			case got := <-done:
				if got.err != nil || got.allow != approve {
					t.Fatalf("manual decision: %+v", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("manual decision did not finish")
			}
			if approve {
				if _, err := memory.NewRememberTool(store).Execute(memory.WithQueue(ctx, c), args); err != nil {
					t.Fatal(err)
				}
				if got, ok := store.Read("release-target"); !ok || got.Activation != memory.ActivationPinned {
					t.Fatalf("approved pinned memory not preserved: %+v", got)
				}
			} else if len(store.List()) != 0 {
				t.Fatal("declined memory was persisted")
			}
		})
	}
}

func TestHeadlessModesCannotAutoApprovePinnedMemory(t *testing.T) {
	for _, mode := range []string{ToolApprovalAsk, ToolApprovalAuto, ToolApprovalYolo} {
		t.Run(mode, func(t *testing.T) {
			c := New(Options{Memory: &memory.Set{Store: memory.Store{Dir: t.TempDir()}}})
			args := json.RawMessage(`{"name":"release-target","description":"Project release target","type":"reference","activation":"pinned","body":"Release from main-v2."}`)
			allow, _, err := c.newHeadlessGate(mode).Check(t.Context(), memoryRememberTool, args, false)
			if err != nil || allow || c.ClaimAutoMemoryWrite(args) {
				t.Fatalf("headless %s allowed pinned memory: allow=%v err=%v", mode, allow, err)
			}
		})
	}
}
