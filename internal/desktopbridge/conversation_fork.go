package desktopbridge

import "fmt"

// The wire result is shared with detached old-format forks.
type RuntimeConversationForkProvider interface {
	ForkConversation(forkToken string) (LegacyConversationForkResult, error)
}

func (m *RuntimeManager) ForkConversation(sessionID, forkToken string) (LegacyConversationForkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return LegacyConversationForkResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return LegacyConversationForkResult{}, ErrSessionNotFound
	}
	if m.runtime.State() != "idle" {
		return LegacyConversationForkResult{}, fmt.Errorf("%w: wait for the current turn before forking", ErrSessionConflict)
	}
	provider, ok := m.runtime.(RuntimeConversationForkProvider)
	if !ok {
		return LegacyConversationForkResult{}, fmt.Errorf("%w: conversation fork is unavailable", ErrSessionConflict)
	}
	result, err := provider.ForkConversation(forkToken)
	if m.runtime.SessionPath() != m.view.Path {
		return LegacyConversationForkResult{}, fmt.Errorf("%w: conversation fork changed the source path", ErrSessionConflict)
	}
	return result, err
}
