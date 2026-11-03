package desktopbridge

import (
	"encoding/json"
	"fmt"
	"reasonix/internal/event"
	"strings"
	"testing"
)

func TestLiveSnapshotSurvivesInterleavedReplayOverflowWithoutPrivateContent(t *testing.T) {
	stream := NewEventStream(2)
	a, b := stream.Sink("a"), stream.Sink("b")
	a.Emit(event.Event{Kind: event.TurnStarted})
	b.Emit(event.Event{Kind: event.TurnStarted})
	a.Emit(event.Event{Kind: event.Text, Text: "answer A"})
	b.Emit(event.Event{Kind: event.Text, Text: "answer B"})
	a.Emit(event.Event{Kind: event.Reasoning, Text: "private reasoning"})
	a.Emit(event.Event{Kind: event.Notice, Text: "private notice"})
	view, cursor := stream.LiveSnapshot("a")
	if len(view.Answer.Segments) != 1 || view.Answer.Segments[0] != "answer A" || !view.Active || cursor != stream.LatestSequence() {
		t.Fatalf("lost interleaved answer: %+v", view)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "answer B") {
		t.Fatal("checkpoint disclosed non-public or foreign content")
	}
	view.Answer.Segments[0] = "consumer mutation"
	saved, _ := stream.LiveSnapshot("a")
	if saved.Answer.Segments[0] != "answer A" {
		t.Fatal("checkpoint aliases consumer")
	}
	_, resync, _, off := stream.Subscribe(0)
	defer off()
	if !resync {
		t.Fatal("fixture did not overflow replay")
	}
	_, resync, _, off2 := stream.Subscribe(cursor)
	defer off2()
	if resync {
		t.Fatal("checkpoint cursor cannot resume")
	}
}

func TestLiveSnapshotBoundsTextAndResetsCommittedToolResponse(t *testing.T) {
	stream := NewEventStream(8)
	sink := stream.Sink("a")
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Text, Text: strings.Repeat("中", LiveAnswerRuneLimit+10)})
	view, _ := stream.LiveSnapshot("a")
	if !view.Truncated || len([]rune(view.Answer.Segments[0])) != LiveAnswerRuneLimit {
		t.Fatal("unbounded checkpoint")
	}
	sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "tool", Name: "public-tool"}})
	view, _ = stream.LiveSnapshot("a")
	if len(view.Answer.Segments) != 0 || view.Truncated || len(view.Progress.Tools) != 1 {
		t.Fatal("committed tool response duplicated")
	}
	sink.Emit(event.Event{Kind: event.Text, Text: "next answer"})
	view, _ = stream.LiveSnapshot("a")
	if view.Answer.Segments[0] != "next answer" {
		t.Fatal("new model response did not start cleanly")
	}
	sink.Emit(event.Event{Kind: event.TurnDone})
	view, _ = stream.LiveSnapshot("a")
	if view.Active || len(view.Answer.Segments) != 0 {
		t.Fatal("completed answer remained duplicated")
	}
}

func TestLiveCheckpointInventoryIsBounded(t *testing.T) {
	stream := NewEventStream(2)
	for i := 0; i < liveStateLimit*3; i++ {
		sink := stream.Sink(fmt.Sprint(i))
		sink.Emit(event.Event{Kind: event.TurnStarted})
		sink.Emit(event.Event{Kind: event.TurnDone})
	}
	if len(stream.live) != liveStateLimit {
		t.Fatal("checkpoint inventory is unbounded")
	}
}

func TestLiveAnswerSnapshotsRemainImmutableAcrossAppendAndReset(t *testing.T) {
	stream := NewEventStream(2)
	sink := stream.Sink("a")
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Text, Text: "first 中"})
	before, _ := stream.LiveSnapshot("a")
	sink.Emit(event.Event{Kind: event.Text, Text: " second 🙂"})
	appended, _ := stream.LiveSnapshot("a")
	sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "call", Name: "read"}})
	sink.Emit(event.Event{Kind: event.Text, Text: "fresh"})
	fresh, _ := stream.LiveSnapshot("a")
	if before.Answer.Segments[0] != "first 中" || appended.Answer.Segments[0] != "first 中 second 🙂" || fresh.Answer.Segments[0] != "fresh" {
		t.Fatal("answer snapshot aliased a later delta or reset")
	}
	sink.Emit(event.Event{Kind: event.TurnDone})
	done, _ := stream.LiveSnapshot("a")
	if len(done.Answer.Segments) != 0 || done.Active || fresh.Answer.Segments[0] != "fresh" {
		t.Fatal("terminal reset changed an earlier snapshot")
	}
}

func TestLiveAnswerBuilderRetainsUnicodeLimitAcrossTinyDeltas(t *testing.T) {
	stream := NewEventStream(1)
	sink := stream.Sink("a")
	sink.Emit(event.Event{Kind: event.TurnStarted})
	for range LiveAnswerRuneLimit + 3 {
		sink.Emit(event.Event{Kind: event.Text, Text: "🙂"})
	}
	view, _ := stream.LiveSnapshot("a")
	if !view.Truncated || view.Answer.Segments[0] != strings.Repeat("🙂", LiveAnswerRuneLimit) {
		t.Fatal("tiny unicode deltas exceeded the public answer limit")
	}
}
