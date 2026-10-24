package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"strings"
)

// PointerPosition comes from CGEventGetLocation, in global screen points.
type PointerPosition struct {
	ScreenX float64 `json:"screen_x"`
	ScreenY float64 `json:"screen_y"`
}

// PointerPreview labels the actual cursor, not the requested destination.
type PointerPreview struct {
	PointerPosition
	ImageX float64 `json:"image_x"`
	ImageY float64 `json:"image_y"`
}

func pointerWindowPoint(v nativeResult) (float64, float64, bool) {
	var state struct{ Bounds frameRect }
	if v.Pointer == nil || json.Unmarshal(v.State, &state) != nil {
		return 0, 0, false
	}
	x, y := v.Pointer.ScreenX-state.Bounds.X, v.Pointer.ScreenY-state.Bounds.Y
	valid := !math.IsNaN(x) && !math.IsNaN(y) && x >= 0 && y >= 0 && x < state.Bounds.Width && y < state.Bounds.Height
	return x, y, valid
}

func validatePointerAction(r Request) error {
	if r.Action == "confirm_click" && (r.X != nil || r.Y != nil || r.Element != "" || r.CoordinateSpace != "") {
		return errors.New("confirm_click uses only the move receipt; do not supply a new target")
	}
	if r.Action == "move" && (r.Element != "" || r.X == nil || r.Y == nil) {
		return errors.New("move requires screenshot x/y coordinates without an element path")
	}
	return nil
}

func pinVerifiedInput(r Request, preview, current nativeResult) (Request, error) {
	if r.Action == "confirm_click" {
		return confirmedPointerRequest(r, preview, current)
	}
	if r.Action == "type" || r.Action == "key" {
		var state struct {
			Focus json.RawMessage `json:"focus"`
		}
		_ = json.Unmarshal(current.State, &state)
		r.VerifiedFocus = state.Focus
	}
	return r, nil
}

func confirmedPointerRequest(r Request, preview, current nativeResult) (Request, error) {
	if preview.DispatchMethod != "window_server_move" || observationImageGeometry(preview) == nil {
		return r, errors.New("confirm_click requires a successful move preview; inspect its cursor marker first")
	}
	x, y, ok := pointerWindowPoint(preview)
	if !ok || identity(preview) != identity(current) {
		return r, errors.New("cursor preview is unavailable for this window; move and inspect again")
	}
	pointer := *preview.Pointer
	r.X, r.Y, r.CoordinateSpace, r.VerifiedPointer = &x, &y, "window", &pointer
	return r, nil
}

func (c *Controller) publishAction(v nativeResult, r Request) (Result, error) {
	if r.Action != "move" {
		return c.publish(v, r.PID)
	}
	marked, pointer, err := pointerPreviewImage(v)
	if err != nil {
		result := nativeOutcome(v)
		result.BlockedReason, result.NextAction = "pointer_preview_unavailable", "observe"
		return result, err
	}
	// Only the raw, cursor-free capture enters the receipt digest. The marked
	// copy is sent to the model and PiP; it must never influence state checks.
	result, err := c.publish(v, r.PID)
	result.Image, result.PointerPreview, result.NextAction = marked, pointer, "inspect_pointer_then_confirm_click"
	return result, err
}

func pointerPreviewImage(v nativeResult) (string, *PointerPreview, error) {
	x, y, valid := pointerWindowPoint(v)
	geometry := observationImageGeometry(v)
	if !valid || geometry == nil || v.DispatchMethod != "window_server_move" || !v.Dispatched {
		return "", nil, errors.New("actual cursor preview unavailable; no click was sent; observe before continuing")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v.Image, "data:image/jpeg;base64,"))
	if err != nil {
		return "", nil, err
	}
	source, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	pointer := &PointerPreview{PointerPosition: *v.Pointer, ImageX: x / geometry.PointsPerPixelX, ImageY: y / geometry.PointsPerPixelY}
	marked := image.NewRGBA(source.Bounds())
	draw.Draw(marked, marked.Bounds(), source, source.Bounds().Min, draw.Src)
	drawPointerMarker(marked, int(math.Round(pointer.ImageX)), int(math.Round(pointer.ImageY)))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, marked, &jpeg.Options{Quality: 85}); err != nil {
		return "", nil, err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), pointer, nil
}

func drawPointerMarker(dst *image.RGBA, x, y int) {
	for dy := -14; dy <= 14; dy++ {
		for dx := -14; dx <= 14; dx++ {
			distance := dx*dx + dy*dy
			ring := distance >= 64 && distance <= 121
			cross := (dx >= -1 && dx <= 1) || (dy >= -1 && dy <= 1)
			if ring || cross {
				dst.SetRGBA(x+dx, y+dy, color.RGBA{R: 230, G: 30, B: 60, A: 255})
			} else if distance >= 49 && distance <= 144 {
				dst.SetRGBA(x+dx, y+dy, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
}
