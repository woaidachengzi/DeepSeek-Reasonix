package computeruse

import (
	"encoding/json"
	"errors"
	"math"
)

// CuaRequest intentionally exposes no shell, arbitrary JS, desktop-wide
// capture, clipboard, file paths, driver settings or session override.
type CuaRequest struct {
	Action       string   `json:"action"`
	PID          int      `json:"pid,omitempty"`
	WindowID     int      `json:"window_id,omitempty"`
	ElementToken string   `json:"element_token,omitempty"`
	CaptureID    string   `json:"capture_id,omitempty"`
	X            *float64 `json:"x,omitempty"`
	Y            *float64 `json:"y,omitempty"`
	Text         string   `json:"text,omitempty"`
	Key          string   `json:"key,omitempty"`
	Keys         []string `json:"keys,omitempty"`
	Direction    string   `json:"direction,omitempty"`
	Amount       int      `json:"amount,omitempty"`
	Query        string   `json:"query,omitempty"`
	DeliveryMode string   `json:"delivery_mode,omitempty"`
}

func (r CuaRequest) Validate() error {
	switch r.Action {
	case "apps", "status":
		return nil
	case "windows", "observe", "click", "type", "key", "hotkey", "scroll", "move":
	default:
		return errors.New("unsupported Cua action")
	}
	if err := r.validateTarget(); err != nil {
		return err
	}
	if err := r.validateBounds(); err != nil {
		return err
	}
	return r.validateInput()
}

func (r CuaRequest) validateTarget() error {
	if r.PID < 1 || r.PID > math.MaxInt32 || r.WindowID < 0 || uint64(r.WindowID) > math.MaxUint32 || (r.Action != "windows" && r.WindowID == 0) {
		return errors.New("select the approved app PID and exact window_id from windows")
	}
	return nil
}

func (r CuaRequest) validateBounds() error {
	if r.DeliveryMode != "" && r.DeliveryMode != "background" && r.DeliveryMode != "foreground" {
		return errors.New("invalid delivery_mode")
	}
	if (r.X == nil) != (r.Y == nil) || r.X != nil && (!finite(*r.X) || !finite(*r.Y) || *r.X < 0 || *r.Y < 0) {
		return errors.New("x and y must be paired finite screenshot coordinates")
	}
	if len(r.Text) > 16384 || len(r.Query) > 256 || len(r.ElementToken) > 128 || len(r.CaptureID) > 256 || len(r.Key) > 64 || len(r.Keys) > 4 {
		return errors.New("Cua argument exceeds its bound")
	}
	for _, key := range r.Keys {
		if len(key) > 64 || key == "" {
			return errors.New("invalid hotkey key")
		}
	}
	return nil
}

func (r CuaRequest) validateInput() error {
	if r.CaptureID != "" && (r.Action != "click" || r.X == nil) {
		return errors.New("capture_id is only supported for coordinate click")
	}
	if r.Action == "click" && ((r.ElementToken == "") == (r.X == nil)) {
		return errors.New("click needs either an observed element_token or screenshot x/y")
	}
	if (r.Action == "scroll" || r.Action == "move") && r.X == nil {
		return errors.New("scroll and move require screenshot x/y")
	}
	if r.Action == "type" && r.Text == "" || r.Action == "key" && r.Key == "" || r.Action == "hotkey" && len(r.Keys) < 2 {
		return errors.New("missing text or key")
	}
	if r.Action == "scroll" && (r.Amount < 1 || r.Amount > 20 || (r.Direction != "up" && r.Direction != "down")) {
		return errors.New("scroll requires up/down and amount between 1 and 20")
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func CuaSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["status","apps","windows","observe","click","type","key","hotkey","scroll","move"]},"pid":{"type":"integer","minimum":1},"window_id":{"type":"integer","minimum":1},"element_token":{"type":"string","description":"Exact token from this task's latest window observation"},"capture_id":{"type":"string"},"x":{"type":"number","minimum":0,"description":"Screenshot pixels from the returned image, never add screen origins"},"y":{"type":"number","minimum":0},"text":{"type":"string","maxLength":16384},"key":{"type":"string"},"keys":{"type":"array","items":{"type":"string"},"minItems":2,"maxItems":4},"direction":{"type":"string","enum":["up","down"]},"amount":{"type":"integer","minimum":1,"maximum":20},"query":{"type":"string","maxLength":256},"delivery_mode":{"type":"string","enum":["background","foreground"]}},"required":["action"],"additionalProperties":false}`)
}
