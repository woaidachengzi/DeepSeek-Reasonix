package desktopbridge

import (
	"context"
	"fmt"
	"strings"
)

type RuntimeEffortProvider interface{ Effort() string }

// SetSessionEffort builds before publication. Failure leaves the existing
// controller, history and grants owned and usable, just like settings refresh.
func (m *RuntimeManager) SetSessionEffort(ctx context.Context, sessionID, model, effort string) (SessionView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return SessionView{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return SessionView{}, ErrSessionNotFound
	}
	if m.opening || m.runtime.State() != "idle" {
		return SessionView{}, ErrSessionConflict
	}
	if strings.TrimSpace(model) != m.view.ModelRef || effort == "" {
		return SessionView{}, ErrInvalidInput
	}
	previous, ok := m.runtime.(SettingsRuntime)
	factory, supported := m.factory.(RuntimeSettingsFactory)
	if !ok || !supported {
		return SessionView{}, fmt.Errorf("reasoning selection unavailable")
	}
	if err := ctx.Err(); err != nil {
		return SessionView{}, err
	}
	request := OpenRequest{SessionID: sessionID, WorkspaceRoot: m.view.WorkspaceRoot, ModelRef: model, Effort: &effort}
	next, err := factory.Rebuild(ctx, previous, request)
	if err != nil {
		return SessionView{}, err
	}
	view, err := runtimeView(next, request)
	if err != nil {
		if next != nil {
			_ = next.ReleaseForReplacement()
		}
		return SessionView{}, err
	}
	m.runtime, m.view = next, view
	_ = previous.ReleaseForReplacement()
	return view, nil
}
