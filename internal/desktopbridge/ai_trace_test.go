package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"reasonix/internal/event"
)

type aiTraceRuntime struct{ *fakeRuntime }

func (r *aiTraceRuntime) AITrace(detailed bool) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"path": r.path, "detailed": detailed})
}

func TestAITraceUsesExactBackgroundOwner(t *testing.T) {
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		return &aiTraceRuntime{&fakeRuntime{path: "/tmp/" + request.SessionID, state: "idle"}}, nil
	}))
	defer pool.Shutdown()
	for _, id := range []string{"first", "second"} {
		if _, err := pool.Open(t.Context(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"first", "second"} {
		report, err := pool.AITrace(id, true)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(report, &got) != nil || got.Path != "/tmp/"+id {
			t.Fatalf("wrong owner for %s: %s", id, report)
		}
	}
	if _, err := pool.AITrace("unknown", true); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal("unknown owner", err)
	}
}

func TestAITraceTimingIsBoundedAndSafeDuringPublication(t *testing.T) {
	stream := NewEventStream(8)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := range 80 {
			stream.Sink("one").Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: fmt.Sprint(i), Name: "tool", DurationMs: int64(i), Output: "private body"}})
		}
	}()
	for range 80 {
		_ = stream.AITraceTimings("one")
	}
	workers.Wait()
	timings := stream.AITraceTimings("one")
	if len(timings) != 8 || timings[0].ToolCallID != "72" || timings[7].ToolCallID != "79" {
		t.Fatalf("lost bounded order: %+v", timings)
	}
	if len(stream.AITraceTimings("other")) != 0 {
		t.Fatal("another session's timing leaked")
	}
}
