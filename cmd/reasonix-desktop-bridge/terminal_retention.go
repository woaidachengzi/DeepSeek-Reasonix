package main

import (
	"sync"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopterminal"
)

var _ desktopbridge.RuntimeTerminalRetention = (*controllerRuntime)(nil)

type retainedTerminals struct {
	mu       sync.Mutex
	source   *controllerRuntime
	manager  *desktopterminal.Manager
	consumed bool
	closeErr error
}

func (r *controllerRuntime) RetainTerminals() (desktopbridge.TerminalRetention, error) {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	if r.terminalsClosed || r.deleted.Load() || r.deleting.Load() || r.controller == nil {
		return nil, desktopbridge.ErrTerminalUnavailable
	}
	if r.terminalCreating != 0 {
		return nil, desktopbridge.ErrTerminalBusy
	}
	lease := &retainedTerminals{source: r, manager: r.terminals}
	r.terminals = nil
	r.terminalsClosed = true
	return lease, nil
}

func (lease *retainedTerminals) Attach(runtime desktopbridge.Runtime) error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return desktopbridge.ErrTerminalUnavailable
	}
	next, ok := runtime.(*controllerRuntime)
	if !ok || next == nil || next.controller == nil || next.sessionID != lease.source.sessionID ||
		next.controller.SessionPath() != lease.source.controller.SessionPath() ||
		next.terminalEvents != lease.source.terminalEvents {
		return desktopbridge.ErrTerminalUnavailable
	}
	if lease.manager != nil && !lease.manager.MatchesOwner(next.controller.WorkspaceRoot(), next.sessionID, next.terminalEvents) {
		return desktopbridge.ErrTerminalUnavailable
	}
	next.terminalMu.Lock()
	defer next.terminalMu.Unlock()
	if next.deleted.Load() || next.deleting.Load() || next.terminals != nil || next.terminalCreating != 0 ||
		(next.terminalsClosed && next != lease.source) {
		return desktopbridge.ErrTerminalUnavailable
	}
	next.terminals, next.terminalsClosed = lease.manager, false
	lease.manager = nil
	lease.consumed = true
	return nil
}

func (lease *retainedTerminals) Close() error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return lease.closeErr
	}
	lease.consumed = true
	manager := lease.manager
	lease.manager = nil
	if manager != nil {
		lease.closeErr = manager.Close()
	}
	return lease.closeErr
}
