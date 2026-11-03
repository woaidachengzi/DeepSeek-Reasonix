package computeruse

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"testing"
)

// The external binary is the repository's offline preview_pipeline example.
// Both cases use one warm worker and the same complete image: benchmark time
// includes worker thumbnail work, full-reply JSON encoding/pipe/decode, and Go
// validation or legacy image decoding. Go allocation stats exclude the worker.
//
//	REASONIX_PREVIEW_BENCH_BIN=/absolute/path/preview_pipeline go test -run '^$' \
//	  -bench BenchmarkCuaPreviewPipeline -benchmem ./internal/computeruse
func BenchmarkCuaPreviewPipeline(b *testing.B) {
	binary := os.Getenv("REASONIX_PREVIEW_BENCH_BIN")
	if binary == "" {
		b.Skip("build the offline preview_pipeline example and set REASONIX_PREVIEW_BENCH_BIN")
	}
	for _, size := range []image.Point{{X: 640, Y: 400}, {X: 1920, Y: 1080}, {X: 1080, Y: 1920}} {
		for _, format := range []string{"png", "jpeg"} {
			fixture := previewBenchmarkFixture(b, size.X, size.Y, format)
			for _, mode := range []string{"legacy", "rust"} {
				b.Run(fmt.Sprintf("%dx%d_%s/%s", size.X, size.Y, format, mode), func(b *testing.B) {
					benchmarkPreviewPipe(b, binary, mode, fixture)
				})
			}
		}
	}
}

func previewBenchmarkFixture(b testing.TB, width, height int, format string) []byte {
	b.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, width, height))
	// Deterministic, synthetic UI-like tiles, gradient and high-frequency text
	// edges. No real screenshots, user content, APIs or system permissions.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tile := uint8(((x/64)*37 + (y/48)*71) % 256)
			c := color.NRGBA{R: tile, G: uint8(x / 8), B: uint8(y / 8), A: 255}
			if y%24 < 2 && x%20 < 14 {
				c = color.NRGBA{R: 230, G: 230, B: 230, A: 255}
			}
			source.SetNRGBA(x, y, c)
		}
	}
	var encoded bytes.Buffer
	if format == "png" {
		if err := png.Encode(&encoded, source); err != nil {
			b.Fatal(err)
		}
	} else if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 85}); err != nil {
		b.Fatal(err)
	}
	fixture, err := json.Marshal(map[string]string{"mime_type": "image/" + format, "data_base64": base64.StdEncoding.EncodeToString(encoded.Bytes())})
	if err != nil {
		b.Fatal(err)
	}
	return fixture
}

func benchmarkPreviewPipe(b *testing.B, binary, mode string, fixture []byte) {
	b.Helper()
	cmd := exec.Command(binary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		b.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		b.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if _, err := stdin.Write(append(fixture, '\n')); err != nil {
		b.Fatal(err)
	}
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 64<<10), maxCuaReply)
	call := func() {
		if _, err := io.WriteString(stdin, mode+"\n"); err != nil {
			b.Fatal(err)
		}
		if !lines.Scan() {
			b.Fatalf("preview worker ended: %v", lines.Err())
		}
		var reply CuaReply
		if err := json.Unmarshal(lines.Bytes(), &reply); err != nil || len(reply.Images) != 1 || reply.IsError {
			b.Fatalf("invalid benchmark reply: %v", err)
		}
		var thumbnail string
		if mode == "legacy" {
			thumbnail = PreviewImage("data:" + reply.Images[0].MIME + ";base64," + reply.Images[0].Data)
		} else {
			thumbnail = CuaPreviewImage(reply, 0)
			if len(reply.Preview) == 0 {
				b.Fatal("Rust route silently omitted the measured work")
			}
		}
		if thumbnail == "" {
			b.Fatal("benchmark omitted the thumbnail")
		}
	}
	call() // Exclude worker startup/fixture loading equally; production reuses it.
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		call()
	}
}
