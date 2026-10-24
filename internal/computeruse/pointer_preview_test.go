package computeruse

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type previewDesktop struct {
	*fakeDesktop
	pointer *PointerPosition
}

func (f *previewDesktop) call(ctx context.Context, r Request) (nativeResult, error) {
	v, err := f.fakeDesktop.call(ctx, r)
	if r.Action == "move" {
		v.Pointer, v.DispatchMethod = f.pointer, "window_server_move"
	}
	return v, err
}

func pointerFixture(t *testing.T) (*Controller, *previewDesktop, Result) {
	t.Helper()
	f := &previewDesktop{fakeDesktop: &fakeDesktop{
		state: `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":-1920,"y":30,"width":1280,"height":720}}`,
		image: coordinateImage(t, 640, 360),
	}, pointer: &PointerPosition{ScreenX: -1519.5, ScreenY: 230.25}}
	c := &Controller{backend: f, now: time.Now}
	return c, f, observed(t, c)
}

func movePointer(t *testing.T, c *Controller, receipt string) Result {
	t.Helper()
	x, y := 200.0, 100.0
	v, err := c.Execute(t.Context(), Request{Action: "move", PID: 42, Observation: receipt, CoordinateSpace: "image", X: &x, Y: &y})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestComputerUsePointerPreviewUsesActualPositionAndRawReceipt(t *testing.T) {
	c, f, observed := pointerFixture(t)
	v := movePointer(t, c, observed.Observation)
	if v.PointerPreview == nil || v.PointerPreview.ImageX != 200.25 || v.PointerPreview.ImageY != 100.125 || v.NextAction != "inspect_pointer_then_confirm_click" {
		t.Fatalf("actual position was replaced by requested coordinates: %+v", v.PointerPreview)
	}
	if v.Image == f.image || c.receipt.snapshot.Image != f.image || c.receipt.digest != digest(c.receipt.snapshot) {
		t.Fatal("marked image contaminated canonical receipt or was not marked")
	}
	imageV := nativeResult{State: json.RawMessage(f.state), Image: v.Image}
	if geometry := observationImageGeometry(imageV); geometry == nil || geometry.Width != 640 || geometry.Height != 360 {
		t.Fatal("marker changed screenshot geometry")
	}
	f.calls = nil
	confirmed, err := c.Execute(t.Context(), Request{Action: "confirm_click", PID: 42, Observation: v.Observation, VerifiedPointer: &PointerPosition{ScreenX: 999, ScreenY: 999}})
	if err != nil || !confirmed.Dispatched || len(f.calls) != 2 {
		t.Fatalf("confirmation rejected raw unchanged image: %+v %v", confirmed, err)
	}
	dispatch := f.calls[1]
	if dispatch.Action != "confirm_click" || *dispatch.X != 400.5 || *dispatch.Y != 200.25 || *dispatch.VerifiedPointer != *f.pointer || dispatch.WindowID != 11 {
		t.Fatalf("confirmation changed target or trusted supplied pointer: %+v", dispatch)
	}
	count := len(f.calls)
	if _, err := c.Execute(t.Context(), Request{Action: "confirm_click", PID: 42, Observation: v.Observation}); err == nil || len(f.calls) != count {
		t.Fatal("confirmation receipt replayed")
	}
}

func TestComputerUsePointerConfirmationInvalidation(t *testing.T) {
	for _, condition := range []string{"missing-preview", "new-observation", "changed-page", "moved-window", "expired", "cancelled"} {
		t.Run(condition, func(t *testing.T) {
			c, f, initial := pointerFixture(t)
			v := initial
			if condition != "missing-preview" {
				v = movePointer(t, c, initial.Observation)
			}
			switch condition {
			case "new-observation":
				v = observed(t, c)
			case "changed-page":
				f.image = coordinateImage(t, 641, 360)
			case "moved-window":
				f.state = strings.Replace(f.state, `"x":-1920`, `"x":0`, 1)
			case "expired":
				c.now = func() time.Time { return time.Now().Add(ObservationTTL) }
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if condition == "cancelled" {
				cancel()
			}
			f.calls = nil
			result, err := c.Execute(ctx, Request{Action: "confirm_click", PID: 42, Observation: v.Observation})
			if err == nil || result.Dispatched {
				t.Fatalf("invalid preview clicked: %+v %v", result, err)
			}
			for _, call := range f.calls {
				if call.Action != "observe" {
					t.Fatalf("invalid preview reached input: %+v", call)
				}
			}
		})
	}
}

func TestComputerUsePointerPreviewRequiresActualCursorAndImage(t *testing.T) {
	for _, kind := range []string{"missing-position", "outside-window", "invalid-image"} {
		t.Run(kind, func(t *testing.T) {
			c, f, v := pointerFixture(t)
			switch kind {
			case "missing-position":
				f.pointer = nil
			case "outside-window":
				f.pointer.ScreenX = 1000
			case "invalid-image":
				f.image = "not-an-image"
				v = observed(t, c)
			}
			x, y := 400.0, 200.0
			result, err := c.Execute(t.Context(), Request{Action: "move", PID: 42, Observation: v.Observation, X: &x, Y: &y})
			if err == nil || !result.Dispatched || result.Observation != "" || result.PointerPreview != nil || c.receipt.token != "" {
				t.Fatalf("unverified cursor issued confirmation: %+v %v", result, err)
			}
		})
	}
}

func TestComputerUsePointerActionsRejectRetargeting(t *testing.T) {
	x, y := 1.0, 2.0
	for _, r := range []Request{
		{Action: "move", Element: "0.1", X: &x, Y: &y},
		{Action: "move", X: &x},
		{Action: "confirm_click", X: &x, Y: &y},
		{Action: "confirm_click", Element: "0.1"},
		{Action: "confirm_click", CoordinateSpace: "window"},
	} {
		r.PID, r.Observation = 42, "preview"
		if ValidateRequest(r) == nil {
			t.Fatalf("invalid pointer request accepted: %+v", r)
		}
	}
}
