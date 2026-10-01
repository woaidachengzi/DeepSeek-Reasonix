package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

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
