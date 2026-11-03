package desktopbridge

import (
	"strings"
	"unicode/utf8"
)

const LiveAnswerRuneLimit = 64 << 10
const liveStateLimit = 64

type LiveAnswer struct {
	Segments  []string `json:"segments"`
	Streaming bool     `json:"streaming"`
}
type LiveTool struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}
type LiveProgress struct {
	Phase string     `json:"phase"`
	Tools []LiveTool `json:"tools"`
}

// LiveState contains public presentation only. Reasoning, tool arguments,
// outputs, system messages and credentials are never checkpointed here.
type LiveState struct {
	Active    bool         `json:"active"`
	Answer    LiveAnswer   `json:"answer"`
	Progress  LiveProgress `json:"progress"`
	Truncated bool         `json:"truncated"`
}
type liveCheckpoint struct {
	view     LiveState
	runes    int
	sequence uint64
	answer   strings.Builder
}

func emptyLiveState() LiveState {
	return LiveState{Answer: LiveAnswer{Segments: []string{}}, Progress: LiveProgress{Phase: "thinking", Tools: []LiveTool{}}}
}

// LiveSnapshot and the global replay cursor are captured under publication's
// lock. A renderer can hydrate then subscribe after this cursor without a gap.
func (s *EventStream) LiveSnapshot(id string) (LiveState, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	view := emptyLiveState()
	if checkpoint := s.live[id]; checkpoint != nil {
		view = checkpoint.view
		view.Answer.Segments = append([]string{}, view.Answer.Segments...)
		view.Progress.Tools = append([]LiveTool{}, view.Progress.Tools...)
	}
	return view, s.ledger.LatestSequence()
}

// Caller holds s.mu. At most one in-flight model response is retained: text
// completed before a tool dispatch already belongs to authoritative history.
func (s *EventStream) advanceLive(published Event, update liveUpdate) {
	checkpoint := s.live[published.SessionID]
	if published.EventKind == "turn_started" {
		if checkpoint == nil && len(s.live) >= liveStateLimit {
			oldestID := ""
			var oldest uint64
			for id, item := range s.live {
				if !item.view.Active && (oldestID == "" || item.sequence < oldest) {
					oldestID = id
					oldest = item.sequence
				}
			}
			if oldestID == "" {
				return
			}
			delete(s.live, oldestID)
		}
		checkpoint = &liveCheckpoint{view: emptyLiveState()}
		checkpoint.view.Active = true
		s.live[published.SessionID] = checkpoint
	}
	if checkpoint == nil {
		return
	}
	checkpoint.sequence = published.Sequence
	switch published.EventKind {
	case "turn_done":
		checkpoint.view.Active = false
		checkpoint.view.Answer = LiveAnswer{Segments: []string{}}
		checkpoint.answer.Reset()
		checkpoint.view.Progress.Phase = "finishing"
	case "reasoning":
		checkpoint.view.Progress.Phase = "thinking"
	case "text":
		if !update.valid {
			return
		}
		remaining := LiveAnswerRuneLimit - checkpoint.runes
		delta := update.text
		if utf8.RuneCountInString(delta) > remaining {
			delta = string([]rune(delta)[:remaining])
			checkpoint.view.Truncated = true
		}
		if len(checkpoint.view.Answer.Segments) == 0 {
			checkpoint.view.Answer.Segments = []string{""}
		}
		// Append once instead of copying the complete answer for every token.
		// Builder strings remain immutable when later deltas append or reset;
		// LiveSnapshot separately copies the public segment slice.
		checkpoint.answer.WriteString(delta)
		checkpoint.view.Answer.Segments[0] = checkpoint.answer.String()
		checkpoint.runes += utf8.RuneCountInString(delta)
		checkpoint.view.Answer.Streaming = true
		checkpoint.view.Progress.Phase = "answering"
	case "tool_dispatch", "tool_result":
		if !update.valid {
			return
		}
		phase := "tool"
		if published.EventKind == "tool_result" {
			phase = "thinking"
		}
		item := update.tool
		found := false
		for i, current := range checkpoint.view.Progress.Tools {
			if current.ID == item.ID {
				found = true
				checkpoint.view.Progress.Tools[i] = item
				break
			}
		}
		if !found {
			checkpoint.view.Progress.Tools = append(checkpoint.view.Progress.Tools, item)
			if len(checkpoint.view.Progress.Tools) > 30 {
				checkpoint.view.Progress.Tools = checkpoint.view.Progress.Tools[1:]
			}
		}
		checkpoint.view.Progress.Phase = phase
		if published.EventKind == "tool_dispatch" {
			checkpoint.view.Answer = LiveAnswer{Segments: []string{}}
			checkpoint.answer.Reset()
			checkpoint.runes = 0
			checkpoint.view.Truncated = false
		}
	}
}

func clipLiveText(value string, limit int) string {
	if utf8.RuneCountInString(value) > limit {
		return string([]rune(value)[:limit])
	}
	return value
}
