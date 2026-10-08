//go:build !windows

package terminalprocess

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func ownedShellSpec(t *testing.T, args ...string) Spec {
	t.Helper()
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal("test shell unavailable")
	}
	root := filepath.Join(t.TempDir(), "owned terminal 空格")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return Spec{Path: shell, Args: args, Dir: root,
		Env:     []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "TERM=xterm-256color", "PS1=", "ENV="},
		Columns: 80, Rows: 24}
}

// These tests operate only their own PTYs and private temporary workspaces.
// No login shell, actual profile, desktop terminal, clipboard or model is used.
func TestUnixPTYInteractiveInputResizeAndOwnedEnvironment(t *testing.T) {
	t.Setenv("REASONIX_TERMINAL_TEST_LEAK", "owned-sentinel")
	spec := ownedShellSpec(t, "-i")
	proc, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proc.Close() })
	if err := proc.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(proc.Resize(0, 30), ErrInvalidSpec) || !errors.Is(proc.Resize(100, MaxRows+1), ErrInvalidSpec) {
		t.Fatal("invalid resize accepted")
	}
	chunks := make(chan string, 16)
	readDone := make(chan struct{})
	cancelRead := make(chan struct{})
	t.Cleanup(func() { close(cancelRead) })
	go func() {
		defer close(readDone)
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
	// Split the expected output marker in the typed command so terminal echo
	// cannot be mistaken for execution. Both fd 0 and 1 must be real TTYs.
	command := "stty -echo\n" +
		"if test -t 0 && test -t 1; then printf '%s\\n' 'shared-''pty-ready'; fi\n" +
		"printf 'workdir:%s\\n' \"$PWD\"\n" +
		"printf 'inherited:%s\\n' \"${REASONIX_TERMINAL_TEST_LEAK-absent}\"\n" +
		"stty size\nexit 7\n"
	if _, err := proc.Write([]byte(command)); err != nil {
		t.Fatal("PTY input failed")
	}
	var output strings.Builder
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
read:
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				break read
			}
			if output.Len()+len(chunk) > 64<<10 {
				t.Fatal("owned terminal output exceeded test budget")
			}
			output.WriteString(chunk)
		case <-deadline.C:
			t.Fatal("owned terminal reader timed out")
		}
	}
	code, err := proc.Wait()
	if code != 7 || err == nil {
		t.Fatal("PTY exit status not preserved")
	}
	codeAgain, errAgain := proc.Wait()
	if codeAgain != code || errAgain != err {
		t.Fatal("PTY repeated wait changed result")
	}
	for _, expected := range []string{"shared-pty-ready", "workdir:" + spec.Dir, "inherited:absent", "30 100"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("owned PTY assertion failed: %s", strings.SplitN(expected, ":", 2)[0])
		}
	}
	<-readDone
	if err := proc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Close(); err != nil {
		t.Fatal("idempotent PTY close failed")
	}
}

func TestUnixPTYCloseReapsUnregisteredProcessAndKillsOwnedChild(t *testing.T) {
	spec := ownedShellSpec(t, "-c", `sleep 120 & printf '%s' "$!" > "$1"; wait`, "owned-terminal", "child.pid")
	proc, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proc.Close() })
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(spec.Dir, "child.pid"))
		if err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid > 1 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid <= 1 {
		t.Fatal("owned child did not start")
	}
	// Close before the host's Wait registration, as in cancelled admission.
	if err := proc.Close(); err != nil {
		t.Fatal(err)
	}
	actual := proc.(*unixProcess)
	select {
	case <-actual.waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("closed unregistered process was not reaped")
	}
	code, err := proc.Wait()
	if code == 0 || err == nil {
		t.Fatal("closed process reported successful completion")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owned foreground/background child survived PTY close")
}
