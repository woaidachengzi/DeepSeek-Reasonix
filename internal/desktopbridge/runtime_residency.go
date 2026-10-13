package desktopbridge

import "errors"

const DefaultResidentSessions = 16

var ErrRuntimeCapacity = errors.New("desktop bridge live session capacity reached")

// RuntimeResidencyProvider protects non-turn work, such as background jobs,
// terminals and authentication flows, from idle-cache eviction.
type RuntimeResidencyProvider interface{ KeepRuntimeResident() bool }

// tryRetireIdle closes admission before releasing resources. A submit arriving
// concurrently either wins this lock and marks the slot busy, or is rejected;
// it can never be accepted by an owner already selected for eviction.
func (m *RuntimeManager) tryRetireIdle() (bool, error) {
	m.mu.Lock()
	if m.closed || m.opening || m.runtime == nil || m.runtime.State() != "idle" || m.terminalCreates != 0 || m.pendingTerminals != nil {
		m.mu.Unlock()
		return false, nil
	}
	if retained, ok := m.runtime.(RuntimeResidencyProvider); ok && retained.KeepRuntimeResident() {
		m.mu.Unlock()
		return false, nil
	}
	m.closed = true
	m.retireLocalReclaimLocked()
	runtime := m.runtime
	m.runtime = nil
	m.view = SessionView{}
	m.mu.Unlock()
	return true, runtime.Shutdown()
}
