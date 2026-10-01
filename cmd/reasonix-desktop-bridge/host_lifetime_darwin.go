package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type nativeLeaseRecord struct {
	HostPID  int    `json:"hostPid"`
	LaunchID string `json:"launchId"`
}

// Rust publishes this lease before spawning the bridge. It contains no token
// or user data and authorizes removal only of this same private empty tempdir.
// A missing, replaced, aliased or mismatched lease grants no cleanup authority.
func nativeLaunchLease(cfg config) *startupLease {
	if cfg.hostPID <= 1 || len(cfg.launchID) < 32 || len(cfg.launchID) > 128 || !validRequestID(cfg.launchID) {
		return nil
	}
	directory := nativeReadinessParent(cfg.readyFile)
	if directory == nil {
		return nil
	}
	path := filepath.Join(filepath.Dir(cfg.readyFile), "launch-owner.json")
	marker, err := os.Lstat(path)
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0o600 || marker.Size() > 512 {
		return nil
	}
	owner, ok := marker.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return nil
	}
	read := func() bool {
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(marker, current) || !current.Mode().IsRegular() || current.Mode().Perm() != 0o600 {
			return false
		}
		file, err := os.Open(path)
		if err != nil {
			return false
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !os.SameFile(marker, opened) {
			return false
		}
		data, err := io.ReadAll(io.LimitReader(file, 513))
		var record nativeLeaseRecord
		return err == nil && len(data) <= 512 && json.Unmarshal(data, &record) == nil && record.HostPID == cfg.hostPID && record.LaunchID == cfg.launchID
	}
	if !read() {
		return nil
	}
	return &startupLease{
		directory: filepath.Dir(cfg.readyFile),
		lost:      func(pid int) bool { return pid == cfg.hostPID && os.Getppid() != pid },
		remove: func() {
			current := nativeReadinessParent(cfg.readyFile)
			if current == nil || !os.SameFile(directory, current) || !read() {
				return
			}
			if os.Remove(path) == nil {
				removeNativeReadinessParent(cfg.readyFile, directory)
			}
		},
	}
}

// Only an explicit native test and a private fixture can pause a real startup
// at these boundaries. The real host/parent loss, cancellation and cleanup
// still run; no renderer API or unbounded external provider is introduced.
func nativeStartupBoundary(ctx context.Context, cfg config, lease *startupLease, stage string) error {
	if cfg.hostPID == 0 || os.Getenv("REASONIX_TAURI_NATIVE_WINDOW_SMOKE") != stage {
		return nil
	}
	if lease == nil || os.Getppid() != cfg.hostPID {
		return errors.New("native startup test requires its actual leased parent")
	}
	root := filepath.Dir(lease.directory)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || filepath.Clean(os.Getenv("TMPDIR")) != root {
		return errors.New("native startup test requires a private temporary root")
	}
	data, err := json.Marshal(map[string]any{"phase": stage, "hostPid": cfg.hostPID, "sidecarPid": os.Getpid(), "tokenConsumed": stage == "startup-before-ready"})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "reasonix-native-startup.json"), data, 0o600); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(15 * time.Second):
		return errors.New("native startup test host termination did not arrive")
	}
}

func nativeStartupBeforeHostCheck(cfg config, lease *startupLease) error {
	if cfg.hostPID == 0 || os.Getenv("REASONIX_TAURI_NATIVE_WINDOW_SMOKE") != "startup-before-parent-check" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := time.NewTicker(100 * time.Millisecond)
	defer ticks.Stop()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		watchHostLifetime(ctx, cancel, cfg.hostPID, os.Getppid, ticks.C)
	}()
	err := nativeStartupBoundary(ctx, cfg, lease, "startup-before-parent-check")
	cancel()
	<-finished
	return err
}

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
