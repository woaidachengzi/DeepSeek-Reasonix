package responses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/provider"
)

var (
	// ErrInvalidFunctionCall marks malformed wire data in a completed response.
	ErrInvalidFunctionCall = errors.New("responses: completed response contains an invalid function call")
	// ErrUnfinishedFunctionCall marks a terminal stream with an open call.
	ErrUnfinishedFunctionCall = errors.New("responses: stream ended with an unfinished function call")
)

// A protocol failure invalidates the entire attempt, including already streamed
// text and complete neighboring calls. These errors deliberately are not
// transport interruptions: retrying a malformed completion hides the failure.
func (c *client) rejectTerminalCalls(ctx context.Context, out chan<- provider.Chunk, err error) bool {
	c.ResetContext()
	_ = sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: err})
	return false
}

func (c *client) finishResponse(ctx context.Context, out chan<- provider.Chunk, event sseEvent, calls map[string]*streamedCall, callForItem func(string) *streamedCall) (failed, ok bool) {
	if event.Type == "response.completed" && !c.finishTerminalCalls(ctx, out, event.Response, calls, callForItem) {
		return false, false
	}
	if event.Type == "response.incomplete" && !sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkReasoning, ReasoningState: provider.ReasoningIncomplete}) {
		return false, false
	}
	if !emitTerminalResponseUsage(ctx, out, event) {
		return false, false
	}
	if event.Type != "response.failed" {
		return false, true
	}
	err := fmt.Errorf("responses: response failed")
	if event.Response != nil && event.Response.Error != nil {
		if authErr := authErrorFromResponse(c, event.Response.Error); authErr != nil {
			err = authErr
		} else {
			err = fmt.Errorf("responses: %s", event.Response.Error.Message)
		}
	}
	return true, sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: err})
}

func terminalOutputCalls(response *sseResponse) ([]sseItem, error) {
	var items []sseItem
	if response == nil {
		return items, nil
	}
	for _, raw := range response.Output {
		var envelope struct {
			Type      string          `json:"type"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Type != "function_call" {
			continue
		}
		var item sseItem
		if len(envelope.Arguments) == 0 || envelope.Arguments[0] != '"' || json.Unmarshal(raw, &item) != nil ||
			strings.TrimSpace(item.CallID) == "" || strings.TrimSpace(item.Name) == "" || (item.Status != "" && item.Status != "completed") {
			return nil, ErrInvalidFunctionCall
		}
		items = append(items, item)
	}
	return items, nil
}

func terminalCallsError(calls map[string]*streamedCall) error {
	closed := make(map[string]bool, len(calls))
	for _, call := range calls {
		if call.completed {
			if strings.TrimSpace(call.id) == "" || strings.TrimSpace(call.name) == "" {
				return ErrUnfinishedFunctionCall
			}
			closed[call.id] = true
		}
	}
	for _, call := range calls {
		// Every observed call must close. Known item aliases share a record;
		// an unrelated completed neighbor cannot prove orphan deltas were closed.
		if !call.completed && !closed[call.id] && (call.announced || call.name != "" ||
			call.argChars > 0 || call.arguments != "") {
			return ErrUnfinishedFunctionCall
		}
	}
	return nil
}

func (c *client) checkTerminalCalls(ctx context.Context, out chan<- provider.Chunk, calls map[string]*streamedCall) bool {
	if err := terminalCallsError(calls); err != nil {
		return c.rejectTerminalCalls(ctx, out, err)
	}
	return true
}

func (c *client) finishTerminalCalls(ctx context.Context, out chan<- provider.Chunk, response *sseResponse, calls map[string]*streamedCall, callForItem func(string) *streamedCall) bool {
	items, err := terminalOutputCalls(response)
	if err != nil {
		return c.rejectTerminalCalls(ctx, out, err)
	}
	for _, item := range items {
		if !c.completeFunctionItem(ctx, out, item, calls, callForItem) {
			return false
		}
	}
	return c.checkTerminalCalls(ctx, out, calls)
}
