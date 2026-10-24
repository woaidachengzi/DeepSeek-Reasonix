package agent

import (
	"encoding/json"
	"strings"

	"reasonix/internal/provider"
)

const computerUseFullObservations = 2

// This request-only view never edits the conversation or its diagnostic data.
// Old desktop images and AX trees are superseded by fresh observations; input
// outcomes and pairing remain, including refusals and uncertain dispatches.
func computerUseContext(msgs []provider.Message) []provider.Message {
	ids := make(map[string]bool)
	for _, m := range msgs {
		for _, call := range m.ToolCalls {
			if call.ID != "" && computerUseCall(call) {
				ids[call.ID] = true
			}
		}
	}
	var out []provider.Message
	kept := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != provider.RoleTool || (m.Name != "computer_use" && !ids[m.ToolCallID]) {
			continue
		}
		body, prefix := computerUseResult(m.Content)
		if body == nil || body["state"] == nil {
			continue
		}
		var state map[string]json.RawMessage
		if json.Unmarshal(body["state"], &state) != nil || state["window_id"] == nil {
			continue
		}
		kept++
		if kept <= computerUseFullObservations {
			continue
		}
		for key := range state {
			switch key {
			case "pid", "bundle_id", "window_id", "launch_time", "bounds", "frontmost", "focus":
			default:
				delete(state, key)
			}
		}
		body["state"], _ = json.Marshal(state)
		delete(body, "observation")
		body["superseded_observation"] = json.RawMessage(`true`)
		body["history_note"] = json.RawMessage(`"Old screenshot and AX tree omitted from model input; full result remains locally via session:tool_result. Observe again before input."`)
		encoded, err := json.Marshal(body)
		if err != nil {
			continue
		}
		if out == nil {
			out = append([]provider.Message(nil), msgs...)
		}
		out[i].Content = prefix + string(encoded)
		out[i].Images = nil
	}
	if out != nil {
		return out
	}
	return msgs
}

func computerUseCall(call provider.ToolCall) bool {
	if call.Name == "computer_use" || call.ResolvedName == "computer_use" || call.CapabilityID == "tool:computer_use" {
		return true
	}
	if call.Name != "use_capability" {
		return false
	}
	var args struct {
		Action     string `json:"action"`
		Capability string `json:"capability_id"`
	}
	return json.Unmarshal([]byte(call.Arguments), &args) == nil && args.Action == "call" && args.Capability == "tool:computer_use"
}

func computerUseResult(text string) (map[string]json.RawMessage, string) {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return nil, ""
	}
	var body map[string]json.RawMessage
	if json.Unmarshal([]byte(text[start:]), &body) != nil {
		return nil, ""
	}
	if value := string(body["dispatched"]); value != "true" && value != "false" {
		return nil, ""
	}
	return body, text[:start]
}
