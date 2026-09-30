package desktopbridge

import (
	"fmt"
	"strings"
)

type WorkspaceConversationRewindPlan struct {
	PlanID          string `json:"planId,omitempty"`
	Turn            int    `json:"turn"`
	CanConversation bool   `json:"canConversation"`
	DisabledReason  string `json:"disabledReason,omitempty"`
}

type WorkspaceConversationRewindResult struct {
	OK                 bool   `json:"ok"`
	ConversationForked bool   `json:"conversationForked"`
	HeadID             string `json:"headId,omitempty"`
	Error              string `json:"error,omitempty"`
}

type RuntimeConversationRewindProvider interface {
	PrepareConversationRewind(turn int) (WorkspaceConversationRewindPlan, error)
	CommitConversationRewind(planID string) (WorkspaceConversationRewindResult, error)
	UndoConversationRewind(headID string) (WorkspaceConversationRewindResult, error)
}

func (m *RuntimeManager) PrepareConversationRewind(sessionID string, turn int) (WorkspaceConversationRewindPlan, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceConversationRewindPlan{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceConversationRewindPlan{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeConversationRewindProvider)
	if !ok {
		return WorkspaceConversationRewindPlan{}, fmt.Errorf("%w: conversation rewind is unavailable", ErrSessionConflict)
	}
	return provider.PrepareConversationRewind(turn)
}

func (m *RuntimeManager) CommitConversationRewind(sessionID, planID string) (WorkspaceConversationRewindResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceConversationRewindResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceConversationRewindResult{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeConversationRewindProvider)
	if !ok {
		return WorkspaceConversationRewindResult{}, fmt.Errorf("%w: conversation rewind is unavailable", ErrSessionConflict)
	}
	result, err := provider.CommitConversationRewind(planID)
	if err != nil {
		return result, err
	}
	if m.runtime.SessionPath() != m.view.Path {
		return WorkspaceConversationRewindResult{}, fmt.Errorf("%w: conversation rewind changed the transcript path", ErrSessionConflict)
	}
	return result, nil
}

func (m *RuntimeManager) UndoConversationRewind(sessionID, headID string) (WorkspaceConversationRewindResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceConversationRewindResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceConversationRewindResult{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeConversationRewindProvider)
	if !ok {
		return WorkspaceConversationRewindResult{}, fmt.Errorf("%w: conversation rewind is unavailable", ErrSessionConflict)
	}
	return provider.UndoConversationRewind(headID)
}
