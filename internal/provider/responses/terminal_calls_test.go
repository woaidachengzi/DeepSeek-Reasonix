package responses

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func terminalTestChunks(t *testing.T, events ...string) []provider.Chunk {
	t.Helper()
	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: " + event + "\n\n")
	}
	out := make(chan provider.Chunk, 64)
	c := New(Config{Name: "fixture", BaseURL: "https://example.com", Model: "fixture"}).(*client)
	c.readStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body.String()))}, out, nil)
	var chunks []provider.Chunk
	for chunk := range out {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func TestTerminalCallsRejectMalformedOutput(t *testing.T) {
	for _, output := range []string{
		`{"type":"function_call","call_id":"c","name":"echo","arguments":{"secret":"do not expose"}}`,
		`{"type":"function_call","call_id":"c","name":"echo","arguments":null}`,
		`{"type":"function_call","call_id":"c","name":"echo"}`,
		`{"type":"function_call","name":"echo","arguments":"{}"}`,
		`{"type":"function_call","call_id":"c","arguments":"{}"}`,
		`{"type":"function_call","call_id":"c","name":"echo","arguments":"{}","status":"incomplete"}`,
	} {
		t.Run(output, func(t *testing.T) {
			failed := false
			for _, chunk := range terminalTestChunks(t, `{"type":"response.completed","response":{"output":[`+output+`]}}`) {
				if chunk.Type == provider.ChunkDone {
					t.Fatal("malformed call completed successfully")
				}
				if chunk.Type == provider.ChunkError {
					failed = true
					if !errors.Is(chunk.Err, ErrInvalidFunctionCall) || provider.ClassifyRecovery(chunk.Err).Retryable || strings.Contains(chunk.Err.Error(), "secret") {
						t.Fatalf("unsafe error classification: %v", chunk.Err)
					}
				}
			}
			if !failed {
				t.Fatal("missing protocol error")
			}
		})
	}
}

func TestTerminalCallsRecoverAlternateItemIdentityOnce(t *testing.T) {
	chunks := terminalTestChunks(t,
		`{"type":"response.output_item.added","item":{"id":"old","type":"function_call","call_id":"c","name":"echo"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"old","delta":"{}"}`,
		`{"type":"response.completed","response":{"output":[{"id":"new","type":"function_call","call_id":"c","name":"echo","arguments":"{}"}]}}`)
	calls, done := 0, false
	for _, chunk := range chunks {
		if chunk.Type == provider.ChunkError {
			t.Fatal(chunk.Err)
		}
		if chunk.Type == provider.ChunkToolCall {
			calls++
			if chunk.ToolCall.ID != "c" || chunk.ToolCall.Arguments != "{}" {
				t.Fatal(chunk.ToolCall)
			}
		}
		done = done || chunk.Type == provider.ChunkDone
	}
	if calls != 1 || !done {
		t.Fatalf("calls=%d done=%v", calls, done)
	}
}

func TestTerminalCallsKeepTextOnlyDONE(t *testing.T) {
	for _, chunk := range terminalTestChunks(t, `{"type":"response.output_text.delta","delta":"answer"}`, "[DONE]") {
		if chunk.Type == provider.ChunkError {
			t.Fatal(chunk.Err)
		}
		if chunk.Type == provider.ChunkDone {
			return
		}
	}
	t.Fatal("text-only compatibility lost")
}

func TestTerminalCallsRejectUnidentifiedArgumentsDone(t *testing.T) {
	for _, chunk := range terminalTestChunks(t, `{"type":"response.function_call_arguments.done","item_id":"orphan","arguments":"{}"}`, "[DONE]") {
		if chunk.Type == provider.ChunkDone {
			t.Fatal("unidentified call completed")
		}
		if chunk.Type == provider.ChunkError && errors.Is(chunk.Err, ErrUnfinishedFunctionCall) {
			return
		}
	}
	t.Fatal("missing unfinished-call error")
}

func TestTerminalCallsRejectMalformedIntermediateItem(t *testing.T) {
	for _, chunk := range terminalTestChunks(t,
		`{"type":"response.output_text.delta","delta":"before tool"}`,
		`{"type":"response.output_item.done","item":{"id":"i","type":"function_call","call_id":"c","name":"echo","arguments":{"secret":"hidden"}}}`,
		"[DONE]") {
		if chunk.Type == provider.ChunkDone {
			t.Fatal("malformed intermediate call became a successful answer")
		}
		if chunk.Type == provider.ChunkError && errors.Is(chunk.Err, ErrInvalidFunctionCall) {
			return
		}
	}
	t.Fatal("malformed call was silently ignored")
}

func TestTerminalCallsEmitOnceWhenItemIDChanges(t *testing.T) {
	chunks := terminalTestChunks(t,
		`{"type":"response.output_item.added","item":{"id":"old","type":"function_call","call_id":"c","name":"echo"}}`,
		`{"type":"response.function_call_arguments.done","item_id":"old","arguments":"{}"}`,
		`{"type":"response.output_item.done","item":{"id":"new","type":"function_call","call_id":"c","name":"echo","arguments":"{}","status":"completed"}}`,
		"[DONE]")
	calls := 0
	for _, chunk := range chunks {
		if chunk.Type == provider.ChunkError {
			t.Fatal(chunk.Err)
		}
		if chunk.Type == provider.ChunkToolCall {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("call c emitted %d times", calls)
	}
}

func TestTerminalCallsAllowEquivalentArgumentFormatting(t *testing.T) {
	for _, chunk := range terminalTestChunks(t,
		`{"type":"response.output_item.added","item":{"id":"i","type":"function_call","call_id":"c","name":"echo"}}`,
		`{"type":"response.function_call_arguments.done","item_id":"i","arguments":"{\"a\":1,\"b\":2}"}`,
		`{"type":"response.completed","response":{"output":[{"id":"i","type":"function_call","call_id":"c","name":"echo","arguments":"{ \"b\": 2, \"a\": 1 }"}]}}`) {
		if chunk.Type == provider.ChunkError {
			t.Fatal(chunk.Err)
		}
		if chunk.Type == provider.ChunkDone {
			return
		}
	}
	t.Fatal("equivalent arguments did not complete")
}

func TestTerminalCallsRejectUnidentifiedNeighbor(t *testing.T) {
	for _, chunk := range terminalTestChunks(t,
		`{"type":"response.output_item.done","item":{"id":"i","type":"function_call","call_id":"c","name":"echo","arguments":"{}"}}`,
		`{"type":"response.function_call_arguments.done","item_id":"orphan","arguments":"{}"}`,
		"[DONE]") {
		if chunk.Type == provider.ChunkDone {
			t.Fatal("unidentified call hidden by completed neighbor")
		}
		if chunk.Type == provider.ChunkError && errors.Is(chunk.Err, ErrUnfinishedFunctionCall) {
			return
		}
	}
	t.Fatal("missing unresolved-call error")
}

func TestEquivalentArgumentsPreservesValues(t *testing.T) {
	for _, changed := range []string{`{"n":9007199254740993}`, `{"n":"9007199254740992"}`, `{"n":9007199254740992} {}`} {
		if equivalentArguments(`{"n":9007199254740992}`, changed) {
			t.Errorf("different arguments treated as equal: %s", changed)
		}
	}
}

func TestTerminalCallsRejectMissingOrNullArgumentsDone(t *testing.T) {
	for _, ending := range []string{`"arguments":null`, `"ignored":true`} {
		t.Run(ending, func(t *testing.T) {
			for _, chunk := range terminalTestChunks(t,
				`{"type":"response.output_item.added","item":{"id":"i","type":"function_call","call_id":"c","name":"echo"}}`,
				`{"type":"response.function_call_arguments.delta","item_id":"i","delta":"{}"}`,
				`{"type":"response.function_call_arguments.done","item_id":"i",`+ending+`}`,
				"[DONE]") {
				if chunk.Type == provider.ChunkDone {
					t.Fatal("invalid arguments.done accepted")
				}
				if chunk.Type == provider.ChunkError && errors.Is(chunk.Err, ErrInvalidFunctionCall) {
					return
				}
			}
			t.Fatal("missing protocol error")
		})
	}
}

func TestTerminalCallsRejectOrphanDeltasWithCompleteNeighbor(t *testing.T) {
	for _, chunk := range terminalTestChunks(t,
		`{"type":"response.output_item.done","item":{"id":"i","type":"function_call","call_id":"c","name":"echo","arguments":"{}"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"orphan","delta":"{\"text\":"}`,
		"[DONE]") {
		if chunk.Type == provider.ChunkDone {
			t.Fatal("completed neighbor concealed unfinished invocation")
		}
		if chunk.Type == provider.ChunkError && errors.Is(chunk.Err, ErrUnfinishedFunctionCall) {
			return
		}
	}
	t.Fatal("missing unfinished-call error")
}
