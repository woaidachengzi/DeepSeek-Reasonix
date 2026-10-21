package shellrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/internal/secrets"
)

func TestParseShellPATH(t *testing.T) {
	const marker = "__REASONIX_LOGIN_PATH__="
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"simple", marker + "/usr/local/bin:/usr/bin\n", "/usr/local/bin:/usr/bin"},
		{"crlf", "noise\r\n" + marker + "/opt/bin:/bin\r\n", "/opt/bin:/bin"},
		{"last marker wins", marker + "/early\n" + marker + "/late\n", "/late"},
		{"ignores surrounding output", "login banner\n" + marker + "/p\ntrailing\n", "/p"},
		{"absent", "no marker here\n", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseShellPATH([]byte(c.out), marker); got != c.want {
				t.Fatalf("parseShellPATH(%q) = %q, want %q", c.out, got, c.want)
			}
		})
	}
}

func TestMergePathLists(t *testing.T) {
	sep := string(os.PathListSeparator)
	cases := []struct {
		name      string
		primary   string
		secondary string
		want      string
	}{
		{"dedupes, primary first", "/a" + sep + "/b", "/b" + sep + "/c", "/a" + sep + "/b" + sep + "/c"},
		{"empty secondary", "/a" + sep + "/b", "", "/a" + sep + "/b"},
		{"empty primary", "", "/x" + sep + "/y", "/x" + sep + "/y"},
		{"skips blank entries", "/a" + sep + sep + "/b", "", "/a" + sep + "/b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mergePathLists(c.primary, c.secondary); got != c.want {
				t.Fatalf("mergePathLists(%q, %q) = %q, want %q", c.primary, c.secondary, got, c.want)
			}
		})
	}
}

func TestRunShellPATHCommandFiltersEnvWhenEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe")
	}
	secrets.SetFilterSubprocessEnv(true)
	t.Cleanup(func() { secrets.SetFilterSubprocessEnv(false) })
	t.Setenv("REASONIX_TEST_SECRET_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")

	out := runShellPATHCommand(context.Background(), secrets.ProcessEnv(), "/bin/sh", []string{"-c", `printf 'tok=%s' "${REASONIX_TEST_SECRET_TOKEN:-none}"`})
	if !strings.Contains(string(out), "tok=none") {
		t.Fatalf("login-shell PATH probe leaked filtered env: %q", out)
	}
}

func TestLoginPATHCacheFollowsEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX login shell")
	}
	shell := filepath.Join(t.TempDir(), "login-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '\\n__REASONIX_LOGIN_PATH__=%s\\n' \"$REASONIX_TEST_LOGIN_BIN\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	for _, path := range []string{"/first/bin", "/second/bin"} {
		t.Setenv("REASONIX_TEST_LOGIN_BIN", path)
		if got := LoginShellPATH(context.Background()); got != path {
			t.Fatalf("PATH=%q want %q", got, path)
		}
	}
}

func TestCanceledLoginPATHProbeDoesNotPoisonCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX login shell")
	}
	dir := t.TempDir()
	shell, ready := filepath.Join(dir, "login-shell"), filepath.Join(dir, "ready")
	script := "#!/bin/sh\nif [ ! -f \"$REASONIX_PATH_READY\" ]; then\n : > \"$REASONIX_PATH_READY\"\n /bin/sleep 30\nfi\nprintf '\\n__REASONIX_LOGIN_PATH__=/login/bin\\n'\n"
	if err := os.WriteFile(shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("REASONIX_PATH_READY", ready)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan string, 1)
	go func() { done <- LoginShellPATH(ctx) }()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("login-shell probe did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled probe retained child process output pipes")
	}
	if got := LoginShellPATH(context.Background()); got != "/login/bin" {
		t.Fatalf("cancellation poisoned the next probe: %q", got)
	}
}
