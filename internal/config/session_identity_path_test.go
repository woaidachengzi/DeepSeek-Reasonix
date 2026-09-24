package config

import (
	"path/filepath"
	"testing"
)

func TestDesktopSessionIdentityPathUsesStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_STATE_HOME", stateHome)
	if got, want := DesktopSessionIdentityPath(), filepath.Join(stateHome, "desktop", "session-state-v1.sqlite"); got != want {
		t.Fatalf("identity path = %q, want %q", got, want)
	}
	if got := SessionProfileRoot(); got != stateHome {
		t.Fatalf("session profile root = %q, want %q", got, stateHome)
	}
}
