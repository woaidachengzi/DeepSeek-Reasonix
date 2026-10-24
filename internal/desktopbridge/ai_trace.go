package desktopbridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AITrace is an explicit content-bearing export, separate from display history
// and the content-free frontend performance recorder. The runtime owns redaction.
type RuntimeAITraceProvider interface {
	AITrace(detailed bool) (json.RawMessage, error)
}

func (m *RuntimeManager) AITrace(sessionID string, detailed bool) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.runtime == nil || strings.TrimSpace(sessionID) == "" || m.view.ID != sessionID {
		return nil, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeAITraceProvider)
	if !ok {
		return nil, fmt.Errorf("%w: AI diagnostic export is unavailable", ErrSessionConflict)
	}
	return provider.AITrace(detailed)
}

func (p *RuntimePool) AITrace(sessionID string, detailed bool) (json.RawMessage, error) {
	slot, err := p.slot(sessionID)
	if err != nil {
		return nil, err
	}
	return slot.AITrace(sessionID, detailed)
}
