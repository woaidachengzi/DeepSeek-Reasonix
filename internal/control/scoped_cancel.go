package control

import (
	"context"
	"errors"
)

// TurnCancelScope comes from the owned runtime's committed RuntimeState and
// active projection, not a display label or the local foreground controller.
// RuntimeEpoch is the Controller instance epoch, not optional desktop event
// routing metadata. This scope does not itself grant write authority.
type TurnCancelScope struct {
	SessionPath  string
	RuntimeEpoch string
	TurnID       string
}

var ErrTurnCancelScope = errors.New("turn cancellation scope is no longer current")

// CancelScoped signals exactly the requested live turn. Validation and context
// cancellation share the admission lock, so a stale request cannot cancel a
// replacement context. Unlike Cancel it never stops an idle goal or clears
// another turn's approvals, and it must not be used to adopt a saved session.
func (c *Controller) CancelScoped(scope TurnCancelScope) error {
	return c.CancelScopedContext(context.Background(), scope)
}

// CancelScopedContext also fences a revoked transport operation while it waits
// for admission/runtime locks. The context is not stored as turn authority.
func (c *Controller) CancelScopedContext(ctx context.Context, scope TurnCancelScope) error {
	if c == nil || ctx == nil || ctx.Err() != nil || scope.SessionPath == "" || scope.RuntimeEpoch == "" || scope.TurnID == "" {
		return ErrTurnCancelScope
	}
	c.promptResolveMu.Lock()
	// Match refreshRuntimeState's lock order. Never sample runtimeState while
	// already holding c.mu: its sampler needs that same admission lock.
	c.runtimeState.mu.Lock()
	c.mu.Lock()
	c.turnEvents.mu.RLock()
	ledger := c.turnEvents.ledger
	matches := c.turnEvents.err == nil && ledger != nil && ledger.ActiveTurnID() == scope.TurnID &&
		c.runtimeState.snapshot.RuntimeEpoch == scope.RuntimeEpoch && c.runtimeState.path == c.sessionPath && c.runtimeState.ledger == ledger
	c.turnEvents.mu.RUnlock()
	if ctx.Err() != nil || c.closed || c.rotating || !c.running || c.cancel == nil || c.sessionPath != scope.SessionPath || !matches {
		c.mu.Unlock()
		c.runtimeState.mu.Unlock()
		c.promptResolveMu.Unlock()
		return ErrTurnCancelScope
	}
	cancels := c.promptOwner.takeTurnCancels(scope.TurnID)
	c.canceling = true
	c.cancel()
	c.mu.Unlock()
	c.runtimeState.mu.Unlock()
	for _, cancel := range cancels {
		_ = cancel()
	}
	c.promptResolveMu.Unlock()
	c.finishCancel(scope.TurnID, true)
	return nil
}
