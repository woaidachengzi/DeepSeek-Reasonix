package pinnedcontext

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/fileutil"
	"reasonix/internal/store"
)

// SaveState publishes the desired context atomically without copying file contents.
// The host must hold its session admission gate while changing this manifest.
func SaveState(sessionPath string, files []string) error {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return fmt.Errorf("session is not ready")
	}
	normalized, err := NormalizeFiles(files)
	if err != nil {
		return err
	}
	state := EmptyState(sessionPath)
	state.Files = normalized
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if len(raw) > maxPinnedContextStateBytes {
		return fmt.Errorf("pinned context state exceeds %d bytes", maxPinnedContextStateBytes)
	}
	return fileutil.AtomicWriteFileStrict(store.SessionPinnedContext(sessionPath), raw, 0o600)
}

// Change updates one desired path. Missing existing files remain removable and
// repeated requests are idempotent. Invalid manifests are never overwritten.
// Callers must serialize this operation with turn admission and session changes.
func Change(root, sessionPath, path string, pin bool) ([]Info, error) {
	path, err := NormalizePath(path)
	if err != nil {
		return nil, err
	}
	state, err := LoadState(sessionPath)
	if err != nil {
		return nil, err
	}
	files := slices.Clone(state.Files)
	index := slices.Index(files, path)
	if pin && index < 0 {
		files = append(files, path)
	} else if !pin && index >= 0 {
		files = slices.Delete(files, index, index+1)
	}
	files, err = NormalizeFiles(files)
	if err != nil {
		return nil, err
	}
	build := Build(root, files, nil)
	if pin && index < 0 {
		for _, info := range build.Infos {
			if info.Path == path && info.Error != "" {
				return nil, fmt.Errorf("cannot pin %q: %s", path, info.Error)
			}
		}
	}
	if !slices.Equal(files, state.Files) {
		if err := SaveState(sessionPath, files); err != nil {
			return nil, err
		}
	}
	return build.Infos, nil
}
