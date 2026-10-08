package desktopbridge

import "encoding/json"

// Only typed, session-owned terminal projections enter the replay stream.
// This intentionally bypasses event.Sink: terminal bytes are not Agent events.
func (s *EventStream) publishTerminal(kind, sessionID string, value any) {
	if s == nil || sessionID == "" {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.publish(Event{EventKind: kind, SessionID: sessionID, Payload: payload})
}
