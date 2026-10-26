//go:build linux

package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type nativeBackend struct{}

// Linux has no legacy native backend. Register the tool only when its private
// Cua executable is actually present beside the bridge.
func Available() bool { return BundledCuaRuntime() != nil }

func (nativeBackend) call(ctx context.Context, r Request) (nativeResult, error) {
	if err := ctx.Err(); err != nil {
		return nativeResult{}, err
	}
	if r.Action != "resolve_target" {
		return nativeResult{}, errors.New("Linux desktop operations require the bundled Cua runtime")
	}
	target, err := resolveLinuxTarget(r.PID)
	if err != nil {
		return nativeResult{}, err
	}
	state, err := json.Marshal(map[string]any{"target": target})
	return nativeResult{OK: err == nil, State: state}, err
}

func resolveLinuxTarget(pid int) (Target, error) {
	if pid < 1 || uint64(pid) > 1<<31-1 {
		return Target{}, errors.New("invalid Linux app PID")
	}
	// Pin the proc directory so PID reuse cannot redirect subsequent reads.
	process, err := os.Open(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return Target{}, err
	}
	defer process.Close()
	info, err := process.Stat()
	if err != nil {
		return Target{}, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) {
		return Target{}, errors.New("Linux app must belong to the current desktop user")
	}
	root := fmt.Sprintf("/proc/self/fd/%d", process.Fd())
	read := func(name string) (string, error) {
		value, err := os.ReadFile(filepath.Join(root, name))
		return string(value), err
	}
	stat, err := read("stat")
	if err != nil {
		return Target{}, err
	}
	ticks, err := linuxStartTicks(stat)
	if err != nil {
		return Target{}, err
	}
	executable, err := os.Readlink(root + "/exe")
	if err != nil || !filepath.IsAbs(executable) || strings.HasSuffix(executable, " (deleted)") {
		return Target{}, errors.New("Linux app executable unavailable")
	}
	exe, err := os.Stat(root + "/exe")
	if err != nil {
		return Target{}, err
	}
	metadata, ok := exe.Sys().(*syscall.Stat_t)
	if !ok || !exe.Mode().IsRegular() {
		return Target{}, errors.New("invalid Linux executable")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || strings.TrimSpace(string(boot)) == "" {
		return Target{}, errors.New("Linux boot identity unavailable")
	}
	namespace, err := os.Readlink(root + "/ns/pid")
	if err != nil {
		return Target{}, err
	}
	name, err := read("comm")
	if err != nil || strings.TrimSpace(name) == "" {
		return Target{}, errors.New("Linux app name unavailable")
	}
	again, err := read("stat")
	if err != nil {
		return Target{}, err
	}
	currentTicks, err := linuxStartTicks(again)
	currentExe, exeErr := os.Stat(root + "/exe")
	currentPath, pathErr := os.Readlink(root + "/exe")
	if err != nil || currentTicks != ticks || exeErr != nil || !os.SameFile(exe, currentExe) || pathErr != nil || currentPath != executable {
		return Target{}, errors.New("Linux app changed during identity read")
	}
	return Target{PID: pid, Name: strings.TrimSpace(name), BundleID: linuxExecutableIdentity(string(boot), namespace, uint64(owner.Uid), metadata.Dev, metadata.Ino, executable), LaunchTime: float64(ticks)}, nil
}
