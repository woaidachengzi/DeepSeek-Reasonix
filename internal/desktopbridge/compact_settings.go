package desktopbridge

type CompactRatioProvider interface {
	SetCompactRatio(float64) bool
}

// SetCompactRatio updates only the exact resident owner, including while it is
// running. The provider must publish the setting without replacing its runtime.
func (m *RuntimeManager) SetCompactRatio(sessionID string, ratio float64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.opening || m.runtime == nil || m.view.ID != sessionID || !(ratio > 0 && ratio < 1) {
		return false
	}
	provider, ok := m.runtime.(CompactRatioProvider)
	return ok && provider.SetCompactRatio(ratio)
}
