package desktopbridge

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

func TestLivePreparationBoundsOversizedInvalidUTF8(t *testing.T) {
	input := strings.Repeat("\xff", 4<<20)
	text := prepareLiveText(input)
	if !utf8.ValidString(text) || text != strings.Repeat("\uFFFD", LiveAnswerRuneLimit+1) {
		t.Fatal("bounded text preparation differs from JSON replacement/rune-prefix semantics")
	}
	tool := prepareLiveTool("tool_result", input, input, true)
	if !tool.valid || tool.tool.ID != strings.Repeat("\uFFFD", 256) || tool.tool.Name != strings.Repeat("\uFFFD", 120) || tool.tool.State != "failed" {
		t.Fatal("tool preparation lost per-byte replacement, bounds or error state")
	}
}

type legacyLiveCheckpoint struct {
	view     LiveState
	runes    int
	sequence uint64
	answer   strings.Builder
}

// Frozen 0.1.72 JSON-driven checkpoint logic, independent of the typed update
// extraction and application path under test.
type legacyLiveOracle struct {
	live     map[string]*legacyLiveCheckpoint
	sequence uint64
}

func newLegacyLiveOracle() *legacyLiveOracle {
	return &legacyLiveOracle{live: make(map[string]*legacyLiveCheckpoint)}
}

func legacyEmptyLive() LiveState {
	return LiveState{Answer: LiveAnswer{Segments: []string{}}, Progress: LiveProgress{Phase: "thinking", Tools: []LiveTool{}}}
}

func legacyLiveClip(value string, limit int) string {
	if utf8.RuneCountInString(value) > limit {
		return string([]rune(value)[:limit])
	}
	return value
}

func (o *legacyLiveOracle) publish(input Event) {
	o.sequence++
	input.Sequence = o.sequence
	checkpoint := o.live[input.SessionID]
	if input.EventKind == "turn_started" {
		if checkpoint == nil && len(o.live) >= liveStateLimit {
			oldestID := ""
			var oldest uint64
			for id, item := range o.live {
				if !item.view.Active && (oldestID == "" || item.sequence < oldest) {
					oldestID, oldest = id, item.sequence
				}
			}
			if oldestID == "" {
				return
			}
			delete(o.live, oldestID)
		}
		checkpoint = &legacyLiveCheckpoint{view: legacyEmptyLive()}
		checkpoint.view.Active = true
		o.live[input.SessionID] = checkpoint
	}
	if checkpoint == nil {
		return
	}
	checkpoint.sequence = input.Sequence
	switch input.EventKind {
	case "turn_done":
		checkpoint.view.Active = false
		checkpoint.view.Answer = LiveAnswer{Segments: []string{}}
		checkpoint.answer.Reset()
		checkpoint.view.Progress.Phase = "finishing"
	case "reasoning":
		checkpoint.view.Progress.Phase = "thinking"
	case "text":
		var payload struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		}
		if json.Unmarshal(input.Payload, &payload) != nil || payload.Kind != "text" {
			return
		}
		remaining := LiveAnswerRuneLimit - checkpoint.runes
		delta := payload.Text
		if utf8.RuneCountInString(delta) > remaining {
			delta = string([]rune(delta)[:remaining])
			checkpoint.view.Truncated = true
		}
		if len(checkpoint.view.Answer.Segments) == 0 {
			checkpoint.view.Answer.Segments = []string{""}
		}
		checkpoint.answer.WriteString(delta)
		checkpoint.view.Answer.Segments[0] = checkpoint.answer.String()
		checkpoint.runes += utf8.RuneCountInString(delta)
		checkpoint.view.Answer.Streaming = true
		checkpoint.view.Progress.Phase = "answering"
	case "tool_dispatch", "tool_result":
		var payload struct {
			Tool *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Err  string `json:"err"`
			} `json:"tool"`
		}
		if json.Unmarshal(input.Payload, &payload) != nil || payload.Tool == nil || payload.Tool.ID == "" {
			return
		}
		state, phase := "running", "tool"
		if input.EventKind == "tool_result" {
			state, phase = "done", "thinking"
			if payload.Tool.Err != "" {
				state = "failed"
			}
		}
		item := LiveTool{ID: legacyLiveClip(payload.Tool.ID, 256), Name: legacyLiveClip(payload.Tool.Name, 120), State: state}
		if strings.TrimSpace(item.Name) == "" {
			item.Name = "工具"
		}
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
		if input.EventKind == "tool_dispatch" {
			checkpoint.view.Answer = LiveAnswer{Segments: []string{}}
			checkpoint.answer.Reset()
			checkpoint.runes = 0
			checkpoint.view.Truncated = false
		}
	}
}

