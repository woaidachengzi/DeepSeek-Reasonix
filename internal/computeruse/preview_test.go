package computeruse

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestPreviewImageAcceptsCuaPNG(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 640, 400))); err != nil {
		t.Fatal(err)
	}
	thumb := PreviewImage("data:image/png;base64," + base64.StdEncoding.EncodeToString(source.Bytes()))
	if !strings.HasPrefix(thumb, "data:image/jpeg;base64,") {
		t.Fatal("Cua PNG did not publish a bounded JPEG preview")
	}
}

func TestPreviewImageBoundedAndRejectsUntrustedInputs(t *testing.T) {
	var source bytes.Buffer
	_ = jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 1920, 1080)), nil)
	encoded := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(source.Bytes())
	thumb := PreviewImage(encoded)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(thumb, "data:image/jpeg;base64,"))
	if err != nil || thumb == "" || len(thumb) > 132000 {
		t.Fatalf("invalid thumbnail %d %v", len(thumb), err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 480 || cfg.Height != 270 {
		t.Fatalf("thumbnail dimensions %+v %v", cfg, err)
	}
	for _, bad := range []string{"https://example.com/private.jpg", "data:image/svg+xml;base64,AAAA", "data:image/jpeg;base64,not-jpeg"} {
		if PreviewImage(bad) != "" {
			t.Fatal("accepted invalid image")
		}
	}
}

func TestApprovedTargetReplacementNeverCaptures(t *testing.T) {
	c, f := fixture()
	f.state = `{"apps":[{"pid":42,"name":"Test","bundle_id":"example.test","launch_time":100}]}`
	target, err := c.Target(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	f.state = `{"apps":[{"pid":42,"name":"Test","bundle_id":"example.test","launch_time":200}]}`
	f.calls = nil
	if _, err = c.ExecuteForTarget(t.Context(), Request{Action: "observe", PID: 42}, target); err == nil {
		t.Fatal("replaced app accepted")
	}
	if len(f.calls) != 1 || f.calls[0].Action != "apps" {
		t.Fatal("captured before approval identity check")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f.calls = nil
	if _, err = c.ExecuteForTarget(ctx, Request{Action: "observe", PID: 42}, target); err == nil || len(f.calls) != 0 {
		t.Fatal("cancelled task reached native capture")
	}
}
