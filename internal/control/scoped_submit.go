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
	c.turnEvents.mu.RLock()
	ledger := c.turnEvents.ledger
	matches := c.turnEvents.err == nil && ledger != nil &&
		c.runtimeState.path == c.sessionPath && c.runtimeState.ledger == ledger &&
		c.runtimeState.snapshot.RuntimeEpoch == scope.RuntimeEpoch && c.runtimeState.snapshot.Revision == scope.Revision &&
		c.runtimeState.snapshot.Phase == "idle" && !c.runtimeState.snapshot.Running && !c.runtimeState.snapshot.PendingPrompt
	c.turnEvents.mu.RUnlock()
	if requestCtx.Err() != nil || !matches || c.sessionPath != scope.SessionPath || c.closed || c.rotating || c.running || c.finishing || c.rejectDrainingGenerationLocked() {
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
