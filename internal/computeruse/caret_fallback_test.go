package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"
)

func browserCaretFrame(t *testing.T, caret bool, extra image.Rectangle) nativeResult {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 256, 192))
	draw.Draw(im, im.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	if caret {
		draw.Draw(im, image.Rect(64, 64, 65, 84), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	if !extra.Empty() {
		draw.Draw(im, extra, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, im, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatal(err)
	}
	return nativeResult{OK: true, WindowID: 11, State: json.RawMessage(`{"pid":42,"window_id":11,"bundle_id":"fixture.browser","launch_time":1,"frontmost":true,"bounds":{"x":-256,"y":29,"width":256,"height":192},"focus":{"role":"AXTextField","bounds":{"x":-196,"y":89,"width":140,"height":30},"value":"","selection":{"location":0,"length":0}}}`), Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())}
}

func TestBrowserMissingCaretBoundsAllowsOnlyKeyboardBlink(t *testing.T) {
	for _, action := range []string{"type", "key", "click", "scroll"} {
		t.Run(action, func(t *testing.T) {
			before, after := browserCaretFrame(t, false, image.Rectangle{}), browserCaretFrame(t, true, image.Rectangle{})
			c, f := fixture()
			f.state = string(before.State)
			f.image = before.Image
			receipt := observed(t, c)
			f.state = string(after.State)
			f.image = after.Image
			f.calls = nil
			x, y := 70.0, 70.0
			result, err := c.Execute(t.Context(), Request{Action: action, PID: 42, Observation: receipt.Observation, Text: "fixture", Key: "tab", X: &x, Y: &y, Delta: -1, VerifiedFocus: json.RawMessage(`{"role":"forged"}`)})
			if action == "type" || action == "key" {
				if err != nil || !result.Dispatched || result.StateChanged || len(f.calls) != 2 {
					t.Fatalf("caret blink blocked keyboard: %+v %v", result, err)
				}
				var state struct {
					Focus json.RawMessage `json:"focus"`
				}
				_ = json.Unmarshal(after.State, &state)
				if !bytes.Equal(canonicalState(f.calls[1].VerifiedFocus), canonicalState(state.Focus)) {
					t.Fatal("native dispatch did not receive the actual verified focus")
				}
				if action == "type" && result.InputVerification != "unconfirmed" {
					t.Fatal("unread input was reported as confirmed")
				}
			} else if err == nil || result.Dispatched || !result.StateChanged || len(f.calls) != 1 {
				t.Fatal("positional input bypassed changed pixels")
			}
		})
	}
}

func TestBrowserCaretFallbackRejectsRealChanges(t *testing.T) {
	cases := []struct {
		name                   string
		extra                  image.Rectangle
		replaceOld, replaceNew string
	}{
		{name: "another control", extra: image.Rect(180, 130, 184, 150)},
		{name: "overlay within field", extra: image.Rect(62, 64, 82, 84)},
		{name: "text changed", replaceOld: `"value":""`, replaceNew: `"value":"edited"`},
		{name: "focus moved", replaceOld: `"x":-196`, replaceNew: `"x":-186`},
		{name: "selection moved", replaceOld: `"location":0`, replaceNew: `"location":1`},
		{name: "window moved", replaceOld: `"x":-256`, replaceNew: `"x":-255`},
		{name: "window replaced", replaceOld: `"launch_time":1`, replaceNew: `"launch_time":2`},
		{name: "lost foreground", replaceOld: `"frontmost":true`, replaceNew: `"frontmost":false`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := browserCaretFrame(t, false, image.Rectangle{}), browserCaretFrame(t, true, tc.extra)
			if tc.replaceOld != "" {
				b.State = bytes.ReplaceAll(b.State, []byte(tc.replaceOld), []byte(tc.replaceNew))
			}
			if caretBlinkOnly(a, b) {
				t.Fatal("real change was treated as a blinking caret")
			}
		})
	}
	for _, change := range [][2]string{
		{`"role":"AXTextField"`, `"role":"AXButton"`},
		{`"value":""`, `"value":"[secure]"`},
		{`"role":"AXTextField"`, `"role":"AXTextField","subrole":"AXSecureTextField"`},
		{`"selection":{"location":0,"length":0}`, `"selection":{"location":0,"length":1}`},
		{`"selection":{"location":0,"length":0}`, `"missing_selection":true`},
		{`"value":""`, `"missing_value":true`},
	} {
		a, b := browserCaretFrame(t, false, image.Rectangle{}), browserCaretFrame(t, true, image.Rectangle{})
		a.State = bytes.ReplaceAll(a.State, []byte(change[0]), []byte(change[1]))
		b.State = a.State
		if caretBlinkOnly(a, b) {
			t.Fatalf("unsupported focus accepted: %s", change[1])
		}
	}
}

func TestStateKeyOrderDoesNotInvalidateObservation(t *testing.T) {
	a := nativeResult{State: json.RawMessage(`{"pid":42,"focus":{"value":"","role":"AXTextField"}}`), Image: "same"}
	b := nativeResult{State: json.RawMessage(`{ "focus": {"role":"AXTextField", "value":""}, "pid":42 }`), Image: "same"}
	if digest(a) != digest(b) {
		t.Fatal("JSON key order was treated as a UI change")
	}
}
