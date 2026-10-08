package desktopbridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func imageFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	pixels.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestWorkspaceImageUsesOwnedRootAndBoundedRasterPixels(t *testing.T) {
	root := t.TempDir()
	data := imageFixture(t, 2400, 1200)
	path := filepath.Join(root, "screen shot.png")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"screen%20shot.png", path, "file://" + filepath.ToSlash(path)} {
		view := readWorkspaceImage(root, source)
		if view.ErrorCode != "" || view.Size != int64(len(data)) || view.Mime != "image/png" {
			t.Fatalf("view: %+v", view)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(view.URL, "data:image/png;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width != 1200 || cfg.Height != 600 {
			t.Fatalf("bounded config: %+v %v", cfg, err)
		}
	}
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "outside.png")
	if err := os.WriteFile(outsidePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{outsidePath, "../outside.png", "%2e%2e/outside.png", "escape/outside.png", "file://foreign/screen.png", "https://example.org/image.png", "javascript:alert(1)", "screen%00shot.png", "screen%20shot.png?key=secret"} {
		if view := readWorkspaceImage(root, source); view.URL != "" || view.ErrorCode == "" {
			t.Fatalf("unsafe source authorized: %q", source)
		}
	}
	for _, pair := range []struct {
		name string
		data []byte
	}{{"secret.png", []byte("not-image-secret")}, {"script.svg", []byte(`<svg onload="alert(1)"/>`)}} {
		if err := os.WriteFile(filepath.Join(root, pair.name), pair.data, 0600); err != nil {
			t.Fatal(err)
		}
		if view := readWorkspaceImage(root, pair.name); view.URL != "" {
			t.Fatal("non-raster returned")
		}
	}
	if view := readWorkspaceImage(root, "."); view.ErrorCode != "forbidden" {
		t.Fatalf("directory: %+v", view)
	}
}

func TestWorkspaceImageRejectsBytePixelAndInlineFormatSpoofing(t *testing.T) {
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, "large.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(workspaceImageMaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := readWorkspaceImage(root, "large.png").ErrorCode; got != "too-large" {
		t.Fatal(got)
	}
	data := imageFixture(t, 1, 1)
	oversized := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(oversized[16:20], 100000)
	binary.BigEndian.PutUint32(oversized[20:24], 100000)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	if view := workspaceImagePixels(oversized, "bomb.png", int64(len(oversized)), ""); view.ErrorCode != "too-large" {
		t.Fatal(view.ErrorCode)
	}
	inline := base64.StdEncoding.EncodeToString(data)
	if view := readWorkspaceImage(root, "data:image/png;base64,"+inline); view.ErrorCode != "" || view.URL == "" {
		t.Fatal(view.ErrorCode)
	}
	if view := readWorkspaceImage(root, "data:image/jpeg;base64,"+inline); view.ErrorCode != "invalid-image" {
		t.Fatal(view.ErrorCode)
	}
	if view := readWorkspaceImage(root, "data:image/svg+xml;base64,"+inline); view.URL != "" {
		t.Fatal("inline SVG authorized")
	}
}

func TestWorkspaceImageRejectsOtherSessionAndClosedRuntime(t *testing.T) {
	runtime := &workspaceRuntime{fakeRuntime: &fakeRuntime{path: "/session", state: "idle"}, root: t.TempDir()}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.WorkspaceImage("other", "a.png"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
	if _, err := manager.WorkspaceImage("owned", "missing.png"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.WorkspaceImage("owned", "a.png"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
