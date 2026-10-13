package desktopbridge

// SessionSnapshot never changes the selected controller or restores history.
func (m *RuntimeManager) SessionSnapshot(sessionID string) (SessionView, bool) {
	view, ok := m.Snapshot()
	return view, ok && view.ID == sessionID
}

func (m *RuntimeManager) Snapshots() []SessionView {
	if view, ok := m.Snapshot(); ok {
		return []SessionView{view}
	}
	return nil
}

func (m *RuntimeManager) CommandSnapshots() []OwnedCommandView {
	if view, ok := m.CommandSnapshot(); ok {
		return []OwnedCommandView{view}
	}
	return nil
}
