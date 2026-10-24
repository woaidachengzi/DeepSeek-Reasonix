package computeruse

import (
	"encoding/json"
	"testing"
)

func TestComputerUseCursorSampleIsDisplayOnlyAndUsesActualGeometry(t *testing.T) {
	v := nativeResult{State: json.RawMessage(`{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":-1920,"y":-300,"width":1280,"height":720}}`), Image: coordinateImage(t, 640, 360), Cursor: &PointerPosition{ScreenX: -1520, ScreenY: -100}}
	r := nativeOutcome(v)
	if r.Cursor == nil || r.Cursor.ImageX != 200 || r.Cursor.ImageY != 100 || r.Cursor.Clicked {
		t.Fatalf("actual cursor mapping failed: %+v", r.Cursor)
	}
	before := digest(v)
	v.Cursor = &PointerPosition{ScreenX: -1500, ScreenY: -80}
	if digest(v) != before {
		t.Fatal("cursor presentation changed the canonical observation")
	}
	if _, err := confirmedPointerRequest(Request{Action: "confirm_click"}, v, v); err == nil {
		t.Fatal("display cursor became a move-confirmation grant")
	}
	v.Cursor.ScreenX = 999
	if nativeOutcome(v).Cursor != nil {
		t.Fatal("out-of-window pointer was displayed")
	}
}

func TestComputerUseCursorRippleRequiresProvenCoordinateClick(t *testing.T) {
	v := nativeResult{State: json.RawMessage(`{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":-1920,"y":30,"width":1280,"height":720}}`), Image: coordinateImage(t, 640, 360), Cursor: &PointerPosition{ScreenX: -1520, ScreenY: 230}}
	x, y := 400.0, 200.0
	for _, kind := range []string{"click", "confirm_click", "AX-press", "refused", "typing", "moved", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			r := nativeOutcome(v)
			r.Dispatched, r.DispatchMethod = true, "window_server_mouse"
			request := Request{Action: kind, X: &x, Y: &y}
			switch kind {
			case "AX-press":
				request.Action, request.Element = "click", "0.1"
			case "refused":
				request.Action, r.Dispatched = "click", false
			case "moved":
				request.Action, r.Cursor.ScreenX = "click", -1400
			case "unknown":
				request.Action, r.Dispatched, r.DispatchUnknown = "click", false, true
			}
			markCursorClick(&r, request, v)
			if r.Cursor.Clicked != (kind == "click" || kind == "confirm_click") {
				t.Fatalf("unproven click feedback: %s %+v", kind, r.Cursor)
			}
		})
	}
}
