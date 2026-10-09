package desktopbridge

import "errors"

// TerminalRetention is an exclusive backend lease across a same-session
// controller replacement. Attach must verify the actual session and workspace
// identity, transfer at most once, and permit restoration to the unclosed
// source after a rejected settings candidate. Close disposes an unattached
// lease; it must not close terminals after a successful transfer.
type TerminalRetention interface {
	Attach(Runtime) error
	Close() error
}

type RuntimeTerminalRetention interface {
	RetainTerminals() (TerminalRetention, error)
}

type retainedRuntimeTerminals struct{ TerminalRetention }

// Attachment and controller publication are atomic against Shutdown. During
// the build m.pendingTerminals remains owned by the manager, so exit closes
// terminals immediately even when a factory is stalled or ignores cancellation.
func (m *RuntimeManager) finishRetainedOpen(runtime Runtime, view SessionView, pending *retainedRuntimeTerminals) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		m.opening = false
		return ErrClosed
	}
	if pending != nil {
		if err := pending.Attach(runtime); err != nil {
			return err
		}
	}
	m.pendingTerminals = nil
	m.opening = false
	m.ownerEpoch++
	m.runtime, m.view = runtime, view
	m.activateOwnedEventsLocked()
	return nil
}

// Caller holds m.mu. Creation admission extends through the ownership check,
// not just OS Start: a late create must never be transferred and then closed
// (or retried into a duplicate) as an unacknowledged old-generation result.
func (m *RuntimeManager) retainTerminalsLocked(previous Runtime) (TerminalRetention, error) {
	if m.terminalCreates != 0 {
		return nil, ErrTerminalBusy
	}
	if provider, ok := previous.(RuntimeTerminalRetention); ok {
		return provider.RetainTerminals()
	}
	return nil, nil
}

// Settings/effort candidates are already built and validated, while previous
// remains live. Failed attachment restores the old terminal gate before the
// candidate is released. The manager must publish only after success.
func (m *RuntimeManager) transferTerminalsLocked(previous, next Runtime) error {
	retained, err := m.retainTerminalsLocked(previous)
	if err != nil || retained == nil {
		return err
	}
	if err := retained.Attach(next); err != nil {
		restoreErr := retained.Attach(previous)
		return errors.Join(err, restoreErr, retained.Close())
	}
	return retained.Close()
}
