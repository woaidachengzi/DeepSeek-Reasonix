package builtin

import (
	"encoding/json"

	"reasonix/internal/computeruse"
)

// Upstream follow-up calls contain SDK session arguments and tool names that
// are not our public contract. Project only supported read/setup follow-ups;
// never turn a hint into execution or return the private session label.
func cuaBrowserHints(raw json.RawMessage, request computeruse.CuaRequest) json.RawMessage {
	var state map[string]any
	if json.Unmarshal(raw, &state) != nil || state == nil {
		return raw
	}
	cuaBrowserNextCall(state, request)
	if refusal, ok := state["refusal"].(map[string]any); ok {
		if detail, ok := refusal["detail"].(map[string]any); ok {
			cuaBrowserNextCall(detail, request)
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return raw
	}
	return encoded
}

func cuaBrowserNextCall(state map[string]any, request computeruse.CuaRequest) {
	next, ok := state["next_call"].(map[string]any)
	if !ok {
		return
	}
	args := map[string]any{"pid": request.PID, "window_id": request.WindowID}
	switch next["tool"] {
	case "get_browser_state":
		args["action"] = "browser_observe"
		if request.TargetID != "" && request.TabID != "" {
			args["target_id"], args["tab_id"] = request.TargetID, request.TabID
		}
	case "browser_prepare":
		args["action"] = "browser_prepare"
	case "get_window_state":
		args["action"] = "observe"
	default:
		delete(state, "next_call")
		return
	}
	state["next_call"] = map[string]any{"tool": "computer_use", "arguments": args}
}
