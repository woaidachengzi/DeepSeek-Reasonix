package computeruse

import (
	"bytes"
	"context"
	"testing"
)

func TestCuaDoesNotAdvertiseOverlayAsPointerInput(t *testing.T) {
	if bytes.Contains(CuaSchema(), []byte(`"move"`)) {
		t.Fatal("overlay-only move was exposed as native input")
	}
	x, y := 10., 20.
	request := CuaRequest{Action: "move", PID: 42, WindowID: 11, X: &x, Y: &y}
	if request.Validate() == nil {
		t.Fatal("overlay-only move passed request validation")
	}
	c := cuaFixture("echo")
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := c.Call(t.Context(), lifetime, request, nil); err == nil || c.process != nil {
		t.Fatal("unsupported move started a worker", err)
	}
}
