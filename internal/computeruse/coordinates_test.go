package computeruse

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"testing"
	"time"
)

func coordinateImage(t *testing.T, width, height int) string {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestComputerUseImageCoordinatesReachNativeAsWindowPoints(t *testing.T) {
	for _, action := range []string{"click", "fill"} {
		for _, origin := range []int{-1920, 156} {
			for _, size := range [][4]int{{1920, 1119, 1686, 983}, {1686, 983, 1686, 983}, {1920, 1080, 3840, 2160}} {
				t.Run(fmt.Sprintf("%s/origin=%d/size=%v", action, origin, size), func(t *testing.T) {
					c, f := fixture()
					f.state = fmt.Sprintf(`{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":%d,"y":30,"width":%d,"height":%d}}`, origin, size[2], size[3])
					f.image = coordinateImage(t, size[0], size[1])
					receipt := observed(t, c)
					if receipt.ImageGeometry == nil || receipt.ImageGeometry.Width != size[0] || receipt.ImageGeometry.Height != size[1] {
						t.Fatal("observation did not identify its actual image dimensions")
					}
					x, y := 800.0, 351.0
					f.calls = nil
					result, err := c.Execute(t.Context(), Request{Action: action, PID: 42, Observation: receipt.Observation, CoordinateSpace: "image", X: &x, Y: &y, Text: "local test"})
					if err != nil || !result.Dispatched || len(f.calls) != 2 || f.calls[0].Action != "observe" {
						t.Fatalf("mapping skipped live verification or input: %+v %v", result, err)
					}
					request := f.calls[1]
					wantX, wantY := x*float64(size[2])/float64(size[0]), y*float64(size[3])/float64(size[1])
					if math.Abs(*request.X-wantX) > 1e-8 || math.Abs(*request.Y-wantY) > 1e-8 || request.CoordinateSpace != "window" || request.WindowID != 11 || x != 800 || y != 351 {
						t.Fatalf("wrong screenshot-to-window mapping or mutated caller: %+v want=(%f,%f)", request, wantX, wantY)
					}
				})
			}
		}
	}
}

func TestComputerUseImageMappingRefusesUnknownAndOutOfImagePoints(t *testing.T) {
	for _, kind := range []string{"unknown-image", "unknown-bounds", "outside", "negative"} {
		t.Run(kind, func(t *testing.T) {
			c, f := fixture()
			f.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"width":1686,"height":983}}`
			f.image = coordinateImage(t, 1920, 1119)
			x, y := 800.0, 351.0
			switch kind {
			case "unknown-image":
				f.image = "not a screenshot"
			case "unknown-bounds":
				f.state = `{"pid":42,"window_id":11,"frontmost":true}`
			case "outside":
				x = 1920
			case "negative":
				y = -1
			}
			receipt := observed(t, c)
			request := Request{Action: "click", PID: 42, Observation: receipt.Observation, CoordinateSpace: "image", X: &x, Y: &y}
			f.calls = nil
			result, err := c.Execute(t.Context(), request)
			if err == nil || result.Dispatched || result.BlockedReason != "coordinate_mapping_unavailable" || result.Observation != "" || result.NextAction != "observe" || len(f.calls) != 1 {
				t.Fatalf("unverified image coordinate reached input: %+v %v", result, err)
			}
			if _, err := c.Execute(t.Context(), request); err == nil || len(f.calls) != 1 {
				t.Fatal("failed coordinate conversion left a replayable receipt")
			}
		})
	}
}

func TestComputerUseCoordinateSpaceDoesNotChangeLegacyOrStateGuards(t *testing.T) {
	for _, space := range []string{"", "window", "image"} {
		for _, change := range []string{"none", "window", "expired"} {
			c, f := fixture()
			f.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":156,"y":30,"width":1686,"height":983}}`
			f.image = coordinateImage(t, 1920, 1119)
			receipt := observed(t, c)
			x, y := 800.0, 351.0
			if change == "window" {
				f.state = `{"pid":42,"window_id":11,"frontmost":true,"bounds":{"x":0,"y":30,"width":1686,"height":983}}`
			} else if change == "expired" {
				c.now = func() time.Time { return time.Now().Add(ObservationTTL) }
			}
			f.calls = nil
			result, err := c.Execute(t.Context(), Request{Action: "click", PID: 42, Observation: receipt.Observation, CoordinateSpace: space, X: &x, Y: &y})
			if change != "none" {
				if err == nil || result.Dispatched || len(f.calls) > 1 {
					t.Fatal("coordinate mode bypassed state or receipt validation")
				}
			} else if err != nil || !result.Dispatched || (space != "image" && (*f.calls[1].X != x || *f.calls[1].Y != y)) {
				t.Fatal("legacy window-local points changed meaning")
			}
		}
	}
	x, y := 10.0, 10.0
	for _, r := range []Request{{Action: "click", CoordinateSpace: "screen", X: &x, Y: &y}, {Action: "click", CoordinateSpace: "image", Element: "0.1", X: &x, Y: &y}, {Action: "type", CoordinateSpace: "image", X: &x, Y: &y, Text: "text"}} {
		r.PID, r.Observation = 42, "receipt"
		if ValidateRequest(r) == nil {
			t.Fatal("ambiguous or inapplicable coordinate mode accepted")
		}
	}
}
