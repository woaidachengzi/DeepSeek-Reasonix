package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"math"
	"strings"
)

type frameRect struct{ X, Y, Width, Height float64 }

// Compare decoded pixels rather than JPEG metadata. The AX-provided insertion
// caret may blink without changing the UI's meaning; exclude only its small
// render region, including surrounding JPEG compression blocks. Focus, value,
// selection and geometry remain part of the separately hashed native state.
func framePixels(v nativeResult) []byte {
	encoded, ok := strings.CutPrefix(v.Image, "data:image/jpeg;base64,")
	if !ok {
		return []byte(v.Image)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return []byte(v.Image)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 1920 || config.Height > 1920 {
		return data
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	pixels := image.NewRGBA(im.Bounds())
	draw.Draw(pixels, pixels.Bounds(), im, im.Bounds().Min, draw.Src)
	var state struct {
		Bounds frameRect
		Focus  struct {
			Role   string
			Bounds frameRect
			Caret  *frameRect
		}
	}
	if json.Unmarshal(v.State, &state) == nil && state.Focus.Caret != nil {
		caret := *state.Focus.Caret
		// Some single-line AppKit fields report range bounds above the actual
		// field. Keep the AX caret's narrow x position, but use the field's
		// line height so its blinking insertion bar is fully covered.
		field := state.Focus.Bounds
		if state.Focus.Role == "AXTextField" && field.Height > 0 && field.Height <= 100 && caret.X >= field.X && caret.X+caret.Width <= field.X+field.Width {
			caret.Y, caret.Height = field.Y, field.Height
		}
		mask := caretMask(state.Bounds, caret, pixels.Bounds())
		if !mask.Empty() {
			draw.Draw(pixels, mask, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
		}
	}
	var dimensions [8]byte
	binary.BigEndian.PutUint32(dimensions[:4], uint32(config.Width))
	binary.BigEndian.PutUint32(dimensions[4:], uint32(config.Height))
	return append(dimensions[:], pixels.Pix...)
}

func caretMask(window, caret frameRect, bounds image.Rectangle) image.Rectangle {
	if window.Width <= 0 || window.Height <= 0 || caret.Width < 0 || caret.Width > 4 || caret.Height <= 0 || caret.Height > 100 {
		return image.Rectangle{}
	}
	if caret.X < window.X || caret.Y < window.Y || caret.X+caret.Width > window.X+window.Width || caret.Y+caret.Height > window.Y+window.Height {
		return image.Rectangle{}
	}
	sx, sy := float64(bounds.Dx())/window.Width, float64(bounds.Dy())/window.Height
	// Align to JPEG MCU boundaries so a blinking line's compression halo is
	// excluded too, without ignoring the surrounding field or other controls.
	x0 := int(math.Floor((caret.X-window.X)*sx/16))*16 - 16
	y0 := int(math.Floor((caret.Y-window.Y)*sy/16))*16 - 16
	x1 := int(math.Ceil((caret.X-window.X+math.Max(1, caret.Width))*sx/16))*16 + 16
	y1 := int(math.Ceil((caret.Y-window.Y+caret.Height)*sy/16))*16 + 16
	return image.Rect(x0, y0, x1, y1).Intersect(bounds)
}
