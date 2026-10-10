package control

import (
	"context"
	"errors"
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/extension"
)

// TurnSubmitScope fences an already-owned idle Controller observation. It is
// not permission to restore, acquire a lease, or switch the foreground. The
// revision prevents an old idle page from sending after intervening turns.
type TurnSubmitScope struct {
	SessionPath  string
	RuntimeEpoch string
	Revision     uint64
}

var ErrTurnSubmitScope = errors.New("turn submission scope is no longer current")

// SubmitScopedContext accepts user text, never shell/management commands, with
// references resolved only inside this Controller's workspace. A nil result
// means admission was reserved, not that the provider or durable save succeeded.
// The transport context fences admission only; disconnecting after acceptance
// does not cancel the accepted user turn. Callers must not automatically retry.
func (c *Controller) SubmitScopedContext(ctx context.Context, scope TurnSubmitScope, input string) error {
	if strings.TrimSpace(input) == "" {
		return ErrTurnSubmitScope
	}
	return c.admitScopedTurn(ctx, scope, func(turnCtx context.Context) error {
		return c.runRefTurnWithResolverSync(turnCtx, input, input, "", "", c.ResolveScopedRefs)
	})
}

func (c *Controller) admitScopedTurn(requestCtx context.Context, scope TurnSubmitScope, body func(context.Context) error) error {
	return c.admitScopedTurnGuarded(requestCtx, scope, body, nil)
}

// guard runs only under the exact same runtime/admission locks as reservation.
// A non-nil guard is private to the driving API, never a caller-provided hook.
func (c *Controller) admitScopedTurnGuarded(requestCtx context.Context, scope TurnSubmitScope, body func(context.Context) error, guard func() bool) error {
	if c == nil || requestCtx == nil || requestCtx.Err() != nil || scope.SessionPath == "" || scope.RuntimeEpoch == "" || scope.Revision == 0 || body == nil {
		return ErrTurnSubmitScope
	}
	if err := c.ensureWriteAuthorityReady(); err != nil {
		return ErrTurnSubmitScope
	}
	// Match runtime sampling's order. Reserve running under the same admission
	// lock as the identity comparison; never validate then call Submit separately.
	c.runtimeState.mu.Lock()
	c.mu.Lock()
	if guard == nil && requestCtx.Err() == nil && c.matchesDrivingOwnerLocked(scope) {
		c.revokeDesktopDrivingLocked()
	}
	matches := c.matchesScopedIdleLocked(scope)
	if requestCtx.Err() != nil || !matches || c.sessionPath != scope.SessionPath || c.closed || c.rotating || c.running || c.finishing || c.rejectDrainingGenerationLocked() {
		c.mu.Unlock()
		c.runtimeState.mu.Unlock()
		return ErrTurnSubmitScope
	}
	if guard != nil && !guard() {
		c.mu.Unlock()
		c.runtimeState.mu.Unlock()
		return ErrTurnSubmitScope
	}
	turnCtx, cancel := context.WithCancel(extension.ContextWithRuntimeOwner(context.Background(), c.runtimeOwner))
	c.cancel = cancel
	c.running = true
	c.canceling = false
	c.mu.Unlock()
	c.runtimeState.mu.Unlock()
	c.refreshRuntimeState(event.Event{})
	c.spawnGuardedTurn(turnCtx, cancel, body)
	return nil
}
