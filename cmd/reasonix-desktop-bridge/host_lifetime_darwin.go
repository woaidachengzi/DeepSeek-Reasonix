package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// Tauri owns the output pipe readers. After its death, shutdown logging must
// return EPIPE rather than terminate this process before defers run. Notify
// handles SIGPIPE in this Go process; unlike Ignore, it does not make an
// ignored disposition survive exec into the core's shell/tool subprocesses.
func nativeOutputPipeGuard(expected int) func() {
	if expected <= 1 || os.Getppid() != expected {
		return func() {}
	}
	notifications := make(chan os.Signal, 1)
	signal.Notify(notifications, syscall.SIGPIPE)
	return func() { signal.Stop(notifications) }
}

// The native host creates this private TempDir and would normally remove it
// itself. After parent death, remove only that same empty directory, never a
// replacement, symlink, shared directory or user data beneath it.
func nativeReadinessParent(path string) os.FileInfo {
	parent := filepath.Dir(path)
	if filepath.Base(path) != "ready.json" || !strings.HasPrefix(filepath.Base(parent), "reasonix-tauri-bridge-") {
		return nil
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return nil
	}
	return info
}

func removeNativeReadinessParent(path string, owned os.FileInfo) {
	current := nativeReadinessParent(path)
	if current != nil && os.SameFile(owned, current) {
		_ = os.Remove(filepath.Dir(path)) // Fails if any other file is present.
	}
}
