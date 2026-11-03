package desktopbridge

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"reasonix/internal/eventwire"
)

// Only public checkpoint fields cross the publication lock. Tool output,
// arguments, reasoning and the full event remain solely in the wire envelope.
type liveUpdate struct {
	valid bool
	text  string
	tool  LiveTool
}

func liveUpdateFromWire(wire eventwire.Event) liveUpdate {
	switch wire.Kind {
	case "text":
		return liveUpdate{valid: true, text: prepareLiveText(wire.Text)}
	case "tool_dispatch", "tool_result":
		if wire.Tool != nil {
			return prepareLiveTool(wire.Kind, wire.Tool.ID, wire.Tool.Name, wire.Tool.Err != "")
		}
	}
	return liveUpdate{}
}

// Raw publishers retain the old JSON interpretation, but parsing happens
// before taking the shared lock. Typed Agent events bypass this round trip.
func liveUpdateFromPayload(input Event) liveUpdate {
	switch input.EventKind {
	case "text":
		var payload struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		}
		if json.Unmarshal(input.Payload, &payload) == nil && payload.Kind == "text" {
			return liveUpdate{valid: true, text: prepareLiveText(payload.Text)}
		}
	case "tool_dispatch", "tool_result":
		var payload struct {
			Tool *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Err  string `json:"err"`
			} `json:"tool"`
		}
		if json.Unmarshal(input.Payload, &payload) == nil && payload.Tool != nil {
			return prepareLiveTool(input.EventKind, payload.Tool.ID, payload.Tool.Name, payload.Tool.Err != "")
		}
	}
	return liveUpdate{}
}

func prepareLiveTool(kind, id, name string, failed bool) liveUpdate {
	if id == "" {
		return liveUpdate{}
	}
	state := "running"
	if kind == "tool_result" {
		state = "done"
		if failed {
			state = "failed"
		}
	}
	id = strings.Clone(jsonLiveText(livePrefix(id, 256)))
	name = strings.Clone(jsonLiveText(livePrefix(name, 120)))
	if strings.TrimSpace(name) == "" {
		name = "工具"
	}
	return liveUpdate{valid: true, tool: LiveTool{ID: id, Name: name, State: state}}
}

func prepareLiveText(text string) string {
	// One extra rune preserves the old over-limit/truncation decision while
	// bounding work under the lock even for a very large text event.
	return jsonLiveText(livePrefix(text, LiveAnswerRuneLimit+1))
}

func livePrefix(text string, limit int) string {
	runes := 0
	for index := range text {
		if runes == limit {
			return text[:index]
		}
		runes++
	}
	return text
}

func jsonLiveText(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	// JSON replaces each invalid UTF-8 byte, not an entire invalid byte run.
	return string([]rune(text))
}
