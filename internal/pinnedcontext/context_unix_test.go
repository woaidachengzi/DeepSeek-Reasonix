//go:build !windows && !plan9

package pinnedcontext

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"reasonix/internal/store"
)

func TestLoadStateRejectsFIFOWithoutBlocking(t *testing.T) {
	session := filepath.Join(t.TempDir(), "owned.jsonl")
	if err := unix.Mkfifo(store.SessionPinnedContext(session), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := LoadState(session); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("FIFO was not rejected as a non-regular manifest: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manifest reader blocked on FIFO")
	}
}
