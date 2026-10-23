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

func frameFixture(t *testing.T, caret, otherChange bool) nativeResult {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 256, 192))
	draw.Draw(im, im.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	if caret {
		draw.Draw(im, image.Rect(64, 64, 65, 84), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	if otherChange {
		draw.Draw(im, image.Rect(180, 100, 200, 120), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatal(err)
	}
	return nativeResult{State: json.RawMessage(`{"bounds":{"x":0,"y":0,"width":256,"height":192},"focus":{"role":"AXTextField","value":"","selection":{"location":0,"length":0},"caret":{"x":64,"y":64,"width":1,"height":20}}}`), Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())}
}

func TestComputerUseFrameIgnoresCaretBlinkButKeepsOtherChanges(t *testing.T) {
	off := frameFixture(t, false, false)
	on := frameFixture(t, true, false)
	if digest(off) != digest(on) {
		t.Fatal("caret blink changed the frame revision")
	}
	if digest(on) == digest(frameFixture(t, true, true)) {
		t.Fatal("a visual change outside the caret was ignored")
	}
	on.State = json.RawMessage(bytes.ReplaceAll(on.State, []byte(`"location":0`), []byte(`"location":1`)))
	if digest(off) == digest(on) {
		t.Fatal("focus selection changed without invalidating the receipt")
	}
}

func TestComputerUseCaretMaskRejectsBroadOrForeignRegions(t *testing.T) {
	window := frameRect{Width: 256, Height: 192}
	for _, caret := range []frameRect{{X: 10, Y: 10, Width: 100, Height: 20}, {X: -1, Y: 10, Width: 1, Height: 20}, {X: 20, Y: 20, Width: 1, Height: 101}} {
		if !caretMask(window, caret, image.Rect(0, 0, 256, 192)).Empty() {
			t.Fatal("unsafe caret exclusion accepted")
		}
	}
}

func TestComputerUseSingleLineCaretUsesItsFieldHeight(t *testing.T) {
	off, on := frameFixture(t, false, false), frameFixture(t, true, false)
	state := json.RawMessage(`{"bounds":{"x":0,"y":0,"width":256,"height":192},"focus":{"role":"AXTextField","bounds":{"x":60,"y":64,"width":140,"height":24},"caret":{"x":64,"y":40,"width":0,"height":16}}}`)
	off.State, on.State = state, state
	if digest(off) != digest(on) {
		t.Fatal("AppKit range bounds outside a single-line field did not cover its insertion bar")
	}
	changed := frameFixture(t, true, true)
	changed.State = state
	if digest(on) == digest(changed) {
		t.Fatal("single-line caret masking hid another control")
	}
}
