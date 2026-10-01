package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func leasedStartup(t *testing.T, pid int) (config, string) {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "reasonix-tauri-bridge-startup")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config{hostPID: pid, launchID: testToken, readyFile: filepath.Join(parent, "ready.json"), listen: "127.0.0.1:0"}
	record, err := json.Marshal(nativeLeaseRecord{HostPID: pid, LaunchID: cfg.launchID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "launch-owner.json"), record, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg, parent
}

func TestStartupLeaseReclaimsBeforeTokenOrReadyWithoutCoreWrites(t *testing.T) {
	// A lease's recorded parent no longer matches the kernel parent, including
	// the reparenting/PID-reuse startup path. Refusal must precede stdin/core.
	cfg, parent := leasedStartup(t, os.Getpid())
	root := filepath.Join(t.TempDir(), "missing-core")
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	if err := runFromStdin(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(parent); !os.IsNotExist(err) {
		t.Fatal("leased empty startup directory remains")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("lost parent created core state")
	}
}

func TestStartupLeaseRejectsMismatchesAndProtectsReplacementAndOtherFiles(t *testing.T) {
	for _, change := range []string{"id", "pid", "mode", "alias", "replacement", "other-file"} {
		t.Run(change, func(t *testing.T) {
			cfg, parent := leasedStartup(t, os.Getppid())
			marker := filepath.Join(parent, "launch-owner.json")
			lease := nativeLaunchLease(cfg)
			if lease == nil {
				t.Fatal("valid startup lease rejected")
			}
			switch change {
			case "id":
				cfg.launchID = "different-0123456789abcdef0123456789abcdef"
			case "pid":
				cfg.hostPID = os.Getpid()
			case "mode":
				if err := os.Chmod(marker, 0o644); err != nil {
					t.Fatal(err)
				}
			case "alias":
				if err := os.Rename(marker, marker+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(marker+"-original", marker); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := os.Rename(parent, parent+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(parent, 0o700); err != nil {
					t.Fatal(err)
				}
			case "other-file":
				if err := os.WriteFile(filepath.Join(parent, "keep"), []byte("original"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if change == "id" || change == "pid" || change == "mode" || change == "alias" {
				if nativeLaunchLease(cfg) != nil {
					t.Fatal("invalid startup lease accepted")
				}
				if change == "id" || change == "pid" {
					return
				}
			}
			lease.cleanup()
			if _, err := os.Lstat(parent); err != nil {
				t.Fatal("protected startup directory removed")
			}
			if change == "other-file" {
				if data, err := os.ReadFile(filepath.Join(parent, "keep")); err != nil || string(data) != "original" {
					t.Fatal("another startup file changed")
				}
			}
		})
	}
}

func TestNativeOutputPipeGuardAllowsCleanupWithoutChangingExecSignals(t *testing.T) {
	for _, mode := range []string{"default", "guarded"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestNativeOutputPipeGuardHelper$")
			command.Env = append(os.Environ(), "REASONIX_NATIVE_PIPE_TEST="+mode)
			output, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = command.Process.Kill(); _ = input.Close(); _ = output.Close() })
			if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "pipe-ready\n" {
				t.Fatal("pipe guard helper did not reach its control boundary")
			}
			_ = output.Close() // Close the only output reader before allowing a write.
			if _, err := input.Write([]byte("go\n")); err != nil {
				t.Fatal(err)
			}
			err = command.Wait()
			if mode == "guarded" {
				if err != nil {
					t.Fatalf("guarded broken output prevented cleanup: %v", err)
				}
			} else {
				var exited *exec.ExitError
				if !errors.As(err, &exited) || exited.Sys().(syscall.WaitStatus).Signal() != syscall.SIGPIPE {
					t.Fatalf("default broken stdout did not terminate with SIGPIPE: %v", err)
				}
			}
		})
	}
}

func TestNativeOutputPipeGuardHelper(t *testing.T) {
	mode := os.Getenv("REASONIX_NATIVE_PIPE_TEST")
	if mode == "" {
		return
	}
	if mode == "guarded" {
		_ = nativeOutputPipeGuard(os.Getppid())
		// A subprocess must still have its default SIGPIPE behavior, not inherit
		// an ignored disposition from the bridge's signal notification guard.
		child := exec.Command("/bin/sh", "-c", "kill -PIPE $$")
		var exited *exec.ExitError
		if !errors.As(child.Run(), &exited) || exited.Sys().(syscall.WaitStatus).Signal() != syscall.SIGPIPE {
			os.Exit(2)
		}
	}
	fmt.Fprintln(os.Stdout, "pipe-ready")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		os.Exit(3)
	}
	_, err := os.Stdout.WriteString("write-after-host-death\n")
	if mode == "guarded" && errors.Is(err, syscall.EPIPE) {
		os.Exit(0)
	}
	os.Exit(4)
}

func TestNativeReadinessCleanupProtectsOtherFilesReplacementsAndAliases(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "reasonix-tauri-bridge-owned")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "ready.json")
	owned := nativeReadinessParent(path)
	if owned == nil {
		t.Fatal("native private directory not recognized")
	}
	canary := filepath.Join(parent, "other-file")
	if err := os.WriteFile(canary, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, owned)
	if data, err := os.ReadFile(canary); err != nil || string(data) != "keep" {
		t.Fatal("another file was removed")
	}
	if err := os.Rename(parent, parent+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, owned)
	if _, err := os.Stat(parent); err != nil {
		t.Fatal("replacement directory removed")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent+"-original", parent); err != nil {
		t.Fatal(err)
	}
	if nativeReadinessParent(path) != nil {
		t.Fatal("directory alias accepted")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, nativeReadinessParent(path))
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatal("owned empty directory remains")
	}
}
