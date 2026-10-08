//go:build windows

package terminalprocess

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWindowsConPTYInteractiveInputResizeAndExit(t *testing.T) {
	if available, reason := Available(); !available {
		t.Skip(reason)
	}
	shell, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal("test Command Prompt unavailable")
	}
	owned := t.TempDir()
	proc, err := Start(Spec{Path: shell, Args: []string{"/D", "/Q"}, Dir: owned,
		Env:     []string{"SystemRoot=" + os.Getenv("SystemRoot"), "TEMP=" + owned, "TMP=" + owned, "USERPROFILE=" + owned},
		Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proc.Close() })
	if err := proc.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	chunks := make(chan string, 16)
	cancelRead := make(chan struct{})
	t.Cleanup(func() { close(cancelRead) })
	go func() {
		defer close(chunks)
		buffer := make([]byte, 4096)
		for {
			n, err := proc.Read(buffer)
			if n > 0 {
				select {
				case chunks <- string(buffer[:n]):
				case <-cancelRead:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// The caret splits the typed marker, so echo cannot prove execution.
	if _, err := proc.Write([]byte("echo shared-con^pty-ready\r\nexit 17\r\n")); err != nil {
		t.Fatal("ConPTY input failed")
	}
	waited := make(chan struct{})
	var code int
	var waitErr error
	go func() { code, waitErr = proc.Wait(); close(waited) }()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	var output strings.Builder
	for !strings.Contains(output.String(), "shared-conpty-ready") {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				t.Fatal("ConPTY ended without executed marker")
			}
			if output.Len()+len(chunk) > 64<<10 {
				t.Fatal("owned terminal output exceeded test budget")
			}
			output.WriteString(chunk)
		case <-deadline.C:
			t.Fatal("owned ConPTY output timed out")
		}
	}
	select {
	case <-waited:
	case <-deadline.C:
		t.Fatal("owned ConPTY exit timed out")
	}
	if code != 17 || waitErr != nil {
		t.Fatal("ConPTY exit status not preserved")
	}
	codeAgain, errAgain := proc.Wait()
	if codeAgain != code || errAgain != waitErr {
		t.Fatal("ConPTY repeated wait changed result")
	}
	if err := proc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Close(); err != nil {
		t.Fatal("idempotent ConPTY close failed")
	}
}
