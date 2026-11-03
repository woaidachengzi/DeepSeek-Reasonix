package agent

import "reasonix/internal/provider"

// A nonzero-sized allocation has a distinct address for each Session. The token
// intentionally has no pointer back to the Session or its transcript payloads.
type sessionHistoryIdentity struct{ _ byte }

// SessionHistoryCursor is an opaque in-process append fence. It belongs to one
// Session instance, is never persisted or sent to a desktop, and remains valid
// only for the exact prefix returned by HistoryDelta. A zero cursor starts a
// fresh projection.
type SessionHistoryCursor struct {
	identity       *sessionHistoryIdentity
	version        uint64
	rewriteVersion int
	historyEpoch   uint64
	count          int
}

// MessageCount is the raw transcript length at the cursor's atomic read point,
// including system messages and host-only records. It is not a display count.
func (c SessionHistoryCursor) MessageCount() int { return c.count }

func (s *Session) historyIdentityToken() *sessionHistoryIdentity {
	if identity := s.historyIdentity.Load(); identity != nil {
		return identity
	}
	identity := &sessionHistoryIdentity{}
	if s.historyIdentity.CompareAndSwap(nil, identity) {
		return identity
	}
	return s.historyIdentity.Load()
}

// HistoryDelta reads one coherent transcript version under one lock. reset
// requires the caller to discard its previous projection and rebuild from all
// returned messages. Otherwise only newly appended messages are returned; an
// unchanged transcript or a turn-marker-only change returns no messages.
//
// Like Snapshot, the returned slice owns its message values, while nested
// records remain immutable: Session writers replace nested records rather than
// editing ones already published to readers. Consumers must not mutate them.
func (s *Session) HistoryDelta(previous SessionHistoryCursor) (messages []provider.Message, next SessionHistoryCursor, reset bool) {
	if s == nil {
		return nil, SessionHistoryCursor{}, true
	}
	identity := s.historyIdentityToken()
	s.mu.RLock()
	defer s.mu.RUnlock()
	next = SessionHistoryCursor{
		identity: identity, version: s.version, rewriteVersion: s.rewriteVersion,
		historyEpoch: s.historyEpoch, count: len(s.Messages),
	}
	reset = previous.identity != identity || previous.rewriteVersion != next.rewriteVersion ||
		previous.historyEpoch != next.historyEpoch || previous.count > next.count || previous.version > next.version
	start := previous.count
	if reset {
		start = 0
	}
	if start == next.count {
		return nil, next, reset
	}
	return append([]provider.Message(nil), s.Messages[start:]...), next, reset
}
