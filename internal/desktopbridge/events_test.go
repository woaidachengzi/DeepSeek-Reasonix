package desktopbridge

import (
	"encoding/json"
	"testing"

	"reasonix/internal/event"
)

func TestEventStreamReplaysTypedEventsAndPublishesLiveFrames(t *testing.T) {
	stream := NewEventStream(4)
	stream.Sink("tab-1").Emit(event.Event{Kind: event.Text, Text: "hello"})

	replay, resyncRequired, live, cancel := stream.Subscribe(0)
	defer cancel()
	if resyncRequired || len(replay) != 1 {
		t.Fatalf("replay = %#v, resync = %v", replay, resyncRequired)
	}
	if replay[0].EventKind != "text" || replay[0].SessionID != "tab-1" || replay[0].Sequence != 1 {
		t.Fatalf("unexpected replay event: %#v", replay[0])
	}
	var payload map[string]any
	if err := json.Unmarshal(replay[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["text"] != "hello" || payload["kind"] != "text" {
		t.Fatalf("unexpected payload: %#v", payload)
	}

	stream.Sink("tab-1").Emit(event.Event{Kind: event.Notice, Text: "ready"})
	got := <-live
	if got.EventKind != "notice" || got.Sequence != 2 {
		t.Fatalf("unexpected live event: %#v", got)
	}
}

func TestEventStreamRequestsResyncOutsideReplayWindow(t *testing.T) {
	stream := NewEventStream(1)
	sink := stream.Sink("tab-1")
	sink.Emit(event.Event{Kind: event.Text, Text: "first"})
	sink.Emit(event.Event{Kind: event.Text, Text: "second"})
	_, resyncRequired, live, cancel := stream.Subscribe(0)
	defer cancel()
	if !resyncRequired || live != nil {
		t.Fatalf("resync = %v, live = %v", resyncRequired, live)
	}
}
