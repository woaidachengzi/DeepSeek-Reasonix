package desktopbridge

import "encoding/json"

// AITraceTiming contains execution facts only, never event text or screenshots.
// This is a bounded in-memory supplement, not durable history or a new recorder.
type AITraceTiming struct {
	Sequence   uint64 `json:"sequence"`
	ToolCallID string `json:"toolCallId"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	EndedAt    int64  `json:"endedAt,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

func (s *EventStream) AITraceTimings(sessionID string) []AITraceTiming {
	s.ledger.mu.Lock()
	frames := make([]Event, 0)
	for _, frame := range s.ledger.events {
		if frame.SessionID == sessionID && frame.EventKind == "tool_result" {
			// Published payload bytes are immutable. Copy envelopes so decoding
			// never stalls agent publication while holding the ledger lock.
			frames = append(frames, frame)
		}
	}
	s.ledger.mu.Unlock()
	result := []AITraceTiming{}
	for _, frame := range frames {
		var payload struct {
			Tool struct {
				ID         string `json:"id"`
				StartedAt  int64  `json:"startedAt"`
				EndedAt    int64  `json:"endedAt"`
				DurationMs int64  `json:"durationMs"`
			} `json:"tool"`
		}
		if json.Unmarshal(frame.Payload, &payload) != nil || payload.Tool.ID == "" {
			continue
		}
		t := payload.Tool
		result = append(result, AITraceTiming{Sequence: frame.Sequence, ToolCallID: t.ID, StartedAt: t.StartedAt, EndedAt: t.EndedAt, DurationMs: t.DurationMs})
	}
	return result
}
