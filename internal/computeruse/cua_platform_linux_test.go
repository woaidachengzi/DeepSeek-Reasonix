//go:build linux

package computeruse

import (
	"bytes"
	"testing"
)

func TestCuaLinuxDoesNotExposeUnboundCursorOverlayAsNativeMove(t *testing.T) {
	if bytes.Contains(CuaSchema(), []byte(`"move"`)) {
		t.Fatal("unbound Linux cursor overlay exposed as window-scoped input")
	}
	x, y := 10., 20.
	if (CuaRequest{Action: "move", PID: 42, WindowID: 11, X: &x, Y: &y}).Validate() == nil {
		t.Fatal("unsupported native move reached approval")
	}
	if err := (CuaRequest{Action: "browser_pointer", PID: 42, WindowID: 11, TargetID: "b1", TabID: "t1", Ref: "p1:e1", Gesture: "hover"}).Validate(); err != nil {
		t.Fatalf("scoped browser hover lost: %v", err)
	}
}
