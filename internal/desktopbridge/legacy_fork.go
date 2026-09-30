package desktopbridge

import (
	"fmt"
	"strings"
)

type LegacyConversationForkResult struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"sessionId,omitempty"`
	Error     string `json:"error,omitempty"`
}

type RuntimeLegacyForkProvider interface {
	PrepareLegacyConversationFork(turn int) (WorkspaceConversationRewindPlan, error)
	CommitLegacyConversationFork(planID string) (LegacyConversationForkResult, error)
}

func (m *RuntimeManager) PrepareLegacyConversationFork(sessionID string, turn int) (WorkspaceConversationRewindPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceConversationRewindPlan{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceConversationRewindPlan{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeLegacyForkProvider)
	if !ok {
		return WorkspaceConversationRewindPlan{}, fmt.Errorf("%w: legacy conversation fork is unavailable", ErrSessionConflict)
	}
	return provider.PrepareLegacyConversationFork(turn)
}

func (m *RuntimeManager) CommitLegacyConversationFork(sessionID, planID string) (LegacyConversationForkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return LegacyConversationForkResult{}, ErrClosed
	}
	if m.runtime == nil || strings.TrimSpace(sessionID) == "" || m.view.ID != sessionID {
		return LegacyConversationForkResult{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeLegacyForkProvider)
	if !ok {
		return LegacyConversationForkResult{}, fmt.Errorf("%w: legacy conversation fork is unavailable", ErrSessionConflict)
	}
	result, err := provider.CommitLegacyConversationFork(planID)
	if m.runtime.SessionPath() != m.view.Path {
		return LegacyConversationForkResult{}, fmt.Errorf("%w: legacy conversation fork changed the source path", ErrSessionConflict)
	}
	return result, err
}
