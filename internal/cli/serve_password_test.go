package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestServePasswordFileHashAndPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("file-secret-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"serve", "web"} {
		var code int
		out := captureStdout(t, func() {
			code = runServeWithOptions([]string{"--hash-password", "--password", "argv-secret", "--password-file", path}, serveRunOptions{command: command})
		})
		if code != 0 || bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(out)), []byte("file-secret-123")) != nil {
			t.Fatalf("%s: code=%d, password-file hash did not match", command, code)
		}
		if strings.Contains(out, "file-secret-123") || strings.Contains(out, "argv-secret") {
			t.Fatal("plaintext password printed")
		}
	}
}

func TestServePasswordFileInvalidNeverFallsBack(t *testing.T) {
	for _, test := range []struct {
		body string
		mode os.FileMode
	}{
		{" \n", 0o600}, {"first\nsecond\n", 0o600}, {strings.Repeat("a", (64<<10)+1), 0o600}, {"secret\n", 0o644},
	} {
		if runtime.GOOS == "windows" && test.mode == 0o644 {
			continue
		}
		path := filepath.Join(t.TempDir(), "password")
		if err := os.WriteFile(path, []byte(test.body), test.mode); err != nil {
			t.Fatal(err)
		}
		// Validate both startup and hash generation before either can bootstrap.
		for _, hash := range []bool{false, true} {
			args := []string{"--auth", "password", "--password", "fallback-secret", "--password-file", path}
			if hash {
				args = append(args, "--hash-password")
			}
			var code int
			stderr := captureStderr(t, func() { code = runServeWithOptions(args, serveRunOptions{command: "serve"}) })
			if code != 1 || strings.Contains(stderr, "fallback-secret") || strings.Contains(stderr, test.body) {
				t.Fatalf("invalid file: code=%d, stderr=%q", code, stderr)
			}
		}
	}
	if _, err := servePassword("fallback", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("unreadable password file fell back to argv")
	}
}
