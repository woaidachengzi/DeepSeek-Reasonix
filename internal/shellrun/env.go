package shellrun

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"reasonix/internal/proc"
	"reasonix/internal/secrets"
)

var loginPathCache = struct {
	sync.Mutex
	values map[[32]byte]string
}{values: make(map[[32]byte]string)}

// LoginShellPATH resolves the user's login PATH for both tool and ! commands.
// Include the environment in the cache key so profile or filtering changes
// cannot reuse a probe made with a different environment.
func LoginShellPATH(ctx context.Context) string {
	if runtime.GOOS == "windows" || ctx.Err() != nil {
		return ""
	}
	env := secrets.ProcessEnv()
	shell := loginShell(env)
	if shell == "" {
		return ""
	}
	key := sha256.Sum256([]byte(shell + "\x00" + strings.Join(env, "\x00")))
	loginPathCache.Lock()
	path, ok := loginPathCache.values[key]
	loginPathCache.Unlock()
	if ok {
		return path
	}
	path = probeLoginPATH(ctx, env, shell)
	if ctx.Err() == nil {
		loginPathCache.Lock()
		loginPathCache.values[key] = path
		loginPathCache.Unlock()
	}
	return path
}

func probeLoginPATH(ctx context.Context, env []string, shell string) string {
	const marker = "__REASONIX_LOGIN_PATH__="
	script := "printf '\\n" + marker + "%s\\n' \"$PATH\""
	for _, args := range [][]string{{"-l", "-i", "-c", script}, {"-l", "-c", script}, {"-c", script}} {
		if ctx.Err() != nil {
			break
		}
		if path := parseShellPATH(runShellPATHCommand(ctx, env, shell, args), marker); path != "" {
			return path
		}
	}
	return ""
}

func loginShell(env []string) string {
	var shell, path string
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "SHELL="); ok {
			shell = strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			path = value
		}
	}
	if strings.ContainsAny(shell, `/\`) && isExecutableFile(shell) {
		return shell
	}
	if shell != "" && !strings.ContainsAny(shell, `/\`) {
		for _, dir := range filepath.SplitList(path) {
			if filepath.IsAbs(dir) && isExecutableFile(filepath.Join(dir, shell)) {
				return filepath.Join(dir, shell)
			}
		}
	}
	for _, fallback := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if isExecutableFile(fallback) {
			return fallback
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

func runShellPATHCommand(parent context.Context, env []string, shell string, args []string) []byte {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	cmd := proc.CommandContext(ctx, shell, args...)
	cmd.Env = env
	proc.PrepareShellPATHProbe(cmd)
	proc.SetCancelKillsTree(cmd)
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.CombinedOutput()
	return out
}

func parseShellPATH(out []byte, marker string) string {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	for _, line := range slices.Backward(lines) {
		if rest, ok := strings.CutPrefix(line, marker); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// WithLoginPATH adds the login PATH without losing inherited entries or other
// environment variables. It does not introduce empty (current directory) entries.
func WithLoginPATH(env []string, loginPath string) []string {
	if runtime.GOOS == "windows" || strings.TrimSpace(loginPath) == "" {
		return env
	}
	var inherited string
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			inherited = value
		}
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "PATH=") {
			out = append(out, entry)
		}
	}
	return append(out, "PATH="+mergePathLists(loginPath, inherited))
}

func mergePathLists(primary, secondary string) string {
	var out []string
	seen := map[string]bool{}
	for _, path := range []string{primary, secondary} {
		for _, part := range filepath.SplitList(path) {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}
