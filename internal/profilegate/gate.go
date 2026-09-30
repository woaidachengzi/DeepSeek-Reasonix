// Package profilegate coordinates lock-aware writers and offline maintenance
// for one Reasonix state profile. Older binaries do not participate in this
// protocol and must be stopped separately before a consistent snapshot.
package profilegate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"reasonix/internal/filelock"
	"reasonix/internal/pathidentity"
)

var ErrHeld = errors.New("session profile is already owned by another lock-aware process")

const lockName = ".reasonix-session-profile.lock"

// TryAcquireDesktop owns every mutable desktop profile root: the config home
// as well as an independently selected state home. Canonical aliases are
// deduplicated. A failed acquisition releases earlier roots, and release is
// idempotent so shutdown and startup failure can share cleanup.
func TryAcquireDesktop(configRoot, stateRoot string) (func(), error) {
	roots := make(map[string]struct{}, 2)
	for _, root := range []string{configRoot, stateRoot} {
		if strings.TrimSpace(root) == "" {
			return nil, errors.New("desktop config or state profile root is unavailable")
		}
		roots[pathidentity.Canonical(root)] = struct{}{}
	}
	ordered := make([]string, 0, len(roots))
	for root := range roots {
		ordered = append(ordered, root)
	}
	sort.Strings(ordered)
	var releases []func()
	var ownedRoots []string
	var once sync.Once
	release := func() {
		once.Do(func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		})
	}
	for _, root := range ordered {
		// Canonical path spelling can retain case on an insensitive volume.
		// Check after each acquisition so aliases of newly created roots also
		// share the lock already held by this call.
		duplicate := false
		if info, err := os.Stat(root); err == nil {
			for _, ownedRoot := range ownedRoots {
				if ownedInfo, err := os.Stat(ownedRoot); err == nil && os.SameFile(info, ownedInfo) {
					duplicate = true
					break
				}
			}
		}
		if duplicate {
			continue
		}
		unlock, err := TryAcquire(root)
		if err != nil {
			release()
			return nil, fmt.Errorf("desktop profile ownership: %w", err)
		}
		releases = append(releases, unlock)
		ownedRoots = append(ownedRoots, root)
	}
	return release, nil
}

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
