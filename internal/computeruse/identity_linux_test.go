//go:build linux

package computeruse

import (
	"os"
	"os/exec"
	"testing"
)

func TestLinuxTargetResolvesOwnProcessAndRefusesExitedProcess(t *testing.T) {
	target, err := New().Target(t.Context(), os.Getpid())
	if err != nil || target.PID != os.Getpid() || target.LaunchTime <= 0 {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Wait()
	defer child.Process.Kill()
	approved, err := New().Target(t.Context(), child.Process.Pid)
	if err != nil || approved == target {
		t.Fatalf("child identity=%+v err=%v", approved, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if _, err := New().Target(t.Context(), child.Process.Pid); err == nil {
		t.Fatal("exited process retained its grant")
	}
	if _, err := (nativeBackend{}).call(t.Context(), Request{Action: "click", PID: os.Getpid()}); err == nil {
		t.Fatal("Linux entered legacy input backend")
	}
}
