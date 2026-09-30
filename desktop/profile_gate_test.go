package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"reasonix/internal/profilegate"
)

func TestDesktopProfilesExcludeBridgeProfileAndReleaseAfterShutdown(t *testing.T) {
	configRoot, stateRoot := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", configRoot)
	t.Setenv("REASONIX_STATE_HOME", stateRoot)
	release, err := acquireDesktopProfiles()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := profilegate.TryAcquireDesktop(configRoot, stateRoot); !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("Wails-owned roots did not exclude bridge: %v", err)
	}
	release()
	bridgeRelease, err := profilegate.TryAcquireDesktop(configRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer bridgeRelease()
	if _, err := acquireDesktopProfiles(); !errors.Is(err, profilegate.ErrHeld) {
		t.Fatalf("bridge-owned roots did not exclude Wails: %v", err)
	}
}

func TestDesktopProfileOwnerRejectsRealBridgeWithSharedConfig(t *testing.T) {
	binary := os.Getenv("REASONIX_TAURI_BRIDGE_TEST_BIN")
	if binary == "" {
		t.Skip("set REASONIX_TAURI_BRIDGE_TEST_BIN to exercise the real bridge")
	}
	home, state := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", state)
	configPath := filepath.Join(home, "config.toml")
	original := []byte("default_model = \"shared-profile-test\"\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := acquireDesktopProfiles()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ready := filepath.Join(t.TempDir(), "ready.json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--ready-file", ready, "--launch-id", "profile-owner-test")
	// Same config home, a different session home: a state-only gate would
	// incorrectly admit this child and allow concurrent configuration writes.
	command.Env = append(os.Environ(), "REASONIX_STATE_HOME="+t.TempDir())
	command.Stdin = strings.NewReader(strings.Repeat("x", 48) + "\n")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "profile ownership") || ctx.Err() != nil {
		t.Fatalf("real bridge did not promptly refuse shared config: %v\n%s", err, output)
	}
	if _, err := os.Stat(ready); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected bridge published readiness: %v", err)
	}
	if content, err := os.ReadFile(configPath); err != nil || string(content) != string(original) {
		t.Fatalf("rejected bridge changed config: %v", err)
	}
}

func TestDesktopProfileRefusalPreservesNativeHandoffButDisablesWriters(t *testing.T) {
	called := false
	callback := func(context.Context) { called = true }
	handoff := &options.SingleInstanceLock{UniqueId: "profile-test"}
	config := &options.App{
		Bind: []interface{}{NewApp()}, SingleInstanceLock: handoff,
		OnStartup: callback, OnShutdown: callback, OnDomReady: callback,
		OnBeforeClose: func(context.Context) bool { called = true; return false },
	}
	refuseDesktopProfileStartup(config)
	if config.Bind != nil || config.OnStartup != nil || config.OnShutdown != nil || config.OnBeforeClose != nil {
		t.Fatal("rejected profile still exposes initialization, bindings or shutdown writers")
	}
	if config.SingleInstanceLock != handoff || config.OnDomReady == nil || called {
		t.Fatal("refusal changed handoff or ran initialization")
	}
}

func TestDesktopProfileOwnerRejectsRealWailsBeforeWriters(t *testing.T) {
	binary := os.Getenv("REASONIX_WAILS_PROFILE_TEST_BIN")
	if binary == "" {
		t.Skip("set REASONIX_WAILS_PROFILE_TEST_BIN to exercise the native Wails refusal path")
	}
	home, state := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", state)
	configPath := filepath.Join(home, "config.toml")
	original := []byte("default_model = \"refused-native-test\"\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := profilegate.TryAcquireDesktop(home, state)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary)
	// Exercise this process's DOM-ready refusal even if an installed Wails
	// instance is running; native single-instance handoff would exit earlier.
	command.Env = append(os.Environ(), "REASONIX_DEV=1")
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil || !strings.Contains(string(output), "Close the other Reasonix process") {
		t.Fatalf("native Wails refusal did not exit: %v\n%s", err, output)
	}
	if content, err := os.ReadFile(configPath); err != nil || string(content) != string(original) {
		t.Fatalf("rejected Wails changed config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "sessions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected Wails initialized sessions: %v", err)
	}
}
