package computeruse

import (
	"context"
	"encoding/json"
	"image"
	"testing"
)

type fillBackend struct {
	calls      []Request
	dispatched bool
	unknown    bool
}

func TestFillAcceptsOnlyVerifiedCaretBlink(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		before := browserCaretFrame(t, false, image.Rectangle{})
		extra := image.Rectangle{}
		if overlay {
			extra = image.Rect(180, 130, 184, 150)
		}
		after := browserCaretFrame(t, true, extra)
		c, f := fixture()
		f.state = string(before.State)
		f.image = before.Image
		receipt := observed(t, c)
		f.state = string(after.State)
		f.image = after.Image
		f.calls = nil
		x, y := 70.0, 70.0
		result, err := c.Execute(t.Context(), Request{Action: "fill", PID: 42, Observation: receipt.Observation, X: &x, Y: &y, Text: "local"})
		if overlay {
			if err == nil || result.Dispatched || len(f.calls) != 1 {
				t.Fatal("fill ignored a real change outside the caret")
			}
		} else if err != nil || !result.Dispatched || result.InputVerification != "unconfirmed" || len(f.calls) != 2 {
			t.Fatalf("verified caret blocked fill or invented readback: %+v %v", result, err)
		}
	}
}

func (f *fillBackend) call(_ context.Context, r Request) (nativeResult, error) {
	f.calls = append(f.calls, r)
	v := nativeResult{OK: true, WindowID: 11, State: json.RawMessage(`{"pid":42,"window_id":11,"frontmost":true}`), Image: "stable"}
	if r.Action == "fill" {
		v.Dispatched = f.dispatched
		v.DispatchUnknown = f.unknown
		v.InputVerification = "confirmed"
		v.Unchanged = !f.dispatched
	}
	return v, nil
}

func TestFillUsesOneVerifiedActionAndKeepsReadback(t *testing.T) {
	for _, dispatched := range []bool{true, false} {
		f := &fillBackend{dispatched: dispatched}
		c := New()
		c.backend = f
		receipt := observed(t, c)
		result, err := c.Execute(t.Context(), Request{Action: "fill", PID: 42, Observation: receipt.Observation, Element: "0.1", Text: "local draft\n第二行"})
		if err != nil || result.Dispatched != dispatched || result.Unchanged == dispatched || result.InputVerification != "confirmed" || len(f.calls) != 3 || f.calls[2].Action != "fill" {
			t.Fatalf("fill did not remain one verified native action: result=%+v calls=%+v err=%v", result, f.calls, err)
		}
		if _, err = c.Execute(t.Context(), Request{Action: "fill", PID: 42, Observation: receipt.Observation, Element: "0.1", Text: "repeat"}); err == nil {
			t.Fatal("fill receipt replayed")
		}
	}
}

func TestFillValidationAndChangedFrameSendNoInput(t *testing.T) {
	for _, request := range []Request{{Action: "fill", PID: 42, Observation: "x", Text: "value"}, {Action: "fill", PID: 42, Observation: "x", Element: "0.1"}} {
		if ValidateRequest(request) == nil {
			t.Fatal("incomplete fill accepted")
		}
	}
	c, f := fixture()
	receipt := observed(t, c)
	f.image = "changed"
	f.calls = nil
	result, err := c.Execute(t.Context(), Request{Action: "fill", PID: 42, Observation: receipt.Observation, Element: "0.1", Text: "draft"})
	if err == nil || result.Dispatched || !result.StateChanged || len(f.calls) != 1 || f.calls[0].Action != "observe" {
		t.Fatal("fill skipped changed-frame validation")
	}
}
