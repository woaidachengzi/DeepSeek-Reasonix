package desktopbridge

// RuntimeArchiveLifecycle flushes the idle session before a reversible archive
// commit, then releases ownership without another write that could fail after
// the commit. It does not remove any session artifacts.
type RuntimeArchiveLifecycle interface {
	PrepareArchive() error
	ReleaseArchived()
}

// ChangeArchive serializes admission, submit, open and shutdown with archive
// persistence. Even an inactive target cannot be archived during active work.
type RuntimeArchiveReadiness interface{ CheckArchiveIdle() error }

func (m *RuntimeManager) ChangeArchive(sessionID string, archived bool, commit func(bool) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if m.opening {
		return ErrOpenInProgress
	}
	if m.runtime != nil && m.runtime.State() != "idle" {
		return ErrSessionConflict
	}
	if readiness, ok := m.runtime.(RuntimeArchiveReadiness); ok {
		if err := readiness.CheckArchiveIdle(); err != nil {
			return err
		}
	}
	var lifecycle RuntimeArchiveLifecycle
	if archived && m.runtime != nil && m.view.ID == sessionID {
		var ok bool
		lifecycle, ok = m.runtime.(RuntimeArchiveLifecycle)
		if !ok {
			return ErrSessionConflict
		}
		if err := lifecycle.PrepareArchive(); err != nil {
			return err
		}
	}
	if err := commit(m.runtime != nil && m.view.ID == sessionID); err != nil {
		return err
	}
	if lifecycle != nil {
		lifecycle.ReleaseArchived()
		m.runtime = nil
		m.view = SessionView{}
	}
	return nil
}
