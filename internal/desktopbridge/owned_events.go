package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

var (
	ErrOwnedObservationClosed = errors.New("owned event observation is closed")
	ErrOwnedObservationGap    = errors.New("owned event observation requires a fresh snapshot")
)

const ownedObserverLimit = 8
const ownedObserverBuffer = 32
const ownedEventBytes = 64 << 10

// OwnedObservedEvent is private host observation, not a replay ledger or a
// renderer/IM message. Its payload retains the core's event turn/prompt IDs;
// Scope separately identifies the published manager/Controller owner.
type OwnedObservedEvent struct {
	Scope   OwnedCommandScope
	Kind    string
	Payload json.RawMessage
}

type OwnedEventStream struct {
	mu          sync.Mutex
	closed      bool
	sources     map[OwnedCommandScope]*OwnedEventSource
	subscribers map[*OwnedEventSubscription]struct{}
}

type OwnedEventSource struct {
	stream        *OwnedEventStream
	scope         OwnedCommandScope
	bound, closed bool // protected by stream.mu
	emitMu        sync.Mutex
}

type OwnedEventSubscription struct {
	stream *OwnedEventStream
	scope  OwnedCommandScope
	frames chan OwnedObservedEvent
	ended  error // protected by stream.mu
}

func NewOwnedEventStream() *OwnedEventStream {
	return &OwnedEventStream{sources: make(map[OwnedCommandScope]*OwnedEventSource), subscribers: make(map[*OwnedEventSubscription]struct{})}
}

func (s *OwnedEventStream) NewSource() *OwnedEventSource { return &OwnedEventSource{stream: s} }

// Activate is called only after manager publication, never at boot/candidate
// construction. A source may not adopt another generation or revive after close.
func (s *OwnedEventSource) Activate(scope OwnedCommandScope) bool {
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	if s.stream.closed || s.closed || scope.SessionID == "" || scope.OwnerEpoch == 0 || scope.SessionPath == "" || scope.RuntimeEpoch == "" {
		return false
	}
	if s.bound {
		return s.scope == scope
	}
	if other := s.stream.sources[scope]; other != nil {
		return false
	}
	s.scope = scope
	s.bound = true
	s.stream.sources[scope] = s
	return true
}

func (s *OwnedEventStream) Subscribe(scope OwnedCommandScope) (*OwnedEventSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sources[scope] == nil {
		return nil, ErrOwnedRuntimeChanged
	}
	if len(s.subscribers) >= ownedObserverLimit {
		return nil, ErrOwnedObservationGap
	}
	sub := &OwnedEventSubscription{stream: s, scope: scope, frames: make(chan OwnedObservedEvent, ownedObserverBuffer)}
	s.subscribers[sub] = struct{}{}
	return sub, nil
}

// Read never returns a queued prefix after overflow, cancellation/retirement,
// or Close. No hidden resubscribe/replay; callers must refresh authoritative state.
func (s *OwnedEventSubscription) Read(ctx context.Context) (OwnedObservedEvent, error) {
	if ctx == nil {
		return OwnedObservedEvent{}, ErrOwnedObservationClosed
	}
	if err := ctx.Err(); err != nil {
		s.Close()
		return OwnedObservedEvent{}, err
	}
	select {
	case <-ctx.Done():
		s.Close()
		return OwnedObservedEvent{}, ctx.Err()
	case frame, ok := <-s.frames:
		s.stream.mu.Lock()
		defer s.stream.mu.Unlock()
		if s.ended != nil {
			return OwnedObservedEvent{}, s.ended
		}
		if err := ctx.Err(); err != nil {
			s.stream.endLocked(s, ErrOwnedObservationClosed)
			return OwnedObservedEvent{}, err
		}
		if !ok {
			return OwnedObservedEvent{}, ErrOwnedObservationClosed
		}
		return frame, nil
	}
}

func (s *OwnedEventStream) endLocked(sub *OwnedEventSubscription, reason error) {
	if sub.ended != nil {
		return
	}
	sub.ended = reason
	delete(s.subscribers, sub)
	close(sub.frames)
	// Retire buffered private payloads too; a retained closed subscription must
	// not hold a full stale turn in memory indefinitely.
	for range sub.frames {
	}
}

func (s *OwnedEventSubscription) Close() {
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	s.stream.endLocked(s, ErrOwnedObservationClosed)
}

func (s *OwnedEventSource) Emit(input event.Event) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.stream.mu.Lock()
	if s.closed || !s.bound || s.stream.closed {
		s.stream.mu.Unlock()
		return
	}
	scope := s.scope
	listening := false
	for sub := range s.stream.subscribers {
		if sub.scope == scope {
			listening = true
			break
		}
	}
	s.stream.mu.Unlock()
	if !listening {
		return
	} // observation never manufactures startup/replay events
	kind, ok := eventwire.KindName(input.Kind)
	payload, err := json.Marshal(eventwire.ToWire(input))
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	if s.closed || s.stream.closed || s.stream.sources[scope] != s {
		return
	}
	for sub := range s.stream.subscribers {
		if sub.scope != scope {
			continue
		}
		if !ok || err != nil || len(payload) > ownedEventBytes {
			s.stream.endLocked(sub, ErrOwnedObservationGap)
			continue
		}
		frame := OwnedObservedEvent{scope, kind, append(json.RawMessage(nil), payload...)}
		select {
		case sub.frames <- frame:
		default:
			s.stream.endLocked(sub, ErrOwnedObservationGap)
		}
	}
}

func (s *OwnedEventSource) Close() {
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.stream.sources[s.scope] == s {
		delete(s.stream.sources, s.scope)
	}
	for sub := range s.stream.subscribers {
		if s.bound && sub.scope == s.scope {
			s.stream.endLocked(sub, ErrOwnedRuntimeChanged)
		}
	}
}

func (s *OwnedEventStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for _, source := range s.sources {
		source.closed = true
	}
	clear(s.sources)
	for sub := range s.subscribers {
		s.endLocked(sub, ErrOwnedObservationClosed)
	}
}

// RuntimeOwnedEventPublisher is optional. Activation performs only bounded
// local bookkeeping and must not call back into the manager or use network IO.
type RuntimeOwnedEventPublisher interface{ ActivateOwnedEvents(OwnedCommandScope) }

func (m *RuntimeManager) activateOwnedEventsLocked() {
	publisher, ok := m.runtime.(RuntimeOwnedEventPublisher)
	if !ok {
		return
	}
	commands, ok := m.runtime.(RuntimeOwnedCommands)
	if !ok {
		return
	}
	epoch, _ := commands.CommandSnapshot()
	publisher.ActivateOwnedEvents(OwnedCommandScope{m.view.ID, m.ownerEpoch, m.view.Path, epoch})
}
