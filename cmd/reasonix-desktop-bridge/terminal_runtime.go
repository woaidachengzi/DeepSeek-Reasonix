package main

import (
	"context"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopterminal"
)

var _ desktopbridge.RuntimeTerminalProvider = (*controllerRuntime)(nil)

// Caller holds terminalMu through each bounded operation. A retained manager
// can never be reached by a delayed old-controller action after transfer.
func (r *controllerRuntime) terminalOwnerLocked() (*desktopterminal.Manager, error) {
	if r.terminalsClosed || r.deleted.Load() || r.deleting.Load() || r.controller == nil {
		return nil, desktopbridge.ErrTerminalUnavailable
	}
	if r.terminals == nil {
		manager, err := desktopterminal.New(r.controller.WorkspaceRoot(), r.sessionID, r.terminalEvents)
		if err != nil {
			return nil, err
		}
		r.terminals = manager
	}
	return r.terminals, nil
}

func (r *controllerRuntime) closeTerminals() error {
	r.terminalMu.Lock()
	r.terminalsClosed = true
	manager := r.terminals
	r.terminalMu.Unlock()
	if manager != nil {
		return manager.Close()
	}
	return nil
}

func (r *controllerRuntime) TerminalWorkspace() (desktopbridge.TerminalWorkspaceView, error) {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return desktopbridge.TerminalWorkspaceView{}, err
	}
	return manager.Workspace()
}

func (r *controllerRuntime) CreateTerminal(ctx context.Context, path, shellID string) (desktopbridge.TerminalSessionView, error) {
	r.terminalMu.Lock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		r.terminalMu.Unlock()
		return desktopbridge.TerminalSessionView{}, err
	}
	r.terminalCreating++
	r.terminalMu.Unlock()
	defer func() { r.terminalMu.Lock(); r.terminalCreating--; r.terminalMu.Unlock() }()
	return manager.Create(ctx, path, shellID)
}

func (r *controllerRuntime) WriteTerminal(id string, data []byte) error {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return err
	}
	return manager.Write(id, data)
}

func (r *controllerRuntime) ResizeTerminal(id string, columns, rows int) error {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return err
	}
	return manager.Resize(id, columns, rows)
}

func (r *controllerRuntime) RenameTerminal(id, title string) error {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return err
	}
	return manager.Rename(id, title)
}

func (r *controllerRuntime) CloseTerminal(id string) error {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return err
	}
	return manager.CloseTerminal(id)
}

func (r *controllerRuntime) TerminalOutput(id string) (desktopbridge.TerminalOutputView, error) {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	manager, err := r.terminalOwnerLocked()
	if err != nil {
		return desktopbridge.TerminalOutputView{}, err
	}
	return manager.Output(id)
}
