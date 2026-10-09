package control

import (
	"context"
	"encoding/json"
	"errors"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
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
	if !c.promptScopeCurrentLocked(ctx, scope) {
		return ErrPromptResolveScope
	}
	// promptResolveMu remains held through check, durable transition, and wake.
	// Exact resolver separately verifies the immutable prompt routing identity.
	return c.resolvePromptExactLocked(identity, answer)
}

// Caller holds promptResolveMu. Read and decision use the same current-owner
// check; a display snapshot never refreshes or acquires any runtime lease.
func (c *Controller) promptScopeCurrentLocked(ctx context.Context, scope PromptResolveScope) bool {
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
	return valid && ctx.Err() == nil
}

// ReadPromptScopedContext returns one private wire snapshot without replay,
// routing rebinding, emission, durable transition, or resolver callbacks.
// Serialization under the leaf approval lock gives the caller independent data.
func (c *Controller) ReadPromptScopedContext(ctx context.Context, scope PromptResolveScope, identity PromptIdentity) (json.RawMessage, error) {
	if c == nil || ctx == nil || ctx.Err() != nil || scope.SessionPath == "" || scope.RuntimeEpoch == "" || scope.TurnID == "" || identity.PromptID == "" || identity.TurnID != scope.TurnID {
		return nil, ErrPromptResolveScope
	}
	c.promptResolveMu.Lock()
	defer c.promptResolveMu.Unlock()
	if !c.promptScopeCurrentLocked(ctx, scope) {
		return nil, ErrPromptResolveScope
	}
	pending, ok := c.promptOwner.Prompt(identity.PromptID)
	if !ok || pending.Identity != identity || pending.State != PromptPending {
		return nil, ErrPromptNotPending
	}
	c.approval.mu.Lock()
	defer c.approval.mu.Unlock()
	e := event.Event{TurnID: identity.TurnID, ItemID: identity.PromptID}
	switch identity.Kind {
	case PromptAsk:
		ask, ok := c.approval.asks[identity.PromptID]
		if !ok || ask.queued {
			return nil, ErrPromptNotPending
		}
		e.Kind = event.AskRequest
		e.Ask = event.Ask{ID: identity.PromptID, TurnID: identity.TurnID, Questions: ask.questions}
	case PromptMCP:
		interaction, ok := c.approval.mcpInteractions.pending[identity.PromptID]
		if !ok || interaction.queued {
			return nil, ErrPromptNotPending
		}
		e.Kind = event.MCPInteractionRequest
		e.MCPInteraction = interaction.request
		e.MCPInteraction.TurnID = identity.TurnID
	case PromptApproval, PromptPlan, PromptRecovery:
		approval, ok := c.approval.approvals[identity.PromptID]
		kind := PromptApproval
		if approval.kind == "plan" {
			kind = PromptPlan
		}
		if approval.kind == "recovery" {
			kind = PromptRecovery
		}
		if !ok || kind != identity.Kind {
			return nil, ErrPromptNotPending
		}
		e.Kind = event.ApprovalRequest
		e.Approval = event.Approval{ID: identity.PromptID, TurnID: identity.TurnID, Tool: approval.tool, Subject: approval.subject, Reason: approval.reason, Fresh: approval.fresh, Kind: approval.kind, Recovery: approval.recovery, WriteAccess: event.NormalizeWriteAccessApproval(approval.writeAccess)}
	default:
		return nil, ErrPromptNotPending
	}
	payload, err := json.Marshal(eventwire.ToWire(e))
	if err != nil || len(payload) > 64<<10 || ctx.Err() != nil {
		return nil, ErrPromptResolveScope
	}
	return payload, nil
}
