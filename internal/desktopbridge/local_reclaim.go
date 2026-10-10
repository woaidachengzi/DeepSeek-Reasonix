package desktopbridge

import "context"

// A one-shot observation of the actual manager local-input fence, not a grant
// or input replay. Retired separately fences SDK IO after the signal is read.
type OwnedLocalReclaimObservation struct {
	manager              *RuntimeManager
	scope                OwnedCommandScope
	version              uint64
	done, retired        chan struct{}
	reclaimed, signalled bool // manager.mu
	stop                 func() bool
}

func (m *RuntimeManager) ObserveLocalReclaim(ctx context.Context, view OwnedCommandView) (*OwnedLocalReclaimObservation, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrOwnedRuntimeChanged
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.commandOwnerLocked(ctx, view.Scope); err != nil || view.LocalInputVersion != m.localInputVersion || len(m.localReclaims) >= 8 {
		return nil, ErrOwnedRuntimeChanged
	}
	o := &OwnedLocalReclaimObservation{manager: m, scope: view.Scope, version: view.LocalInputVersion, done: make(chan struct{}), retired: make(chan struct{})}
	if m.localReclaims == nil {
		m.localReclaims = make(map[*OwnedLocalReclaimObservation]struct{})
	}
	m.localReclaims[o] = struct{}{}
	o.stop = context.AfterFunc(ctx, o.Close)
	return o, nil
}
func (o *OwnedLocalReclaimObservation) Done() <-chan struct{}    { return o.done }
func (o *OwnedLocalReclaimObservation) Retired() <-chan struct{} { return o.retired }
func (o *OwnedLocalReclaimObservation) Reclaimed() bool {
	o.manager.mu.Lock()
	defer o.manager.mu.Unlock()
	return o.reclaimed
}
func (o *OwnedLocalReclaimObservation) Close() {
	o.manager.mu.Lock()
	defer o.manager.mu.Unlock()
	o.manager.endLocalReclaimLocked(o)
}
func (m *RuntimeManager) endLocalReclaimLocked(o *OwnedLocalReclaimObservation) {
	if _, exists := m.localReclaims[o]; !exists {
		return
	}
	delete(m.localReclaims, o)
	if o.stop != nil {
		o.stop()
	}
	if !o.signalled {
		close(o.done)
		o.signalled = true
	}
	close(o.retired)
}
func (m *RuntimeManager) retireLocalReclaimLocked() {
	for o := range m.localReclaims {
		m.endLocalReclaimLocked(o)
	}
}
func (m *RuntimeManager) signalLocalReclaimLocked() {
	for o := range m.localReclaims {
		if !o.signalled && o.scope.OwnerEpoch == m.ownerEpoch && o.scope.SessionID == m.view.ID && o.scope.SessionPath == m.view.Path && o.version != m.localInputVersion {
			o.reclaimed = true
			o.signalled = true
			close(o.done)
		}
	}
}
