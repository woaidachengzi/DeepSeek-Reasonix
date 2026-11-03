package control

import (
	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// HistoryDelta captures one Session and reads its atomic append suffix. Its
// identity fence requires a fresh projection after resume or same-path reload,
// even when message counts and versions happen to match.
func (c *Controller) HistoryDelta(previous agent.SessionHistoryCursor) ([]provider.Message, agent.SessionHistoryCursor, bool) {
	var session *agent.Session
	if c != nil && c.executor != nil {
		session = c.executor.Session()
	}
	return session.HistoryDelta(previous)
}

// HistoryReader captures the executor Session once for a chunked history
// request. Keep the returned reader local to that request; resident projection
// caches retain only its opaque cursor, which cannot retain private messages.
func (c *Controller) HistoryReader() agent.SessionHistoryReader {
	var session *agent.Session
	if c != nil && c.executor != nil {
		session = c.executor.Session()
	}
	return session.HistoryReader()
}
