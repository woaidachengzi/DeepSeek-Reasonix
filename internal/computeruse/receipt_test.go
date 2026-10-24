package computeruse

import (
	"fmt"
	"testing"
	"time"
)

func TestModelRoundTripWithinReceiptStillRevalidatesEachMonitor(t *testing.T) {
	for _, origin := range []int{-1920, 0} {
		for _, delay := range []time.Duration{41 * time.Second, 95 * time.Second} {
			t.Run(fmt.Sprintf("origin=%d/delay=%s", origin, delay), func(t *testing.T) {
				c, desktop := fixture()
				now := time.Unix(1000, 0)
				c.now = func() time.Time { return now }
				desktop.state = fmt.Sprintf(`{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":%d,"y":30,"width":1920,"height":983}}`, origin)
				observation := observed(t, c)
				if observation.ObservationTTLMS != 120000 {
					t.Fatal("model did not receive the receipt lifetime")
				}
				now = now.Add(delay)
				x, y := 117.0, 165.0
				desktop.calls = nil
				result, err := c.Execute(t.Context(), Request{Action: "click", PID: 42, Observation: observation.Observation, X: &x, Y: &y})
				if err != nil || !result.Dispatched || len(desktop.calls) != 2 || desktop.calls[0].Action != "observe" {
					t.Fatalf("model delay blocked stable input or skipped validation: %+v %v", result, err)
				}
				input := desktop.calls[1]
				if input.Action != "click" || input.WindowID != 11 || *input.X != x || *input.Y != y {
					t.Fatal("monitor origin changed the window-local click request")
				}
			})
		}
	}
}

func TestLongerReceiptDoesNotAcceptMovedWindow(t *testing.T) {
	c, desktop := fixture()
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	desktop.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":-1920,"y":30,"width":1920,"height":983}}`
	observation := observed(t, c)
	now = now.Add(41 * time.Second)
	desktop.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":0,"y":30,"width":1920,"height":983}}`
	desktop.calls = nil
	x, y := 117.0, 165.0
	request := Request{Action: "click", PID: 42, Observation: observation.Observation, X: &x, Y: &y}
	result, err := c.Execute(t.Context(), request)
	if err == nil || result.Dispatched || !result.StateChanged || result.BlockedReason != "window_changed" || len(desktop.calls) != 1 {
		t.Fatalf("moving between monitors bypassed validation: %+v %v", result, err)
	}
	request.Observation = result.Observation
	now = now.Add(41 * time.Second)
	if result, err = c.Execute(t.Context(), request); err != nil || !result.Dispatched {
		t.Fatalf("fresh observation after move failed: %+v %v", result, err)
	}
}

func TestExpiredObservationExplainsNoInputAndFreshObserve(t *testing.T) {
	c, desktop := fixture()
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	observation := observed(t, c)
	now = now.Add(ObservationTTL)
	desktop.calls = nil
	result, err := c.Execute(t.Context(), Request{Action: "key", PID: 42, Key: "tab", Observation: observation.Observation})
	if err == nil || result.Dispatched || result.NextAction != "observe" || result.ObservationAgeMS != 120000 || result.ObservationTTLMS != 120000 || result.BlockedReason != "observation_expired" || len(desktop.calls) != 0 {
		t.Fatalf("expired receipt was replayed or lacked actionable status: %+v %v", result, err)
	}
}
