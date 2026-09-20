package desktopbridge

import (
	"encoding/json"
	"sync"
)

// ProtocolVersion is the current major version of the private desktop bridge.
const ProtocolVersion = 1

// Event is the replayable portion of a bridge event. Payload is intentionally
// opaque until each eventKind receives its own protocol schema.
type Event struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Sequence        uint64          `json:"sequence"`
	EventKind       string          `json:"eventKind"`
	SessionID       string          `json:"sessionId"`
	TabID           string          `json:"tabId,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

// EventLedger keeps a bounded replay window. A caller below OldestSequence
// must re-fetch an authoritative snapshot instead of guessing missing state.
type EventLedger struct {
	mu       sync.Mutex
	capacity int
	next     uint64
	events   []Event
}

func NewEventLedger(capacity int) *EventLedger {
	if capacity < 1 {
		capacity = 1
	}
	return &EventLedger{capacity: capacity}
}

func (l *EventLedger) Append(event Event) Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	event.ProtocolVersion = ProtocolVersion
	event.Sequence = l.next
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	l.events = append(l.events, event)
	if overflow := len(l.events) - l.capacity; overflow > 0 {
		copy(l.events, l.events[overflow:])
		l.events = l.events[:len(l.events)-overflow]
	}
	return event
}

// After returns events strictly newer than sequence. resyncRequired means the
// requested sequence predates the bounded ledger and the client must snapshot.
func (l *EventLedger) After(sequence uint64) (events []Event, resyncRequired bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.events) == 0 {
		return nil, false
	}
	oldest := l.events[0].Sequence
	if sequence+1 < oldest {
		return nil, true
	}
	for _, event := range l.events {
		if event.Sequence > sequence {
			event.Payload = append(json.RawMessage(nil), event.Payload...)
			events = append(events, event)
		}
	}
	return events, false
}
