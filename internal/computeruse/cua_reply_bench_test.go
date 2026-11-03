package computeruse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// One persistent synthetic worker; no SDK driver, screen capture, thumbnail,
// input or model. Includes full reply encoding, pipe, Go decode and validation.
func BenchmarkCuaReplyPipeline(b *testing.B) {
	binary := os.Getenv("REASONIX_CUA_REPLY_BENCH_BIN")
	if binary == "" {
		b.Skip("build the offline reply_pipeline example and set REASONIX_CUA_REPLY_BENCH_BIN")
	}
	for _, size := range []int{64 << 10, 1 << 20, 8 << 20} {
		data := strings.Repeat("A", size)
		fixture, err := json.Marshal(map[string]any{
			"images":          []map[string]string{{"mime_type": "image/png", "data_base64": data}},
			"structured_json": `{"mode":"snapshot","effect":"unverifiable"}`,
			"text":            "Synthetic reply fixture",
		})
		if err != nil {
			b.Fatal(err)
		}
		for _, mode := range []string{"legacy", "borrowed"} {
			b.Run(fmt.Sprintf("%dKiB/%s", size>>10, mode), func(b *testing.B) {
				cmd := exec.Command(binary)
				cmd.Env = cuaProcessEnvironment(runtime.GOOS)
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
				sequence := uint64(0)
				call := func() {
					sequence++
					if _, err := io.WriteString(stdin, mode+"\n"); err != nil {
						b.Fatal(err)
					}
					if !lines.Scan() {
						b.Fatalf("synthetic reply worker ended: %v", lines.Err())
					}
					var reply CuaReply
					if err := json.Unmarshal(lines.Bytes(), &reply); err != nil {
						b.Fatal(err)
					}
					if reply.Protocol != 1 || reply.ID != sequence || reply.IsError || reply.SourceFingerprint != "fixture" || reply.SourceRevision != "fixture" || len(reply.Images) != 1 || reply.Images[0].MIME != "image/png" || reply.Images[0].Data != data {
						b.Fatal("reply lost identity or full image bytes")
					}
				}
				call()
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					call()
				}
			})
		}
	}
}
