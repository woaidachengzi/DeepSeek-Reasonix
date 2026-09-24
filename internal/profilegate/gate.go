// Package profilegate coordinates lock-aware writers and offline maintenance
// for one Reasonix state profile. Older binaries do not participate in this
// protocol and must be stopped separately before a consistent snapshot.
package profilegate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/filelock"
	"reasonix/internal/pathidentity"
)

var ErrHeld = errors.New("session profile is already owned by another lock-aware process")

const lockName = ".reasonix-session-profile.lock"

// TryAcquire obtains exclusive ownership of a state profile. The same gate is
// used by the live bridge and future offline maintenance. A caller must hold
// the returned release function until all its profile writers have stopped.
func TryAcquire(profileRoot string) (func(), error) {
	if strings.TrimSpace(profileRoot) == "" {
		return nil, errors.New("session profile root is empty")
	}
	root := pathidentity.Canonical(profileRoot)
	if filepath.Dir(root) == root {
		return nil, errors.New("filesystem root cannot be a session profile")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create session profile root: %w", err)
	}
	lockPath := filepath.Join(root, lockName)
	if info, err := os.Lstat(lockPath); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("session profile gate is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect session profile gate: %w", err)
	}
	release, err := filelock.TryAcquire(lockPath)
	if errors.Is(err, filelock.ErrHeld) {
		return nil, ErrHeld
	}
	if err != nil {
		return nil, fmt.Errorf("acquire session profile gate: %w", err)
	}
	return release, nil
}
