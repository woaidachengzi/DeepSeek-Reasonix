package control

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sandbox"
)

func TestBangCommandUsesLoginShellPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX login shell")
	}
	dir := t.TempDir()
	program := filepath.Join(dir, "reasonix-login-only-program")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf 'login-only-ok'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(t.TempDir(), "login-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '\\n__REASONIX_LOGIN_PATH__=%s\\n' '"+dir+":/usr/bin:/bin'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	sink, done, events := collectSink()
	c := &Controller{sink: sink, shell: sandbox.Shell{Kind: sandbox.ShellBash, Path: "/bin/sh"}}
	c.Submit("!reasonix-login-only-program")
	if e := waitForDone(t, done); e.Err != nil {
		t.Fatal(e.Err)
	}
	for _, e := range *events {
		if e.Kind == event.ToolResult && e.Tool.Err == "" && strings.Contains(e.Tool.Output, "login-only-ok") {
			return
		}
	}
	t.Fatal("user command did not find the program installed on login PATH")
}
