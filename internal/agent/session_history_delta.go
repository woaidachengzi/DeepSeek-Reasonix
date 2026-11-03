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

// SessionHistoryReader pins a source Session for one history request so its
// chunks cannot accidentally cross an executor Session swap. Keep it request-
// local: unlike SessionHistoryCursor, a reader retains the private transcript.
// Begin and Chunk reuse caller-owned memory; Delta is the atomic-copy fallback.
type SessionHistoryReader struct{ session *Session }

// HistoryReader captures this Session for a request-local chunked read.
func (s *Session) HistoryReader() SessionHistoryReader { return SessionHistoryReader{session: s} }

// Begin fixes the prefix to consume from the captured source.
func (r SessionHistoryReader) Begin(previous SessionHistoryCursor) (SessionHistoryCursor, int, bool) {
	return r.session.HistoryReadBegin(previous)
}

// Chunk fills scratch from the captured source, subject to the target fence.
func (r SessionHistoryReader) Chunk(target SessionHistoryCursor, start int, scratch []provider.Message) ([]provider.Message, bool) {
	return r.session.HistoryReadChunk(target, start, scratch)
}

// Delta provides an atomic-copy fallback from the same captured source.
func (r SessionHistoryReader) Delta(previous SessionHistoryCursor) ([]provider.Message, SessionHistoryCursor, bool) {
	return r.session.HistoryDelta(previous)
}

// HistoryReadBegin fixes the transcript prefix a chunked read will consume.
// start identifies its append suffix, or zero when reset requires a rebuild.
// No message values or private payloads are copied by this operation.
func (s *Session) HistoryReadBegin(previous SessionHistoryCursor) (target SessionHistoryCursor, start int, reset bool) {
	if s == nil {
		return SessionHistoryCursor{}, 0, true
	}
	identity := s.historyIdentityToken()
	s.mu.RLock()
	defer s.mu.RUnlock()
	target = s.historyCursorLocked(identity)
	start, reset = historyReadStart(previous, target)
	return target, start, reset
}

// HistoryReadChunk copies at most cap(scratch) message values from the fixed
// target prefix under one short read lock. Appends and turn markers may advance
// the transcript version; edits to any existing prefix invalidate the read.
// An invalid read returns no data: discard every earlier chunk from that build
// and restart or use HistoryDelta. Never publish a partially rebuilt projection.
//
// scratch must be caller-owned and must not alias Session messages. Returned
// values reuse that buffer, including when scratch has length zero. Process
// them before the next chunk overwrites it. Nested records follow Snapshot's
// immutable contract. Clear the scratch before retaining it beyond the request.
func (s *Session) HistoryReadChunk(target SessionHistoryCursor, start int, scratch []provider.Message) (messages []provider.Message, valid bool) {
	if s == nil {
		return scratch[:0], target == (SessionHistoryCursor{}) && start == 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if target.identity == nil || target.identity != s.historyIdentity.Load() ||
		target.rewriteVersion != s.rewriteVersion || target.historyEpoch != s.historyEpoch ||
		target.version > s.version || target.count > len(s.Messages) || start < 0 || start > target.count {
		return nil, false
	}
	if start == target.count {
		return scratch[:0], true
	}
	if cap(scratch) == 0 {
		return nil, false
	}
	count := min(cap(scratch), target.count-start)
	messages = scratch[:count]
	copy(messages, s.Messages[start:start+count])
	return messages, true
}

func (s *Session) historyCursorLocked(identity *sessionHistoryIdentity) SessionHistoryCursor {
	return SessionHistoryCursor{
		identity: identity, version: s.version, rewriteVersion: s.rewriteVersion,
		historyEpoch: s.historyEpoch, count: len(s.Messages),
	}
}

func historyReadStart(previous, target SessionHistoryCursor) (start int, reset bool) {
	reset = previous.identity != target.identity || previous.rewriteVersion != target.rewriteVersion ||
		previous.historyEpoch != target.historyEpoch || previous.count > target.count || previous.version > target.version
	if !reset {
		start = previous.count
	}
	return start, reset
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
	next = s.historyCursorLocked(identity)
	start, reset := historyReadStart(previous, next)
	if start == next.count {
		return nil, next, reset
	}
	return append([]provider.Message(nil), s.Messages[start:]...), next, reset
}
