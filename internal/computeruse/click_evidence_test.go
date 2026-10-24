package computeruse

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestComputerUseClickEvidenceNeverAcknowledgesApplicationEffect(t *testing.T) {
	for _, kind := range []string{"unchanged_page", "changed_page", "cursor_matched", "AX_press", "readback_failed", "unknown_delivery", "refused"} {
		t.Run(kind, func(t *testing.T) {
			c, desktop := fixture()
			reply := nativeResult{OK: true, State: json.RawMessage(desktop.state), WindowID: 11, Dispatched: true, DispatchMethod: "window_server_mouse"}
			var readbackErr error
			delivery, target := "posted", "window_only"
			switch kind {
			case "cursor_matched":
				desktop.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":100,"y":200,"width":1280,"height":720}}`
				desktop.image = coordinateImage(t, 640, 360)
				reply.State, reply.Image = json.RawMessage(desktop.state), desktop.image
				reply.Cursor = &PointerPosition{ScreenX: 110, ScreenY: 220}
			case "changed_page":
				reply.State = json.RawMessage(`{"pid":42,"window_id":11,"frontmost":true,"elements":[{"role":"AXButton","title":"Send"}]}`)
			case "AX_press":
				reply.DispatchMethod, target = "", "unverified"
			case "readback_failed":
				reply.State = nil
				readbackErr = errors.New("capture failed after posting")
			case "unknown_delivery":
				reply.Dispatched, reply.DispatchUnknown = false, true
				delivery, target = "unknown", "unverified"
				readbackErr = errors.New("native result unknown")
			case "refused":
				reply = nativeResult{BlockedReason: "pointer_target_changed", Error: "covered window"}
				delivery, target = "not_sent", "unverified"
			}
			c.backend = &nativeOutcomeDesktop{fakeDesktop: desktop, result: reply, err: readbackErr}
			o := observed(t, c)
			if o.ClickEvidence != nil {
				t.Fatal("observation invented a click outcome")
			}
			x, y := 10., 20.
			r, _ := c.Execute(t.Context(), Request{Action: "click", PID: 42, Observation: o.Observation, X: &x, Y: &y})
			if r.ClickEvidence == nil || r.ClickEvidence.InputDelivery != delivery || r.ClickEvidence.TargetVerification != target || r.ClickEvidence.ApplicationEffect != "unverified" {
				t.Fatalf("dispatch/readback became a page acknowledgement: %+v", r)
			}
			if kind == "cursor_matched" && (r.Cursor == nil || !r.Cursor.Clicked) {
				t.Fatal("test did not exercise actual clicked=true feedback")
			}
		})
	}
}

func TestComputerUseClickRefusalHasEvidenceWithoutCallingNative(t *testing.T) {
	for _, action := range []string{"click", "confirm_click"} {
		c, desktop := fixture()
		r := Request{Action: action, PID: 42, Observation: "foreign"}
		if action == "click" {
			x, y := 10., 20.
			r.X, r.Y = &x, &y
		}
		result, err := c.Execute(t.Context(), r)
		if err == nil || len(desktop.calls) != 0 || result.ClickEvidence == nil || result.ClickEvidence.InputDelivery != "not_sent" || result.ClickEvidence.ApplicationEffect != "unverified" {
			t.Fatalf("invalid receipt lost refusal evidence: %+v %v", result, err)
		}
	}
}
