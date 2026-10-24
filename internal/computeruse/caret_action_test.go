package computeruse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"testing"
	"time"
)

func pointerCaretFrames(t *testing.T, origin, scale float64) (nativeResult, nativeResult) {
	t.Helper()
	before, after := browserCaretFrame(t, false, image.Rectangle{}), browserCaretFrame(t, true, image.Rectangle{})
	state := fmt.Sprintf(`{"pid":42,"window_id":11,"bundle_id":"fixture.browser","launch_time":1,"frontmost":true,"bounds":{"x":%g,"y":29,"width":%g,"height":%g},"focus":{"role":"AXTextArea","bounds":{"x":%g,"y":%g,"width":%g,"height":%g},"value":"","selection":{"location":0,"length":0}}}`, origin, 256*scale, 192*scale, origin+60*scale, 29+60*scale, 140*scale, 30*scale)
	before.State, after.State = json.RawMessage(state), json.RawMessage(state)
	return before, after
}

func TestComputerUseCaretBlinkAllowsOnlyDistantPointerTargets(t *testing.T) {
	for _, action := range []string{"click", "move", "confirm_click"} {
		for _, origin := range []float64{-1920, 156} {
			for _, scale := range []float64{1, 2} {
				t.Run(fmt.Sprintf("%s/origin=%g/scale=%g", action, origin, scale), func(t *testing.T) {
					before, after := pointerCaretFrames(t, origin, scale)
					f := &previewDesktop{fakeDesktop: &fakeDesktop{state: string(before.State), image: before.Image}, pointer: &PointerPosition{ScreenX: origin + 230*scale, ScreenY: 29 + 150*scale}}
					c := &Controller{backend: f, now: time.Now}
					v := observed(t, c)
					x, y := 230.0, 150.0
					r := Request{Action: action, PID: 42, Observation: v.Observation, CoordinateSpace: "image", X: &x, Y: &y}
					if action == "confirm_click" {
						move := r
						move.Action = "move"
						preview, err := c.Execute(t.Context(), move)
						if err != nil {
							t.Fatal(err)
						}
						r = Request{Action: action, PID: 42, Observation: preview.Observation}
					}
					f.image, f.calls = after.Image, nil
					result, err := c.Execute(t.Context(), r)
					if err != nil || !result.Dispatched || result.StateChanged || result.ObservationValidation != "caret_only" || len(f.calls) != 2 {
						t.Fatalf("isolated caret blocked distant target: %+v %v", result, err)
					}
					posted := f.calls[1]
					if posted.Action != action || *posted.X != x*scale || *posted.Y != y*scale {
						t.Fatalf("caret exception changed pointer target: %+v", posted)
					}
					count := len(f.calls)
					if _, err := c.Execute(t.Context(), r); err == nil || len(f.calls) != count {
						t.Fatal("caret exception made a receipt replayable")
					}
				})
			}
		}
	}
}

func TestComputerUseCaretPointerExceptionKeepsRealChangesBlocked(t *testing.T) {
	for _, kind := range []string{"field", "halo", "AX-press", "scroll", "new-text", "selection", "controls", "focus", "target-pixels", "invalid-image", "out-of-window", "missing-preview"} {
		t.Run(kind, func(t *testing.T) {
			before, after := pointerCaretFrames(t, -256, 1)
			x, y := 230.0, 150.0
			r := Request{Action: "click", PID: 42, X: &x, Y: &y}
			switch kind {
			case "field":
				x, y = 70, 70
			case "halo":
				x, y = 55, 100
			case "AX-press":
				r.Element = "0.1"
			case "scroll":
				r.Action, r.Delta = "scroll", -1
			case "new-text":
				after.State = bytes.ReplaceAll(after.State, []byte(`"value":""`), []byte(`"value":"changed"`))
			case "selection":
				after.State = bytes.ReplaceAll(after.State, []byte(`"location":0`), []byte(`"location":1`))
			case "controls":
				after.State = append(after.State[:len(after.State)-1:len(after.State)-1], []byte(`,"elements":[{"role":"AXButton"}]}`)...)
			case "focus":
				after.State = bytes.ReplaceAll(after.State, []byte(`"x":-196`), []byte(`"x":-190`))
			case "target-pixels":
				after.Image = browserCaretFrame(t, true, image.Rect(228, 148, 232, 152)).Image
			case "invalid-image":
				after.Image = "unknown"
			case "out-of-window":
				x = 256
			case "missing-preview":
				r.Action, r.X, r.Y = "confirm_click", nil, nil
			}
			c, f := fixture()
			f.state, f.image = string(before.State), before.Image
			r.Observation = observed(t, c).Observation
			f.state, f.image, f.calls = string(after.State), after.Image, nil
			result, err := c.Execute(t.Context(), r)
			if err == nil || result.Dispatched || len(f.calls) != 1 || f.calls[0].Action != "observe" || result.ObservationValidation != "" {
				t.Fatalf("real/unsupported change reached pointer dispatch: %+v %v", result, err)
			}
		})
	}
}

func TestComputerUseCaretValidationSurvivesDispatchReadbackFailure(t *testing.T) {
	before, after := pointerCaretFrames(t, -256, 1)
	c, f := fixture()
	f.state, f.image = string(before.State), before.Image
	v := observed(t, c)
	f.image = after.Image
	c.backend = &nativeOutcomeDesktop{fakeDesktop: f, result: nativeResult{Error: "capture failed after click", Dispatched: true, DispatchMethod: "window_server_mouse"}}
	x, y := 230.0, 150.0
	r := Request{Action: "click", PID: 42, Observation: v.Observation, X: &x, Y: &y}
	result, err := c.Execute(t.Context(), r)
	if err == nil || !result.Dispatched || result.ObservationValidation != "caret_only" || result.Observation != "" {
		t.Fatalf("dispatch readback lost validation facts: %+v %v", result, err)
	}
	count := len(f.calls)
	if _, err := c.Execute(t.Context(), r); err == nil || len(f.calls) != count {
		t.Fatal("failed post-click readback became replayable")
	}
}
