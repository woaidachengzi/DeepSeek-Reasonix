package desktopbridge

import (
	"encoding/json"
	"reasonix/internal/event"
	"testing"
)

// Exercise the complete publish/checkpoint path on synthetic public deltas.
// These measurements exclude model, network and renderer latency.
func BenchmarkEventStreamTextBurst(b *testing.B) {
	for _, test := range []struct {
		name  string
		count int
	}{{"512", 512}, {"2048", 2048}, {"4096", 4096}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				stream := NewEventStream(1024)
				sink := stream.Sink("fixture-session")
				sink.Emit(event.Event{Kind: event.TurnStarted})
				for j := 0; j < test.count; j++ {
					sink.Emit(event.Event{Kind: event.Text, Text: "abcdefgh"})
				}
				view, _ := stream.LiveSnapshot("fixture-session")
				if len(view.Answer.Segments) != 1 || len(view.Answer.Segments[0]) != test.count*8 {
					b.Fatal("wrong live view")
				}
			}
		})
	}
}
func BenchmarkEventLedgerAppend(b *testing.B) {
	for _, test := range []struct {
		name     string
		capacity int
	}{{"capacity1", 1}, {"capacity1024", 1024}} {
		b.Run(test.name, func(b *testing.B) {
			ledger := NewEventLedger(test.capacity)
			value := Event{EventKind: "text", SessionID: "fixture", Payload: json.RawMessage(`{"kind":"text","text":"abcdefgh"}`)}
			for i := 0; i < test.capacity; i++ {
				ledger.Append(value)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ledger.Append(value)
			}
		})
	}
}
