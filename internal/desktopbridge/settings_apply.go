package desktopbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrSessionSettingsApply leaves the previous runtime owned and usable when
// reading or applying saved model settings fails.
var ErrSessionSettingsApply = errors.New("desktop bridge could not apply saved model settings")

type RuntimeSettingsState interface {
	ModelSettingsState() (applied, desired string, err error)
}

// SettingsRuntime releases a replaced controller without taking a shutdown
// snapshot or ending the logical session shared with its replacement.
type SettingsRuntime interface {
	Runtime
	ReleaseForReplacement() error
}

// Candidates may tentatively write narrowly scoped metadata while building.
// Publication commits that delta; releasing an uncommitted candidate must undo
// it, including a rejected terminal handoff after a successful build.
type RuntimeReplacementCommitter interface{ CommitReplacement() }

func commitReplacement(runtime Runtime) {
	if candidate, ok := runtime.(RuntimeReplacementCommitter); ok {
		candidate.CommitReplacement()
	}
}

// RuntimeSettingsFactory migrates same-session state while keeping previous
// alive on failure. It must not close previous; the manager owns publication.
type RuntimeSettingsFactory interface {
	Rebuild(context.Context, Runtime, OpenRequest) (SettingsRuntime, error)
}

func settingsStale(runtime Runtime) (bool, error) {
	view, ok := runtime.(RuntimeSettingsState)
	if !ok {
		return false, nil
	}
	applied, desired, err := view.ModelSettingsState()
	return applied != desired, err
}

// RebuildSettings applies saved credentials before the next idle turn. The
// ownership lock also fences approvals, session switches and shutdown during
// build/migration. Submit holds this same lock through admission.
func (m *RuntimeManager) RebuildSettings(ctx context.Context, sessionID string) (SessionView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.rebuildSettingsLocked(ctx, strings.TrimSpace(sessionID)); err != nil {
		return SessionView{}, err
	}
	view := m.view
	view.State = m.runtime.State()
	return view, nil
}

func (m *RuntimeManager) rebuildSettingsLocked(ctx context.Context, sessionID string) error {
	if m.closed {
		return ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return ErrSessionNotFound
	}
	if m.opening {
		return ErrOpenInProgress
	}
	state := m.runtime.State()
	if state == "deleting" || state == "deleted" {
		return ErrSessionConflict
	}
	stale, err := settingsStale(m.runtime)
	if err != nil {
		return fmt.Errorf("%w: read saved settings: %v", ErrSessionSettingsApply, err)
	}
	if !stale {
		return nil
	}
	if state != "idle" {
		return fmt.Errorf("%w: active session %q is %s", ErrSessionConflict, sessionID, state)
	}
	previous, ok := m.runtime.(SettingsRuntime)
	factory, supported := m.factory.(RuntimeSettingsFactory)
	if !ok || !supported {
		return fmt.Errorf("%w: runtime does not support settings migration", ErrSessionSettingsApply)
	}
	request := OpenRequest{SessionID: m.view.ID, WorkspaceRoot: m.view.WorkspaceRoot, ModelRef: m.view.ModelRef}
	if request.ModelRef == "" {
		if provider, ok := previous.(RuntimeModelProvider); ok {
			request.ModelRef = strings.TrimSpace(provider.ModelRef())
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrSessionSettingsApply, err)
	}
	if m.terminalCreates != 0 {
		return ErrTerminalBusy
	}
	next, err := factory.Rebuild(ctx, previous, request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionSettingsApply, err)
	}
	view, err := runtimeView(next, request)
	if err != nil {
		if next != nil {
			err = errors.Join(err, next.ReleaseForReplacement())
		}
		return fmt.Errorf("%w: %v", ErrSessionSettingsApply, err)
	}
	if err := m.transferTerminalsLocked(previous, next); err != nil {
		return errors.Join(err, next.ReleaseForReplacement())
	}
	m.ownerEpoch++
	commitReplacement(next)
	m.runtime, m.view = next, view
	// The replacement now owns the live history. Never snapshot the outgoing
	// controller over it or fire SessionEnd for a credentials update.
	_ = previous.ReleaseForReplacement()
	return nil
}
