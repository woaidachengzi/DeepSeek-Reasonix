package desktopbridge

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

var (
	ErrTerminalUnavailable = errors.New("integrated terminal is unavailable")
	ErrTerminalNotFound    = errors.New("terminal is not owned by this session")
	ErrTerminalInput       = errors.New("invalid terminal request")
	ErrTerminalBusy        = errors.New("terminal input or session limit reached")
)

const TerminalInputLimit = 64 << 10

type TerminalSessionView struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Shell     string `json:"shell"`
	Cwd       string `json:"cwd"`
	CreatedAt int64  `json:"createdAt"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Running   bool   `json:"running"`
}

type TerminalShellView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type TerminalWorkspaceView struct {
	Available bool                  `json:"available"`
	ReadOnly  bool                  `json:"readOnly"`
	Reason    string                `json:"reason,omitempty"`
	Sessions  []TerminalSessionView `json:"sessions"`
	Shells    []TerminalShellView   `json:"shells"`
}

// Data is base64, preserving binary/escape bytes and split UTF-8 sequences.
// Offsets count raw bytes, not JSON/base64 characters. A gap requires snapshot
// replacement; terminal data is never part of History or a provider prompt.
type TerminalOutputView struct {
	ID    string `json:"id"`
	Data  string `json:"data"`
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

type TerminalExitView struct {
	ID       string `json:"id"`
	ExitCode int    `json:"exitCode"`
	Removed  bool   `json:"removed"`
}

// Each provider belongs to one controller generation and must close its gate
// and processes in Shutdown/Delete unless an exclusive backend lease transfers
// them to a verified replacement for the same session workspace.
// No renderer-owned shell paths or roots.
type RuntimeTerminalProvider interface {
	TerminalWorkspace() (TerminalWorkspaceView, error)
	CreateTerminal(context.Context, string, string) (TerminalSessionView, error)
	WriteTerminal(string, []byte) error
	ResizeTerminal(string, int, int) error
	RenameTerminal(string, string) error
	CloseTerminal(string) error
	TerminalOutput(string) (TerminalOutputView, error)
}

type terminalOwner struct {
	provider RuntimeTerminalProvider
	epoch    uint64
}

func (m *RuntimeManager) terminalOwner(sessionID string) (terminalOwner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return terminalOwner{}, ErrClosed
	}
	if strings.TrimSpace(sessionID) == "" || m.runtime == nil || m.view.ID != strings.TrimSpace(sessionID) || m.opening {
		return terminalOwner{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeTerminalProvider)
	if !ok {
		return terminalOwner{}, ErrTerminalUnavailable
	}
	return terminalOwner{provider, m.ownerEpoch}, nil
}

func (m *RuntimeManager) terminalStillOwned(sessionID string, owner terminalOwner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if m.runtime == nil || m.opening || m.view.ID != strings.TrimSpace(sessionID) || m.ownerEpoch != owner.epoch {
		return ErrSessionNotFound
	}
	return nil
}

// Do not retain the controller ownership lock while starting or closing a PTY.
// Revocation closes the old provider; generation-fenced responses cannot reach
// a replacement, including switch-away-and-back with the same session ID.
func (m *RuntimeManager) TerminalWorkspace(sessionID string) (TerminalWorkspaceView, error) {
	owner, err := m.terminalOwner(sessionID)
	if err != nil {
		return TerminalWorkspaceView{}, err
	}
	view, err := owner.provider.TerminalWorkspace()
	if stale := m.terminalStillOwned(sessionID, owner); stale != nil {
		return TerminalWorkspaceView{}, stale
	}
	return view, err
}

func (m *RuntimeManager) CreateTerminal(ctx context.Context, sessionID, path, shellID string) (TerminalSessionView, error) {
	// Reserve under the same lock as model/settings replacement, including the
	// final response fence. Other terminal operations do not create resources.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return TerminalSessionView{}, ErrClosed
	}
	if m.runtime == nil || m.opening || m.view.ID != strings.TrimSpace(sessionID) {
		m.mu.Unlock()
		return TerminalSessionView{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeTerminalProvider)
	if !ok {
		m.mu.Unlock()
		return TerminalSessionView{}, ErrTerminalUnavailable
	}
	owner := terminalOwner{provider: provider, epoch: m.ownerEpoch}
	m.terminalCreates++
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.terminalCreates--; m.mu.Unlock() }()
	view, err := owner.provider.CreateTerminal(ctx, path, shellID)
	if stale := m.terminalStillOwned(sessionID, owner); stale != nil {
		if err == nil && view.ID != "" {
			_ = owner.provider.CloseTerminal(view.ID)
		}
		return TerminalSessionView{}, stale
	}
	return view, err
}

func (m *RuntimeManager) TerminalOutput(sessionID, terminalID string) (TerminalOutputView, error) {
	owner, err := m.terminalOwner(sessionID)
	if err != nil {
		return TerminalOutputView{}, err
	}
	view, err := owner.provider.TerminalOutput(terminalID)
	if stale := m.terminalStillOwned(sessionID, owner); stale != nil {
		return TerminalOutputView{}, stale
	}
	return view, err
}

func (m *RuntimeManager) terminalAction(sessionID string, action func(RuntimeTerminalProvider) error) error {
	owner, err := m.terminalOwner(sessionID)
	if err != nil {
		return err
	}
	err = action(owner.provider)
	if stale := m.terminalStillOwned(sessionID, owner); stale != nil {
		return stale
	}
	return err
}

func (m *RuntimeManager) WriteTerminal(sessionID, terminalID string, data []byte) error {
	if len(data) == 0 || len(data) > TerminalInputLimit {
		return ErrTerminalInput
	}
	return m.terminalAction(sessionID, func(p RuntimeTerminalProvider) error { return p.WriteTerminal(terminalID, data) })
}

func (m *RuntimeManager) ResizeTerminal(sessionID, terminalID string, columns, rows int) error {
	return m.terminalAction(sessionID, func(p RuntimeTerminalProvider) error { return p.ResizeTerminal(terminalID, columns, rows) })
}

func (m *RuntimeManager) RenameTerminal(sessionID, terminalID, title string) error {
	return m.terminalAction(sessionID, func(p RuntimeTerminalProvider) error { return p.RenameTerminal(terminalID, title) })
}

func (m *RuntimeManager) CloseTerminal(sessionID, terminalID string) error {
	return m.terminalAction(sessionID, func(p RuntimeTerminalProvider) error { return p.CloseTerminal(terminalID) })
}

func (s *EventStream) TerminalOutput(sessionID string, output TerminalOutputView) {
	if output.ID == "" || len(output.ID) > 64 || len(output.Data) > base64.StdEncoding.EncodedLen(8<<10) || output.End < output.Start {
		return
	}
	data, err := base64.StdEncoding.Strict().DecodeString(output.Data)
	if err != nil || len(data) == 0 || len(data) > 8<<10 || output.End-output.Start != uint64(len(data)) {
		return
	}
	s.publishTerminal("terminal_output", sessionID, output)
}

func (s *EventStream) TerminalExit(sessionID string, exit TerminalExitView) {
	if exit.ID == "" || len(exit.ID) > 64 {
		return
	}
	s.publishTerminal("terminal_exit", sessionID, exit)
}
