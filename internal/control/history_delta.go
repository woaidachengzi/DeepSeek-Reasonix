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
