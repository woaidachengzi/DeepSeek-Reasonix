package desktopbridge

import (
	"fmt"
	"strings"
)

// PinnedFile exposes metadata only; file contents remain in the core.
type PinnedFile struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	TokenEstimate int    `json:"tokenEstimate"`
	Error         string `json:"error,omitempty"`
}

type RuntimePinnedFilesProvider interface {
	PinnedFiles() ([]PinnedFile, error)
	ChangePinnedFile(path string, pin bool) ([]PinnedFile, error)
}

// PinnedFiles uses the same ownership lock as session replacement and submit.
func (m *RuntimeManager) PinnedFiles(sessionID string) ([]PinnedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.pinnedFilesProviderLocked(sessionID)
	if err != nil {
		return nil, err
	}
	return provider.PinnedFiles()
}

func (m *RuntimeManager) ChangePinnedFile(sessionID, path string, pin bool) ([]PinnedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.pinnedFilesProviderLocked(sessionID)
	if err != nil {
		return nil, err
	}
	if m.runtime.State() != "idle" {
		return nil, fmt.Errorf("%w: wait for the current turn before changing pinned files", ErrSessionConflict)
	}
	return provider.ChangePinnedFile(path, pin)
}

func (m *RuntimeManager) pinnedFilesProviderLocked(sessionID string) (RuntimePinnedFilesProvider, error) {
	if m.closed {
		return nil, ErrClosed
	}
	if m.runtime == nil || strings.TrimSpace(sessionID) == "" || m.view.ID != strings.TrimSpace(sessionID) {
		return nil, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimePinnedFilesProvider)
	if !ok {
		return nil, fmt.Errorf("%w: pinned files are unavailable", ErrSessionConflict)
	}
	return provider, nil
}
