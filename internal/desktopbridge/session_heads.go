package desktopbridge

import (
	"fmt"
	"strings"
)

type SessionHeadView struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name,omitempty"`
	ParentID     string `json:"parentId,omitempty"`
	Preview      string `json:"preview,omitempty"`
	MessageCount int    `json:"messageCount"`
	Selected     bool   `json:"selected"`
}

type RuntimeSessionHeadProvider interface {
	SessionHeads() ([]SessionHeadView, error)
	SwitchSessionHead(headID string) error
}

func (m *RuntimeManager) SessionHeads(sessionID string) ([]SessionHeadView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return nil, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeSessionHeadProvider)
	if !ok {
		return nil, fmt.Errorf("%w: session heads are unavailable", ErrSessionConflict)
	}
	return provider.SessionHeads()
}

func (m *RuntimeManager) SwitchSessionHead(sessionID, headID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeSessionHeadProvider)
	if !ok {
		return fmt.Errorf("%w: session heads are unavailable", ErrSessionConflict)
	}
	if err := provider.SwitchSessionHead(strings.TrimSpace(headID)); err != nil {
		return err
	}
	if m.runtime.SessionPath() != m.view.Path {
		return fmt.Errorf("%w: session head switch changed the transcript path", ErrSessionConflict)
	}
	return nil
}
