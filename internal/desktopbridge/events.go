package desktopbridge

import (
	"encoding/json"
	"sync"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

const subscriberBuffer = 128

// EventStream projects the established typed Go event stream into the
// bridge's replayable, session-scoped wire envelope. Slow subscribers may
// miss live frames, but can reconnect from the bounded ledger.
type EventStream struct {
	mu          sync.Mutex
	ledger      *EventLedger
	subscribers map[chan Event]struct{}
}

func NewEventStream(capacity int) *EventStream {
	return &EventStream{
		ledger:      NewEventLedger(capacity),
		subscribers: make(map[chan Event]struct{}),
	}
}

// Sink returns a non-blocking event.Sink for one bridge-owned session.
func (s *EventStream) Sink(sessionID string) event.Sink {
	return event.FuncSink(func(input event.Event) {
		kind, ok := eventwire.KindName(input.Kind)
		if !ok {
			return
		}
		payload, err := json.Marshal(eventwire.ToWire(input))
		if err != nil {
			return
		}
		s.publish(Event{EventKind: kind, SessionID: sessionID, Payload: payload})
	})
}

// Subscribe atomically returns replay frames newer than after and registers a
// live subscriber. If replay is no longer possible, callers must snapshot.
func (s *EventStream) Subscribe(after uint64) (events []Event, resyncRequired bool, live <-chan Event, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, resyncRequired = s.ledger.After(after)
	if resyncRequired {
		return events, true, nil, func() {}
	}
	channel := make(chan Event, subscriberBuffer)
	s.subscribers[channel] = struct{}{}
	return events, false, channel, func() {
		s.mu.Lock()
		delete(s.subscribers, channel)
		s.mu.Unlock()
	}
}

func (s *EventStream) publish(input Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	published := s.ledger.Append(input)
	for subscriber := range s.subscribers {
		select {
		case subscriber <- published:
		default:
			// The ledger is the recovery source; never stall Agent work on a UI.
		}
	}
}
