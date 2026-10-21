package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/sandbox"
	"reasonix/internal/secrets"
)

func TestBashMergesLoginShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell PATH probing is POSIX-only")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	probe := filepath.Join(bin, "reasonix-path-probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'shell-path-ok\\n'\n"), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	// Inject a deterministic login-shell PATH instead of spawning a real login
	// shell. The real probe (shellrun.LoginShellPATH) runs up to three
	// interactive-login shells with a 2s timeout each; under the CPU load of
	// `go test ./...` it times out and returns an empty PATH, so this test failed
	// with command-not-found only in the full suite, never in isolation. This
	// test covers merging the probed PATH into the exec environment; the probe's
	// own parsing/merging is covered by TestParseShellPATH and TestMergePathLists.
	prev := bashShellPATH
	bashShellPATH = func(context.Context) string { return bin + ":/usr/bin:/bin" }
	t.Cleanup(func() { bashShellPATH = prev })

	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	b := bash{shell: sandbox.Shell{Kind: sandbox.ShellBash, Path: "/bin/sh"}}
	args, _ := json.Marshal(map[string]string{"command": "reasonix-path-probe"})

	out, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("command should resolve through merged login-shell PATH: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "shell-path-ok") {
		t.Fatalf("output = %q, want shell-path-ok", out)
	}
}

func TestBashCommandEnvFiltersSensitiveKeysWhenEnabled(t *testing.T) {
	secrets.SetFilterSubprocessEnv(true)
	t.Cleanup(func() { secrets.SetFilterSubprocessEnv(false) })
	t.Setenv("DEEPSEEK_API_KEY", "sk-real-secret-value-123456")
	t.Setenv("GH_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")
	t.Setenv("REASONIX_TEST_VISIBLE", "ok")
	// PWD is the POSIX working-directory variable, not a password: the name
	// filter must never strip it or every subprocess loses its cwd context.
	t.Setenv("PWD", "/tmp/somewhere")

	env := strings.Join(bashCommandEnv(context.Background()), "\n")
	if strings.Contains(env, "DEEPSEEK_API_KEY") || strings.Contains(env, "GH_TOKEN") {
		t.Fatalf("bash env leaked sensitive keys:\n%s", env)
	}
	if !strings.Contains(env, "REASONIX_TEST_VISIBLE=ok") {
		t.Fatalf("bash env dropped non-sensitive key:\n%s", env)
	}
	if !strings.Contains(env, "PWD=/tmp/somewhere") {
		t.Fatalf("bash env dropped PWD:\n%s", env)
	}
}

func TestBashCommandEnvKeepsTokensByDefault(t *testing.T) {
	t.Setenv("GH_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")

	env := strings.Join(bashCommandEnv(context.Background()), "\n")
	if !strings.Contains(env, "GH_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("bash env must inherit tokens while filter_subprocess_env is off (default):\n%s", env)
	}
}