func assertLiveOracle(t *testing.T, stream *EventStream, oracle *legacyLiveOracle, sessions map[string]bool) {
	t.Helper()
	if len(stream.live) != len(oracle.live) {
		t.Fatalf("inventory size=%d want=%d", len(stream.live), len(oracle.live))
	}
	for id := range sessions {
		got, cursor := stream.LiveSnapshot(id)
		want := legacyEmptyLive()
		if checkpoint := oracle.live[id]; checkpoint != nil {
			want = checkpoint.view
		}
		if cursor != oracle.sequence || !reflect.DeepEqual(got, want) {
			t.Fatalf("session %q: cursor=%d want=%d\ngot %#v\nwant %#v", id, cursor, oracle.sequence, got, want)
		}
	}
}

func TestTypedLiveUpdatesMatchLegacyJSONAndWire(t *testing.T) {
	typed, raw := NewEventStream(8), NewEventStream(8)
	oracle := newLegacyLiveOracle()
	sessions := map[string]bool{}
	emit := func(id string, input event.Event) {
		t.Helper()
		sessions[id] = true
		before := oracle.sequence
		typed.Sink(id).Emit(input)
		kind, known := eventwire.KindName(input.Kind)
		payload, err := json.Marshal(eventwire.ToWire(input))
		if known && err == nil {
			frame := Event{EventKind: kind, SessionID: id, Payload: payload}
			raw.publish(frame)
			oracle.publish(frame)
			frames, resync := typed.ledger.After(before)
			if resync || len(frames) != 1 || string(frames[0].Payload) != string(payload) || frames[0].SessionID != id || frames[0].EventKind != kind {
				t.Fatal("typed optimization changed published wire envelope")
			}
		}
		assertLiveOracle(t, typed, oracle, sessions)
		assertLiveOracle(t, raw, oracle, sessions)
	}
	emit("a", event.Event{Kind: event.Text, Text: "before turn"})
	emit("a", event.Event{Kind: event.TurnDone})
	emit("a", event.Event{Kind: event.TurnStarted})
	emit("b", event.Event{Kind: event.TurnStarted})
	emit("a", event.Event{Kind: event.Text, Text: "public <>&中🙂"})
	emit("b", event.Event{Kind: event.Text, Text: string([]byte{'x', 0xff, 0xfe, 0xed, 0xa0, 0x80, 'y'})})
	emit("a", event.Event{Kind: event.Reasoning, Text: "PRIVATE REASONING"})
	emit("a", event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "call", Name: strings.Repeat(" ", 120) + "not-visible", Args: "PRIVATE ARGS"}})
	emit("a", event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "call", Name: "read", Output: strings.Repeat("PRIVATE OUTPUT", 1000), Err: "PRIVATE ERROR"}})
	emit("a", event.Event{Kind: event.Text, Text: "new answer"})
	emit("a", event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "", Name: "ignored"}})
	emit("a", event.Event{Kind: event.ToolProgress, Tool: event.Tool{ID: "progress", Output: "PRIVATE PROGRESS"}})
	emit("a", event.Event{Kind: event.ToolResultPreview, Tool: event.Tool{ID: "preview", Output: "PRIVATE PREVIEW"}})
	emit("a", event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "invalid-encode", ComputerUsePreview: &event.ComputerUsePreview{Cursor: &event.ComputerUseCursor{X: math.NaN()}}}})
	emit("a", event.Event{Kind: event.Kind(-99), Text: "unknown"})
	emit("a", event.Event{Kind: event.Text, Text: strings.Repeat("中", LiveAnswerRuneLimit+3)})
	emit("a", event.Event{Kind: event.TurnDone})
	emit("a", event.Event{Kind: event.Text, Text: "after done without restart"})
	emit("a", event.Event{Kind: event.Notice, Text: "PRIVATE NOTICE"})
	emit("a", event.Event{Kind: event.TurnStarted})
	emit("a", event.Event{Kind: event.Text, Text: ""})
	emit("a", event.Event{Kind: event.Text, Text: strings.Repeat(string([]byte{0xff, 0xfe}), LiveAnswerRuneLimit/2+2)})
	for i := 0; i < 35; i++ {
		id := fmt.Sprintf("tool-%d", i)
		emit("a", event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: id, Name: "tool"}})
		emit("a", event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: id, Name: "tool", Err: strings.Repeat("failure", i%2)}})
	}
	for _, suffix := range []string{"one", "two"} {
		emit("a", event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: strings.Repeat("i", 256) + suffix, Name: strings.Repeat("中", 121) + suffix}})
	}
	emit("a", event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: strings.Repeat("i", 255) + string([]byte{0xff, 0xfe}), Name: string([]byte{0xff, 0xfe}), Err: "\x00"}})
	for i := 0; i < liveStateLimit+2; i++ {
		emit(fmt.Sprintf("active-%d", i), event.Event{Kind: event.TurnStarted})
	}
	for _, id := range []string{"a", "b", "active-0"} {
		emit(id, event.Event{Kind: event.TurnDone})
	}
	emit("a", event.Event{Kind: event.Notice, Text: "sequence affects inactive eviction"})
	for i := 0; i < 5; i++ {
		emit(fmt.Sprintf("replacement-%d", i), event.Event{Kind: event.TurnStarted})
	}
	view, _ := typed.LiveSnapshot("a")
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("live checkpoint retained private event content")
	}
}

