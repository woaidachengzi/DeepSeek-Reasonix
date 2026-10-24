package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"strings"
	"unicode/utf16"
)

// Browsers may expose the focused value and selection but no insertion bounds.
// Accept only a caret-shaped pixel transition, with every AX field unchanged.
// Positional actions additionally verify that their target is outside the
// focused field and the complete caret/JPEG change region (see caret_action.go).
func caretBlinkOnly(before, after nativeResult) bool {
	return !caretBlinkRegion(before, after).Empty()
}

func caretBlinkRegion(before, after nativeResult) image.Rectangle {
	if !bytes.Equal(canonicalState(before.State), canonicalState(after.State)) {
		return image.Rectangle{}
	}
	var state inputState
	if json.Unmarshal(after.State, &state) != nil || !state.Frontmost || !editableFocus(state.Focus) || state.Focus.Caret != nil {
		return image.Rectangle{}
	}
	f := state.Focus
	if f.Selection == nil || f.Selection.Length != 0 || f.Selection.Location < 0 || !insideRect(f.Bounds, state.Bounds, 0) {
		return image.Rectangle{}
	}
	length := len(utf16.Encode([]rune(*f.Value)))
	if f.ValueHash != "" {
		length = f.ValueLength
	} else if length >= 512 {
		return image.Rectangle{}
	}
	if f.Selection.Location > length {
		return image.Rectangle{}
	}
	a, b := caretFrame(before.Image), caretFrame(after.Image)
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return image.Rectangle{}
	}
	all, strong, count := pixelChanges(a, b)
	if strong.Empty() {
		return image.Rectangle{}
	}
	sx, sy := float64(a.Bounds().Dx())/state.Bounds.Width, float64(a.Bounds().Dy())/state.Bounds.Height
	bar := frameRect{X: state.Bounds.X + float64(strong.Min.X)/sx, Y: state.Bounds.Y + float64(strong.Min.Y)/sy, Width: float64(strong.Dx()) / sx, Height: float64(strong.Dy()) / sy}
	if bar.Width > 4 || bar.Height < 8 || bar.Height > 100 || !insideRect(bar, f.Bounds, 2) || float64(count) < float64(strong.Dy())*.7 {
		return image.Rectangle{}
	}
	mask := caretMask(state.Bounds, bar, a.Bounds())
	if !mask.Empty() && all.In(mask) {
		return mask
	}
	return image.Rectangle{}
}

func insideRect(inner, outer frameRect, padding float64) bool {
	for _, v := range []float64{inner.X, inner.Y, inner.Width, inner.Height, outer.X, outer.Y, outer.Width, outer.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return inner.Width > 0 && inner.Height > 0 && outer.Width > 0 && outer.Height > 0 && inner.X >= outer.X-padding && inner.Y >= outer.Y-padding && inner.X+inner.Width <= outer.X+outer.Width+padding && inner.Y+inner.Height <= outer.Y+outer.Height+padding
}

func caretFrame(data string) *image.RGBA {
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(data, prefix) || len(data) > 5<<20 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, prefix))
	if err != nil {
		return nil
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1920 || cfg.Height > 1920 {
		return nil
	}
	src, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func pixelChanges(a, b *image.RGBA) (all, strong image.Rectangle, count int) {
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			i := a.PixOffset(x, y)
			changed, high := false, false
			for c := 0; c < 3; c++ {
				delta := int(a.Pix[i+c]) - int(b.Pix[i+c])
				if delta != 0 {
					changed = true
				}
				if delta > 64 || delta < -64 {
					high = true
				}
			}
			pixel := image.Rect(x, y, x+1, y+1)
			if changed {
				if all.Empty() {
					all = pixel
				} else {
					all = all.Union(pixel)
				}
			}
			if high {
				count++
				if strong.Empty() {
					strong = pixel
				} else {
					strong = strong.Union(pixel)
				}
			}
		}
	}
	return
}
