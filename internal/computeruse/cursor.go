package computeruse

import (
	"encoding/json"
	"math"
)

// CursorSnapshot is a display-only native cursor sample, separate from the
// canonical receipt state and the move-only confirmation target.
type CursorSnapshot struct {
	PointerPreview
	Clicked bool `json:"clicked,omitempty"`
}

func observedCursor(v nativeResult, g *ImageGeometry) *CursorSnapshot {
	var state inputState
	if json.Unmarshal(v.State, &state) != nil || !state.Frontmost || v.Cursor == nil {
		return nil
	}
	v.Pointer = v.Cursor
	x, y, valid := pointerWindowPoint(v)
	if !valid || g == nil {
		return nil
	}
	return &CursorSnapshot{PointerPreview: PointerPreview{PointerPosition: *v.Cursor, ImageX: x / g.PointsPerPixelX, ImageY: y / g.PointsPerPixelY}}
}

func markCursorClick(result *Result, r Request, before nativeResult) {
	if result.Cursor == nil || !result.Dispatched || result.DispatchMethod != "window_server_mouse" || r.Element != "" || r.X == nil || r.Y == nil || (r.Action != "click" && r.Action != "confirm_click") {
		return
	}
	var state inputState
	if json.Unmarshal(before.State, &state) != nil {
		return
	}
	x, y := state.Bounds.X+*r.X, state.Bounds.Y+*r.Y
	result.Cursor.Clicked = math.Hypot(result.Cursor.ScreenX-x, result.Cursor.ScreenY-y) <= 1
}
