package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
)

func observedScope() OwnedCommandScope {
	return OwnedCommandScope{"session", 1, "/owned/session.jsonl", "controller-instance"}
}

func observedRead(t *testing.T, sub *OwnedEventSubscription) OwnedObservedEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frame, err := sub.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestOwnedEventsPublishedScopeAndIndependentConsumers(t *testing.T) {
	stream := NewOwnedEventStream()
	defer stream.Close()
	source := stream.NewSource()
	scope := observedScope()
	source.Emit(event.Event{Kind: event.Text, Text: "unpublished candidate"})
	if _, err := stream.Subscribe(scope); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if !source.Activate(scope) || !source.Activate(scope) {
		t.Fatal("scope activation/idempotence failed")
	}
	other := stream.NewSource()
	if other.Activate(scope) {
		t.Fatal("two sources owned the same scope")
	}
	stale := scope
	stale.OwnerEpoch++
	if source.Activate(stale) {
		t.Fatal("source adopted another manager generation")
	}
	if _, err := stream.Subscribe(stale); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	first, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	source.Emit(event.Event{Kind: event.Text, Text: "owned text", TurnID: "turn"})
	frame := observedRead(t, first)
	if frame.Scope != scope || frame.Kind != "text" {
		t.Fatal(frame)
	}
	frame.Payload[0] = '!'
	independent := observedRead(t, second)
	var wire map[string]any
	if json.Unmarshal(independent.Payload, &wire) != nil || wire["text"] != "owned text" || wire["turnId"] != "turn" || wire["runtimeEpoch"] != nil {
		t.Fatal("payload was mutated or manufactured an event-routing epoch")
	}
}

func TestOwnedEventsRetireQueuedPrefixAndRejectLateOldSource(t *testing.T) {
	stream := NewOwnedEventStream()
	defer stream.Close()
	scope := observedScope()
	old := stream.NewSource()
	old.Activate(scope)
	sub, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	old.Emit(event.Event{Kind: event.Text, Text: "queued old text"})
	old.Close()
	if _, err := sub.Read(context.Background()); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal("retired prefix was exposed", err)
	}
	if len(sub.frames) != 0 {
		t.Fatal("retired private payload retained")
	}
	if old.Activate(scope) {
		t.Fatal("closed source revived")
	}
	newScope := scope
	newScope.OwnerEpoch++
	newScope.RuntimeEpoch = "replacement-instance"
	current := stream.NewSource()
	current.Activate(newScope)
	reader, err := stream.Subscribe(newScope)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	old.Emit(event.Event{Kind: event.Text, Text: "late old text"})
	current.Emit(event.Event{Kind: event.Text, Text: "new text"})
	frame := observedRead(t, reader)
	if frame.Scope != newScope || strings.Contains(string(frame.Payload), "old text") {
		t.Fatal("old source delivered to new owner")
	}
	old.Close()
	current.Emit(event.Event{Kind: event.Notice, Text: "still live"})
	if observedRead(t, reader).Kind != "notice" {
		t.Fatal("old cleanup retired new subscription")
	}
}

func TestOwnedEventsOverflowAndByteFailureRequireExplicitResnapshot(t *testing.T) {
	stream := NewOwnedEventStream()
	defer stream.Close()
	source := stream.NewSource()
	scope := observedScope()
	source.Activate(scope)
	sub, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	for range ownedObserverBuffer + 1 {
		source.Emit(event.Event{Kind: event.Text, Text: "buffered"})
	}
	if _, err := sub.Read(context.Background()); !errors.Is(err, ErrOwnedObservationGap) {
		t.Fatal("overflow silently kept an incomplete prefix", err)
	}
	if len(sub.frames) != 0 || len(stream.subscribers) != 0 {
		t.Fatal("overflow retained subscription/frames")
	}
	fresh, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	source.Emit(event.Event{Kind: event.Text, Text: strings.Repeat("x", ownedEventBytes+1)})
	if _, err := fresh.Read(context.Background()); !errors.Is(err, ErrOwnedObservationGap) {
		t.Fatal("oversized observation accepted", err)
	}
	rebound, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer rebound.Close()
	source.Emit(event.Event{Kind: event.Text, Text: "explicitly rebound"})
	if observedRead(t, rebound).Scope != scope {
		t.Fatal("new observation lost scope")
	}
}

func TestOwnedEventsBoundedObserversAndClose(t *testing.T) {
	stream := NewOwnedEventStream()
	source := stream.NewSource()
	scope := observedScope()
	source.Activate(scope)
	readers := make([]*OwnedEventSubscription, 0, ownedObserverLimit)
	for range ownedObserverLimit {
		sub, err := stream.Subscribe(scope)
		if err != nil {
			t.Fatal(err)
		}
		readers = append(readers, sub)
	}
	if _, err := stream.Subscribe(scope); !errors.Is(err, ErrOwnedObservationGap) {
		t.Fatal("observer bound exceeded")
	}
	readers[0].Close()
	replacement, err := stream.Subscribe(scope)
	if err != nil {
		t.Fatal("close did not release observer slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source.Emit(event.Event{Kind: event.Text, Text: "queued before cancellation"})
	if _, err := replacement.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(replacement.frames) != 0 {
		t.Fatal("cancelled read retained a stale prefix")
	}
	stream.Close()
	stream.Close()
	if _, err := replacement.Read(context.Background()); !errors.Is(err, ErrOwnedObservationClosed) {
		t.Fatal(err)
	}
	if source.Activate(scope) || len(stream.sources) != 0 || len(stream.subscribers) != 0 {
		t.Fatal("stream shutdown retained owners")
	}
}
