package control

import (
	"context"
	"errors"

	"reasonix/internal/event"
)

// PromptResolveScope identifies the already-owned Controller instance and
// live turn. RuntimeEpoch is not PromptIdentity's optional event-routing epoch.
// This scope never restores a session or acquires runtime write authority.
type PromptResolveScope struct {
	SessionPath  string
	RuntimeEpoch string
	TurnID       string
}

var ErrPromptResolveScope = errors.New("prompt resolution scope is no longer current")

// ResolvePromptScopedContext answers one captured prompt, retaining all five
// existing specialized decision paths and their durable receipt semantics.
// Transport cancellation fences admission, not an already committed decision.
// Callers must not automatically retry an uncertain transport result.
func (c *Controller) ResolvePromptScopedContext(ctx context.Context, scope PromptResolveScope, identity PromptIdentity, answer PromptAnswer) error {
	if c == nil || ctx == nil || ctx.Err() != nil || scope.SessionPath == "" || scope.RuntimeEpoch == "" || scope.TurnID == "" || identity.PromptID == "" || identity.TurnID != scope.TurnID {
		return ErrPromptResolveScope
	}
	switch identity.Kind {
	case PromptAsk, PromptApproval, PromptPlan, PromptRecovery, PromptMCP:
	default:
		return ErrPromptResolveScope
	}
	defer c.refreshRuntimeState(event.Event{})
	c.promptResolveMu.Lock()
	defer c.promptResolveMu.Unlock()
	// Match scoped cancellation/runtime sampling lock order. Do not hold the
	// admission/runtime locks across a resolver: durable prompt transitions and
	// explicit skip decisions may themselves publish state or cancel the turn.
	c.runtimeState.mu.Lock()
	c.mu.Lock()
	c.turnEvents.mu.RLock()
	ledger := c.turnEvents.ledger
	matches := c.turnEvents.err == nil && ledger != nil && ledger.ActiveTurnID() == scope.TurnID &&
		c.runtimeState.path == c.sessionPath && c.runtimeState.ledger == ledger &&
		c.runtimeState.snapshot.RuntimeEpoch == scope.RuntimeEpoch
	c.turnEvents.mu.RUnlock()
	valid := ctx.Err() == nil && matches && c.sessionPath == scope.SessionPath && !c.closed && !c.rotating && c.running && !c.canceling
	c.mu.Unlock()
	c.runtimeState.mu.Unlock()
	if !valid || ctx.Err() != nil {
		return ErrPromptResolveScope
	}
	// promptResolveMu remains held through check, durable transition, and wake.
	// Exact resolver separately verifies the immutable prompt routing identity.
	return c.resolvePromptExactLocked(identity, answer)
}
