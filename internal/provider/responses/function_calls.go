package responses

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"reasonix/internal/provider"
)

// Typed decoding failures for function events must invalidate the attempt.
// Ignoring them could turn a malformed tool invocation into a successful answer.
func malformedFunctionEvent(data []byte) bool {
	var envelope struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	_ = json.Unmarshal(data, &envelope)
	return strings.HasPrefix(envelope.Type, "response.function_call_arguments.") || envelope.Item.Type == "function_call"
}

func validArgumentsEvent(data []byte, eventType string) bool {
	key := ""
	switch eventType {
	case "response.function_call_arguments.done":
		key = "arguments"
	case "response.function_call_arguments.delta":
		key = "delta"
	default:
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	value := fields[key]
	return len(value) > 0 && value[0] == '"'
}

// Item IDs can change between streamed and terminal snapshots. The provider's
// call_id remains the execution identity; all item aliases share one record.
func bindFunctionCall(item sseItem, calls map[string]*streamedCall, callForItem func(string) *streamedCall) (*streamedCall, error) {
	key := item.ID
	if key == "" {
		key = item.CallID
	}
	call := calls[key]
	if call != nil && ((call.id != "" && item.CallID != "" && call.id != item.CallID) ||
		(call.name != "" && item.Name != "" && call.name != item.Name)) {
		return nil, ErrInvalidFunctionCall
	}
	if item.CallID != "" {
		for _, existing := range calls {
			if existing.id == item.CallID {
				call = existing
				break
			}
		}
	}
	if call == nil {
		call = callForItem(key)
	}
	if call.name != "" && item.Name != "" && call.name != item.Name {
		return nil, ErrInvalidFunctionCall
	}
	if item.CallID != "" {
		call.id = item.CallID
	}
	if item.Name != "" {
		call.name = item.Name
	}
	// Register new aliases through callForItem so continuation history retains
	// order even when no added event preceded the final snapshot.
	if calls[key] == nil {
		callForItem(key)
	}
	calls[key] = call
	return call, nil
}

func (c *client) finishStreamedArguments(ctx context.Context, out chan<- provider.Chunk, call *streamedCall, arguments string) bool {
	call.announced = true
	if call.completed {
		if !equivalentArguments(call.arguments, arguments) {
			return c.rejectTerminalCalls(ctx, out, ErrInvalidFunctionCall)
		}
		return true
	}
	call.arguments = arguments
	// An arguments.done event alone has no execution identity. Defer emission
	// until the full item arrives; terminal validation rejects unresolved calls.
	if strings.TrimSpace(call.id) == "" || strings.TrimSpace(call.name) == "" {
		return true
	}
	call.completed = true
	return emitFunctionCall(ctx, out, call)
}

func (c *client) completeFunctionItem(ctx context.Context, out chan<- provider.Chunk, item sseItem, calls map[string]*streamedCall, callForItem func(string) *streamedCall) bool {
	call, err := bindFunctionCall(item, calls, callForItem)
	if err != nil {
		return c.rejectTerminalCalls(ctx, out, err)
	}
	if call.completed {
		if !equivalentArguments(call.arguments, item.Arguments) {
			return c.rejectTerminalCalls(ctx, out, ErrInvalidFunctionCall)
		}
		return true
	}
	call.arguments, call.completed = item.Arguments, true
	return emitFunctionCall(ctx, out, call)
}

func emitFunctionCall(ctx context.Context, out chan<- provider.Chunk, call *streamedCall) bool {
	return sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: call.id, Name: call.name, Arguments: call.arguments}})
}

func equivalentArguments(a, b string) bool {
	if a == b {
		return true
	}
	if !json.Valid([]byte(a)) || !json.Valid([]byte(b)) {
		return false
	}
	decode := func(s string) any {
		var value any
		decoder := json.NewDecoder(strings.NewReader(s))
		decoder.UseNumber() // Large integer arguments must never lose precision.
		_ = decoder.Decode(&value)
		return value
	}
	return reflect.DeepEqual(decode(a), decode(b))
}

func completedToolCalls(calls map[string]*streamedCall, order []string) []provider.ToolCall {
	var result []provider.ToolCall
	seen := map[string]bool{}
	for _, key := range order {
		call := calls[key]
		if call.completed && !seen[call.id] {
			seen[call.id] = true
			result = append(result, provider.ToolCall{ID: call.id, Name: call.name, Arguments: call.arguments})
		}
	}
	return result
}
