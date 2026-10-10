package control

import (
	"context"
	"errors"
	"sync"

	"reasonix/internal/event"
	"reasonix/internal/turnevent"
)

var ErrDesktopObservation = errors.New("desktop event observation is closed or requires a fresh owner")

type DesktopEventScope struct{ SessionPath, RuntimeEpoch string }

// Only an approved lifecycle kind, never transcript/error/prompt/credentials.
// Scope is separately captured by the host. This is not replay or a grant.
type DesktopObservedEvent struct{ Kind string }

type controllerDesktopEvents struct {
	mu   sync.Mutex
	subs map[*DesktopEventSubscription]struct{}
}
type DesktopEventSubscription struct {
	owner  *Controller
	ledger *turnevent.Ledger
	scope  DesktopEventScope
	frames chan DesktopObservedEvent
	done   chan struct{}
	ctx    context.Context
	stop   func() bool
}

// ObserveDesktopEvents binds an existing actual Controller/ledger identity.
// It neither reads saved history nor reconstructs an event prefix. Overflow,
// retirement and cancellation reject queued frames, never silently resubscribe.
func (c *Controller) ObserveDesktopEvents(ctx context.Context, scope DesktopEventScope) (*DesktopEventSubscription, error) {
	if c == nil || ctx == nil || ctx.Err() != nil || scope.SessionPath == "" || scope.RuntimeEpoch == "" {
		return nil, ErrDesktopObservation
	}
	_ = c.RuntimeStateSnapshot()
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || !c.matchesDrivingOwnerLocked(TurnSubmitScope{SessionPath: scope.SessionPath, RuntimeEpoch: scope.RuntimeEpoch}) {
		return nil, ErrDesktopObservation
	}
	c.desktopEvents.mu.Lock()
	defer c.desktopEvents.mu.Unlock()
	if len(c.desktopEvents.subs) >= 8 {
		return nil, ErrDesktopObservation
	}
	sub := &DesktopEventSubscription{owner: c, ledger: c.runtimeState.ledger, scope: scope, frames: make(chan DesktopObservedEvent, 32), done: make(chan struct{}), ctx: ctx}
	if c.desktopEvents.subs == nil {
		c.desktopEvents.subs = make(map[*DesktopEventSubscription]struct{})
	}
	c.desktopEvents.subs[sub] = struct{}{}
	sub.stop = context.AfterFunc(ctx, sub.Close)
	return sub, nil
}

func (s *DesktopEventSubscription) Done() <-chan struct{} { return s.done }
func (s *DesktopEventSubscription) Close() {
	p := &s.owner.desktopEvents
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endLocked(s)
}
func (s *DesktopEventSubscription) Read(ctx context.Context) (DesktopObservedEvent, error) {
	if ctx == nil || s.ctx.Err() != nil {
		return DesktopObservedEvent{}, ErrDesktopObservation
	}
	select {
	case <-ctx.Done():
		return DesktopObservedEvent{}, ErrDesktopObservation
	case <-s.done:
		return DesktopObservedEvent{}, ErrDesktopObservation
	default:
	}
	var frame DesktopObservedEvent
	select {
	case <-ctx.Done():
		return frame, ErrDesktopObservation
	case <-s.done:
		return frame, ErrDesktopObservation
	case frame = <-s.frames:
	}
	c := s.owner
	c.runtimeState.mu.Lock()
	c.mu.Lock()
	valid := ctx.Err() == nil && s.ctx.Err() == nil && c.matchesDrivingOwnerLocked(TurnSubmitScope{SessionPath: s.scope.SessionPath, RuntimeEpoch: s.scope.RuntimeEpoch}) && c.runtimeState.ledger == s.ledger
	c.mu.Unlock()
	c.runtimeState.mu.Unlock()
	select {
	case <-s.done:
		valid = false
	default:
	}
	if !valid {
		s.Close()
		return DesktopObservedEvent{}, ErrDesktopObservation
	}
	return frame, nil
}

func (p *controllerDesktopEvents) retire() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for sub := range p.subs {
		p.endLocked(sub)
	}
}

func (p *controllerDesktopEvents) endLocked(sub *DesktopEventSubscription) {
	if _, exists := p.subs[sub]; exists {
		delete(p.subs, sub)
		if sub.stop != nil {
			sub.stop()
		}
		close(sub.done)
	}
}

func (p *controllerDesktopEvents) publish(ledger *turnevent.Ledger, kind event.Kind) {
	var name string
	switch kind {
	case event.TurnStarted:
		name = "turn_started"
	case event.TurnDone:
		name = "turn_done"
	case event.AskRequest:
		name = "ask_request"
	case event.ApprovalRequest:
		name = "approval_request"
	case event.MCPInteractionRequest:
		name = "mcp_interaction"
	default:
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for sub := range p.subs {
		if sub.ledger != ledger {
			p.endLocked(sub)
			continue
		}
		select {
		case sub.frames <- DesktopObservedEvent{Kind: name}:
		default:
			p.endLocked(sub)
		}
	}
}
