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

func TestCuaKeepsUnreviewedSdkSurfacesUnreachable(t *testing.T) {
	for _, action := range []string{"start_recording", "stop_recording", "page", "drag", "move", "move_cursor", "evaluate_javascript"} {
		request := CuaRequest{Action: action, PID: 42, WindowID: 11}
		if request.Validate() == nil {
			t.Errorf("unreviewed SDK action %q passed host validation", action)
		}
		worker := cuaFixture("echo")
		lifetime, cancel := context.WithCancel(t.Context())
		_, err := worker.Call(t.Context(), lifetime, request, nil)
		cancel()
		if err == nil || worker.process != nil {
			t.Errorf("%q started a private worker: %v", action, err)
		}
	}
}
