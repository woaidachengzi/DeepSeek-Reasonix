package computeruse

import (
	"context"
	"errors"
	"testing"
)

type nativeOutcomeDesktop struct {
	*fakeDesktop
	result nativeResult
	err    error
}

func (f *nativeOutcomeDesktop) call(ctx context.Context, r Request) (nativeResult, error) {
	if r.Action == "observe" {
		return f.fakeDesktop.call(ctx, r)
	}
	f.calls = append(f.calls, r)
	return f.result, f.err
}

func TestComputerUseDispatchFactsSurviveNativeOutcome(t *testing.T) {
	for _, kind := range []string{"success", "capture_failed", "native_transport_error", "pointer_refused"} {
		t.Run(kind, func(t *testing.T) {
			c, desktop := fixture()
			reply := nativeResult{OK: true, State: []byte(desktop.state), WindowID: 11, Dispatched: true, DispatchMethod: "window_server_mouse"}
			var transportError error
			switch kind {
			case "capture_failed":
				reply.OK = false
				reply.Error = "capture failed after posting"
			case "native_transport_error":
				transportError = errors.New("readback failed after posting")
			case "pointer_refused":
				reply = nativeResult{Error: "target covered", BlockedReason: "pointer_target_changed"}
			}
			backend := &nativeOutcomeDesktop{fakeDesktop: desktop, result: reply, err: transportError}
			c.backend = backend
			observation := observed(t, c)
			x, y := 10.0, 20.0
			request := Request{Action: "click", PID: 42, Observation: observation.Observation, X: &x, Y: &y}
			result, err := c.Execute(t.Context(), request)
			if result.Dispatched != reply.Dispatched || result.DispatchMethod != reply.DispatchMethod || result.BlockedReason != reply.BlockedReason {
				t.Fatalf("native facts changed: %+v %v", result, err)
			}
			if (err == nil) != (kind == "success") {
				t.Fatalf("wrong error result: %v", err)
			}
			calls := len(backend.calls)
			if _, err := c.Execute(t.Context(), request); err == nil || len(backend.calls) != calls {
				t.Fatal("an attempted click reused its receipt")
			}
		})
	}
}
