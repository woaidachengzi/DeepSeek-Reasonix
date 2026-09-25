package profilegate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTryAcquireExcludesSameProfileAndReleases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	release, err := TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := TryAcquire(root); !errors.Is(err, ErrHeld) {
		t.Fatalf("second owner error = %v, want ErrHeld", err)
	}
	otherRelease, err := TryAcquire(filepath.Join(t.TempDir(), "other"))
	if err != nil {
		t.Fatalf("different profile should be independent: %v", err)
	}
	otherRelease()
	release()
	thirdRelease, err := TryAcquire(root)
	if err != nil {
		t.Fatalf("profile should be available after release: %v", err)
	}
	thirdRelease()
}

func TestTryAcquireResolvesProfileAlias(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "profile")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	release, err := TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := TryAcquire(alias); !errors.Is(err, ErrHeld) {
		t.Fatalf("alias owner error = %v, want ErrHeld", err)
	}
}

func TestTryAcquireRejectsEmptyAndFilesystemRoot(t *testing.T) {
	for _, root := range []string{"", "  ", string(filepath.Separator)} {
		if _, err := TryAcquire(root); err == nil {
			t.Fatalf("TryAcquire(%q) unexpectedly succeeded", root)
		}
	}
}

func TestTryAcquireRejectsSymlinkLockFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, lockName)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := TryAcquire(root); err == nil {
		t.Fatal("symlink gate was accepted")
	}
}

func TestTryAcquireExcludesOtherProcess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	release, err := TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	runChild := func(expectHeld bool) {
		t.Helper()
		child := exec.Command(os.Args[0], "-test.run=^TestProfileGateChildProcess$")
		expected := "available"
		if expectHeld {
			expected = "held"
		}
		child.Env = append(os.Environ(), "REASONIX_PROFILEGATE_CHILD_ROOT="+root,
			"REASONIX_PROFILEGATE_CHILD_EXPECT="+expected)
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("child profile gate check (%s): %v\n%s", expected, err, output)
		}
	}
	runChild(true)
	release()
	runChild(false)
}

func TestTryAcquireReleasesAfterOwnerProcessExit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	marker := filepath.Join(t.TempDir(), "owner-acquired")
	child := exec.Command(os.Args[0], "-test.run=^TestProfileGateChildProcess$")
	child.Env = append(os.Environ(),
		"REASONIX_PROFILEGATE_CHILD_ROOT="+root,
		"REASONIX_PROFILEGATE_CHILD_EXPECT=hold",
		"REASONIX_PROFILEGATE_CHILD_MARKER="+marker,
	)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatalf("start profile lock owner: %v", err)
	}
	waitChild := make(chan error, 1)
	go func() { waitChild <- child.Wait() }()
	childRunning := true
	t.Cleanup(func() {
		if childRunning {
			_ = child.Process.Kill()
			<-waitChild
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Until(deadline) <= 0 {
			_ = child.Process.Kill()
			<-waitChild
			childRunning = false
			t.Fatal("child did not publish profile lock ownership")
		}
		select {
		case err := <-waitChild:
			childRunning = false
			t.Fatalf("profile lock owner exited before signaling: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := TryAcquire(root); !errors.Is(err, ErrHeld) {
		t.Fatalf("second process could acquire a held profile: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		<-waitChild
		childRunning = false
		t.Fatalf("forcibly stop profile lock owner: %v", err)
	}
	if err := <-waitChild; err == nil {
		childRunning = false
		t.Fatal("profile lock owner unexpectedly exited cleanly")
	}
	childRunning = false
	release, err := TryAcquire(root)
	if err != nil {
		t.Fatalf("profile lock remained held after owner process exit: %v", err)
	}
	release()
}

func TestProfileGateChildProcess(t *testing.T) {
	root := os.Getenv("REASONIX_PROFILEGATE_CHILD_ROOT")
	if root == "" {
		return
	}
	release, err := TryAcquire(root)
	if os.Getenv("REASONIX_PROFILEGATE_CHILD_EXPECT") == "hold" {
		if err != nil {
			t.Fatalf("child could not acquire profile: %v", err)
		}
		marker := os.Getenv("REASONIX_PROFILEGATE_CHILD_MARKER")
		if marker == "" {
			t.Fatal("child profile lock marker path is empty")
		}
		if err := os.WriteFile(marker, []byte("held\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour) // The parent kills this owner to exercise OS lock release.
		}
	}
	if os.Getenv("REASONIX_PROFILEGATE_CHILD_EXPECT") == "held" {
		if !errors.Is(err, ErrHeld) {
			t.Fatalf("other process owns profile; child got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	release()
}
