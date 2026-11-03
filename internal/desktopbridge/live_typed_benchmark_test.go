package desktopbridge

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"reasonix/internal/event"
)

func eventTypedBenchmarkFixture(kind string) event.Event {
	if kind == "Text" {
		return event.Event{Kind: event.Text, Text: "abcdefgh"}
	}
	bytes := 4 << 10
	if kind == "Tool1MiB" {
		bytes = 1 << 20
	}
	return event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "fixture-call", Name: "read_file", Args: `{"path":"synthetic-private-file"}`, Output: strings.Repeat("x", bytes)}}
}

func eventTypedBenchmarkStream(b *testing.B, sessions int, input event.Event) (*EventStream, []event.Sink) {
	b.Helper()
	stream := NewEventStream(32)
	sinks := make([]event.Sink, sessions)
	for i := range sinks {
		sinks[i] = stream.Sink(fmt.Sprintf("fixture-%d", i))
		sinks[i].Emit(event.Event{Kind: event.TurnStarted})
	}
	// Fill and wrap the replay ring before timing either implementation.
	for i := 0; i < 64; i++ {
		sinks[i%sessions].Emit(input)
	}
	return stream, sinks
}

// Per-operation cost of the full typed Sink, JSON wire encoding, ledger copy,
// checkpoint and optional subscriber channel. No model, socket or renderer.
func BenchmarkEventTypedPublish(b *testing.B) {
	for _, kind := range []string{"Text", "Tool4KiB", "Tool1MiB"} {
		for _, sessions := range []int{1, 16} {
			for _, subscribers := range []int{0, 1} {
				b.Run(fmt.Sprintf("%s/Sessions%d/Subscribers%d", kind, sessions, subscribers), func(b *testing.B) {
					input := eventTypedBenchmarkFixture(kind)
					stream, sinks := eventTypedBenchmarkStream(b, sessions, input)
					var live <-chan Event
					if subscribers != 0 {
						_, resync, channel, cancel := stream.Subscribe(stream.LatestSequence())
						if resync {
							b.Fatal("fixture subscription cannot resume")
						}
						live = channel
						defer cancel()
					}
					emit := func(sink event.Sink, input event.Event) {
						sink.Emit(input)
						if live != nil {
							<-live
						}
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						// Keep text inside a live response rather than benchmarking
						// only the already-truncated checkpoint's empty append.
						if kind == "Text" && i != 0 && i%(256*sessions) == 0 {
							for _, sink := range sinks {
								emit(sink, event.Event{Kind: event.TurnStarted})
							}
						}
						emit(sinks[i%sessions], input)
					}
					b.StopTimer()
					for i := range sinks {
						view, _ := stream.LiveSnapshot(fmt.Sprintf("fixture-%d", i))
						if !view.Active || kind != "Text" && (len(view.Progress.Tools) != 1 || view.Progress.Tools[0].State != "done") {
							b.Fatal("benchmark lost the live projection")
						}
					}
				})
			}
		}
	}
}

// Run separately with Go's -test.mutexprofile and -test.mutexprofilefraction=1
// to diagnose the publication lock. Profiling is not part of timing samples.
func BenchmarkEventTypedConcurrent(b *testing.B) {
	for _, kind := range []string{"Tool4KiB", "Tool1MiB"} {
		for _, sessions := range []int{1, 16} {
			for _, subscribers := range []int{0, 1} {
				b.Run(fmt.Sprintf("%s/Sessions%d/Subscribers%d", kind, sessions, subscribers), func(b *testing.B) {
					input := eventTypedBenchmarkFixture(kind)
					stream, sinks := eventTypedBenchmarkStream(b, sessions, input)
					var cancel func()
					var stop chan struct{}
					var drained sync.WaitGroup
					if subscribers != 0 {
						_, resync, channel, unsubscribe := stream.Subscribe(stream.LatestSequence())
						if resync {
							b.Fatal("fixture subscription cannot resume")
						}
						cancel = unsubscribe
						stop = make(chan struct{})
						drained.Add(1)
						go func() {
							defer drained.Done()
							for {
								select {
								case <-channel:
								case <-stop:
									return
								}
							}
						}()
					}
					var next atomic.Uint64
					b.ReportAllocs()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							sinks[(next.Add(1)-1)%uint64(len(sinks))].Emit(input)
						}
					})
					b.StopTimer()
					if cancel != nil {
						cancel()
						close(stop)
						drained.Wait()
					}
				})
			}
		}
	}
}
