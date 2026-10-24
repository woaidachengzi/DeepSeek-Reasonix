package computeruse

import (
	"encoding/json"
	"image"
	"math"
)

func caretBlinkActionAllowed(r Request, before, after nativeResult) bool {
	switch r.Action {
	case "type", "key", "fill":
		return caretBlinkOnly(before, after)
	case "click", "move", "confirm_click":
		mask := caretBlinkRegion(before, after)
		return !mask.Empty() && pointerOutsideCaret(r, before, after, mask)
	default:
		return false
	}
}

// The complete AX state must already match and every changed pixel must be
// explained by a thin caret. This does not relax AX presses, field clicks,
// scrolls, changes near the click target, or unknown coordinate geometry.
func pointerOutsideCaret(r Request, before, after nativeResult, mask image.Rectangle) bool {
	var err error
	if r.Action == "confirm_click" {
		r, err = confirmedPointerRequest(r, before, after)
	} else {
		r, err = windowPointRequest(r, before)
	}
	if err != nil || r.Element != "" || r.X == nil || r.Y == nil {
		return false
	}
	geometry := observationImageGeometry(after)
	var state inputState
	if geometry == nil || json.Unmarshal(after.State, &state) != nil {
		return false
	}
	x, y := *r.X, *r.Y
	if math.IsNaN(x) || math.IsNaN(y) || x < 0 || y < 0 || x >= state.Bounds.Width || y >= state.Bounds.Height {
		return false
	}
	scaleX, scaleY := geometry.PointsPerPixelX, geometry.PointsPerPixelY
	point := image.Pt(int(math.Floor(x/scaleX)), int(math.Floor(y/scaleY)))
	focus := state.Focus.Bounds
	field := image.Rect(
		int(math.Floor((focus.X-state.Bounds.X)/scaleX)), int(math.Floor((focus.Y-state.Bounds.Y)/scaleY)),
		int(math.Ceil((focus.X+focus.Width-state.Bounds.X)/scaleX)), int(math.Ceil((focus.Y+focus.Height-state.Bounds.Y)/scaleY)),
	)
	return !point.In(field.Inset(-2)) && !point.In(mask.Inset(-2))
}
