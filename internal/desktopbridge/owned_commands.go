package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

var ErrOwnedRuntimeChanged = errors.New("desktop bridge command owner is no longer current")

// OwnedCommandScope captures a local Preview owner, not authority to open,
// restore, rebuild or switch a session. Manager and Controller epochs differ.
type OwnedCommandScope struct {
	SessionID    string
	OwnerEpoch   uint64
	SessionPath  string
	RuntimeEpoch string
}

type OwnedPrompt struct {
	ID           string
	Kind         string
	TurnID       string
	RuntimeEpoch string // optional prompt-routing stamp, not Controller epoch
}

type OwnedCommandState struct {
	Revision        uint64
	Phase           string
	Running         bool
	TurnID          string
	PendingPrompt   bool
	CancelRequested bool
	Pending         []OwnedPrompt
}

type OwnedCommandView struct {
	Scope             OwnedCommandScope
	Session           SessionView
	State             OwnedCommandState
	LocalInputVersion uint64 // host-only local reclaim fence, never a wire epoch
}

// RuntimeOwnedCommands implements the core's atomic scoped admission and
// exact specialized prompt resolution. Implementations must not perform a
// blocking model turn, call the manager, or acquire another runtime owner.
type RuntimeOwnedCommands interface {
	CommandSnapshot() (runtimeEpoch string, state OwnedCommandState)
	SubmitOwnedContext(context.Context, OwnedCommandView, string) error
	ResolveOwnedContext(context.Context, OwnedCommandScope, OwnedPrompt, json.RawMessage) error
}

// RuntimeOwnedPromptReader reads one private display snapshot without emitting
// into the UI/event bus. A captured exact prompt is not permission to resolve it.
type RuntimeOwnedPromptReader interface {
	ReadOwnedPrompt(context.Context, OwnedCommandScope, OwnedPrompt) (json.RawMessage, error)
}

func (m *RuntimeManager) ReadOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt) (json.RawMessage, error) {
	if prompt.ID == "" || prompt.TurnID == "" {
		return nil, ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.commandOwnerLocked(ctx, scope)
	if err != nil {
		return nil, err
	}
	reader, ok := provider.(RuntimeOwnedPromptReader)
	if !ok {
		return nil, ErrOwnedRuntimeChanged
	}
	_, state := provider.CommandSnapshot()
	found := false
	for _, current := range state.Pending {
		if current == prompt {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrOwnedRuntimeChanged
	}
	payload, err := reader.ReadOwnedPrompt(ctx, scope, prompt)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || len(payload) == 0 || len(payload) > 64<<10 {
		return nil, ErrOwnedRuntimeChanged
	}
	return append(json.RawMessage(nil), payload...), nil
}

// CommandSnapshot observes only the currently owned local runtime. It does not
// enumerate saved files, remote clients, Wails controllers or guessed IDs.
func (m *RuntimeManager) CommandSnapshot() (OwnedCommandView, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.opening || m.runtime == nil || m.runtime.State() == "deleting" {
		return OwnedCommandView{}, false
	}
	provider, ok := m.runtime.(RuntimeOwnedCommands)
	if !ok {
		return OwnedCommandView{}, false
	}
	epoch, state := provider.CommandSnapshot()
	if epoch == "" || state.Revision == 0 || state.Phase == "closed" {
		return OwnedCommandView{}, false
	}
	state.Pending = append([]OwnedPrompt(nil), state.Pending...)
	session := m.view
	session.State = m.runtime.State()
	return OwnedCommandView{Scope: OwnedCommandScope{session.ID, m.ownerEpoch, session.Path, epoch}, Session: session, State: state, LocalInputVersion: m.localInputVersion}, true
}

// Caller holds m.mu through the core's bounded admission/decision boundary.
// A callback retained outside this lock could dispatch to a replaced owner.
func (m *RuntimeManager) commandOwnerLocked(ctx context.Context, scope OwnedCommandScope) (RuntimeOwnedCommands, error) {
	if ctx == nil || ctx.Err() != nil || m.closed || m.opening || m.runtime == nil ||
		scope.SessionID == "" || scope.OwnerEpoch == 0 || scope.RuntimeEpoch == "" ||
		scope.SessionPath == "" || m.view.ID != scope.SessionID || m.view.Path != scope.SessionPath || m.ownerEpoch != scope.OwnerEpoch ||
		m.runtime.State() == "deleting" || m.runtime.State() == "deleted" {
		return nil, ErrOwnedRuntimeChanged
	}
	provider, ok := m.runtime.(RuntimeOwnedCommands)
	if !ok {
		return nil, ErrOwnedRuntimeChanged
	}
	epoch, _ := provider.CommandSnapshot()
	if epoch != scope.RuntimeEpoch || ctx.Err() != nil {
		return nil, ErrOwnedRuntimeChanged
	}
	return provider, nil
}

// SubmitOwned preserves the captured idle revision. It never resamples a new
// revision to make a stale input succeed, interprets commands, or retries.
func (m *RuntimeManager) SubmitOwned(ctx context.Context, view OwnedCommandView, input string) error {
	if strings.TrimSpace(input) == "" || view.State.Revision == 0 {
		return ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.commandOwnerLocked(ctx, view.Scope)
	if err != nil {
		return err
	}
	if view.LocalInputVersion != m.localInputVersion {
		return ErrOwnedRuntimeChanged
	}
	stale, err := settingsStale(m.runtime)
	if err != nil {
		return ErrSessionSettingsApply
	}
	if stale || ctx.Err() != nil {
		return ErrOwnedRuntimeChanged // an explicit desktop refresh must rebind
	}
	view.State.Pending = append([]OwnedPrompt(nil), view.State.Pending...)
	return provider.SubmitOwnedContext(ctx, view, input)
}

// ResolveOwnedPrompt cannot use ID-only compatibility resolvers. The core
// validates the five-kind answer union and both independent epoch identities.
func (m *RuntimeManager) ResolveOwnedPrompt(ctx context.Context, scope OwnedCommandScope, prompt OwnedPrompt, answer json.RawMessage) error {
	if prompt.ID == "" || prompt.TurnID == "" || len(answer) == 0 || len(answer) > 64<<10 {
		return ErrInvalidInput
	}
	answer = append(json.RawMessage(nil), answer...)
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.commandOwnerLocked(ctx, scope)
	if err != nil {
		return err
	}
	return provider.ResolveOwnedContext(ctx, scope, prompt, answer)
}