func TestRawLiveUpdatesKeepMalformedPayloadSemantics(t *testing.T) {
	stream := NewEventStream(8)
	oracle := newLegacyLiveOracle()
	sessions := map[string]bool{"a": true}
	for _, frame := range []Event{
		{EventKind: "turn_started", Payload: json.RawMessage(`not-json`)},
		{EventKind: "text", Payload: json.RawMessage(`null`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"notice","text":"ignored"}`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"text","text":null}`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"text","text":123}`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"text","text":"before"} garbage`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"text","text":"first","text":"last"}`)},
		{EventKind: "text", Payload: json.RawMessage(`{"kind":"text","text":"\ud800\ud800"}`)},
		{EventKind: "tool_dispatch", Payload: json.RawMessage(`{"tool":null}`)},
		{EventKind: "tool_dispatch", Payload: json.RawMessage(`{"tool":{"id":"","name":"ignored"}}`)},
		{EventKind: "tool_dispatch", Payload: json.RawMessage(`{"kind":"notice","tool":{"id":"call","name":" "}}`)},
		{EventKind: "tool_result", Payload: json.RawMessage(`{"tool":{"id":"call","name":1}}`)},
		{EventKind: "tool_result", Payload: json.RawMessage(`{"tool":{"id":"call","name":"read","err":"bad"}}`)},
		{EventKind: "reasoning", Payload: json.RawMessage(`[]`)},
		{EventKind: "turn_done", Payload: json.RawMessage(`bad-json`)},
	} {
		frame.SessionID = "a"
		stream.publish(frame)
		oracle.publish(frame)
		assertLiveOracle(t, stream, oracle, sessions)
	}
}

func TestTypedLiveToolDoesNotRetainProducerBackingStrings(t *testing.T) {
	private := strings.Repeat("x", 1<<20)
	stream := NewEventStream(1)
	sink := stream.Sink("a")
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: private[123:127], Name: private[800:804], Args: private}})
	view, _ := stream.LiveSnapshot("a")
	if len(view.Progress.Tools) != 1 {
		t.Fatal("missing tool")
	}
	base := uintptr(unsafe.Pointer(unsafe.StringData(private)))
	for _, value := range []string{view.Progress.Tools[0].ID, view.Progress.Tools[0].Name} {
		pointer := uintptr(unsafe.Pointer(unsafe.StringData(value)))
		if pointer >= base && pointer < base+uintptr(len(private)) {
			t.Fatal("small live field keeps the producer's full private backing string alive")
		}
	}
	view.Progress.Tools[0].Name = "consumer mutation"
	again, _ := stream.LiveSnapshot("a")
	if again.Progress.Tools[0].Name != "xxxx" {
		t.Fatal("tool snapshot aliases caller")
	}
}
