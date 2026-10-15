package plugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/sandbox"
	"reasonix/internal/secrets"
)

func TestStdioShellPATHProbeFiltersEnvWhenEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe")
	}
	secrets.SetFilterSubprocessEnv(true)
	t.Cleanup(func() { secrets.SetFilterSubprocessEnv(false) })
	t.Setenv("REASONIX_TEST_SECRET_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")

	out := runShellPATHCommand(context.Background(), "/bin/sh", []string{"-c", `printf 'tok=%s' "${REASONIX_TEST_SECRET_TOKEN:-none}"`})
	if !strings.Contains(string(out), "tok=none") {
		t.Fatalf("stdio shell PATH probe leaked filtered env: %q", out)
	}
}

func TestPrepareMCPPrivateStateWindowsPreservesHostTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mcp-state", "0123456789abcdef", "matlab")
	hostTemp := `C:\Users\user\AppData\Local\Temp`
	env := []string{"TMP=" + hostTemp, "TEMP=" + hostTemp, "TMPDIR=" + hostTemp}

	_, got, err := prepareMCPPrivateStateForOS(Spec{StateDir: root}, sandbox.Spec{}, env, "windows")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
		if value, ok := envValue(got, key); !ok || value != hostTemp {
			t.Fatalf("%s = %q, %v; want inherited host temp %q", key, value, ok, hostTemp)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "tmp")); !os.IsNotExist(err) {
		t.Fatalf("private Windows temp directory exists or stat failed: %v", err)
	}
	for key, want := range map[string]string{
		"XDG_CACHE_HOME": filepath.Join(root, "cache"),
		"XDG_STATE_HOME": filepath.Join(root, "state"),
	} {
		if value, ok := envValue(got, key); !ok || value != want {
			t.Fatalf("%s = %q, %v; want %q", key, value, ok, want)
		}
	}
}

func TestPrepareMCPPrivateStateUnixIsolatesTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mcp-state", "matlab")
	hostTemp := "/tmp/host"
	env := []string{"TMP=" + hostTemp, "TEMP=" + hostTemp, "TMPDIR=" + hostTemp}

	_, got, err := prepareMCPPrivateStateForOS(Spec{StateDir: root}, sandbox.Spec{}, env, "linux")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "tmp")
	for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
		if value, ok := envValue(got, key); !ok || value != want {
			t.Fatalf("%s = %q, %v; want private temp %q", key, value, ok, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("private Unix temp directory = (%v, %v), want directory", info, err)
	}
}

func TestPlaywrightSocketsAreShortPrivateAndOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses named pipes")
	}
	spec := Spec{Args: []string{"-y", "@playwright/mcp@0.0.83"}}
	env := []string{"TMPDIR=" + filepath.Join(t.TempDir(), strings.Repeat("long", 20))}
	_, first, cleanFirst, err := preparePlaywrightSockets(spec, sandbox.Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanFirst)
	_, second, cleanSecond, err := preparePlaywrightSockets(spec, sandbox.Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanSecond)
	a, _ := envValue(first, "PWTEST_SOCKETS_DIR")
	b, _ := envValue(second, "PWTEST_SOCKETS_DIR")
	if a == b || len(a) > 40 {
		t.Fatalf("socket dirs must be unique and short: %q, %q", a, b)
	}
	if info, err := os.Stat(a); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("socket dir must be private: %v %v", info, err)
	}
	if got, _ := envValue(first, "TMPDIR"); got != strings.TrimPrefix(env[0], "TMPDIR=") {
		t.Fatal("other temporary state must stay isolated")
	}
	transport := &stdioTransport{cleanupSockets: cleanFirst}
	transport.close()
	transport.close()
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("transport close did not remove its socket dir")
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatal("closing one transport removed another's socket dir")
	}
	_, custom, cleanup, err := preparePlaywrightSockets(spec, sandbox.Spec{}, []string{"PWTEST_SOCKETS_DIR=/custom"})
	if err != nil || cleanup != nil {
		t.Fatal("an explicit socket directory must stay user-owned")
	}
	if value, _ := envValue(custom, "PWTEST_SOCKETS_DIR"); value != "/custom" {
		t.Fatal("explicit directory was overridden")
	}
	_, _, cleanup, err = preparePlaywrightSockets(Spec{Args: []string{"other-mcp"}}, sandbox.Spec{}, env)
	if err != nil || cleanup != nil {
		t.Fatal("other MCP servers must retain their existing launch behavior")
	}
}
