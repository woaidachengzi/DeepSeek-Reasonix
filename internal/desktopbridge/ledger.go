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
	head     int
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
	if len(l.events) < l.capacity {
		l.events = append(l.events, event)
	} else {
		// Replace the oldest slot without moving the entire replay window for
		// every delta. Replay still walks oldest to newest under the same lock.
		l.events[l.head] = event
		l.head = (l.head + 1) % len(l.events)
	}
	return event
}

// After returns events strictly newer than sequence. resyncRequired means the
// requested sequence predates the bounded ledger and the client must snapshot.
func (l *EventLedger) After(sequence uint64) (events []Event, resyncRequired bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if sequence > l.next {
		// A cursor from another process lifetime cannot subscribe to this
		// ledger and silently ignore all newly restarted sequence numbers.
		return nil, true
	}
	if len(l.events) == 0 {
		return nil, false
	}
	oldest := l.events[l.head].Sequence
	if sequence < oldest-1 {
		return nil, true
	}
	for offset := range len(l.events) {
		event := l.events[(l.head+offset)%len(l.events)]
		if event.Sequence > sequence {
			event.Payload = append(json.RawMessage(nil), event.Payload...)
			events = append(events, event)
		}
	}
	return events, false
}

// LatestSequence reports the newest sequence assigned by this ledger, even
// when no event remains in its bounded replay window.
func (l *EventLedger) LatestSequence() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.next
}
