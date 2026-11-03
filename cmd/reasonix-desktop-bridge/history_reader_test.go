package main

import (
	"reflect"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

type controlledHistoryReader struct {
	agent.SessionHistoryReader
	begins, chunks, deltas int
	beforeChunk            func(int)
	buffer                 []provider.Message
}

func (r *controlledHistoryReader) Begin(previous agent.SessionHistoryCursor) (agent.SessionHistoryCursor, int, bool) {
	r.begins++
	return r.SessionHistoryReader.Begin(previous)
}

func (r *controlledHistoryReader) Chunk(target agent.SessionHistoryCursor, start int, scratch []provider.Message) ([]provider.Message, bool) {
	r.chunks++
	if r.beforeChunk != nil {
		r.beforeChunk(r.chunks)
	}
	r.buffer = scratch[:cap(scratch)]
	return r.SessionHistoryReader.Chunk(target, start, scratch)
}

func (r *controlledHistoryReader) Delta(previous agent.SessionHistoryCursor) ([]provider.Message, agent.SessionHistoryCursor, bool) {
	r.deltas++
	return r.SessionHistoryReader.Delta(previous)
}

func assertHistoryReaderPage(t *testing.T, messages []desktopbridge.HistoryMessage, total int, raw []provider.Message) {
	t.Helper()
	want := projectBridgeHistory(raw)
	start := max(0, len(want)-bridgeHistoryCacheCapacity)
	if total != len(want) || !reflect.DeepEqual(messages, append([]desktopbridge.HistoryMessage{}, want[start:]...)) {
		t.Fatalf("reader page differs: total=%d rows=%d; expected total=%d rows=%d", total, len(messages), len(want), len(want)-start)
	}
}

func assertHistoryScratchCleared(t *testing.T, buffer []provider.Message) {
	t.Helper()
	for _, message := range buffer {
		if !reflect.DeepEqual(message, provider.Message{}) {
			t.Fatal("request scratch retained private transcript references")
		}
	}
}

func TestBridgeHistoryReaderChunkBoundaries(t *testing.T) {
	for _, count := range []int{0, 1, 63, 64, 65, 399, 400, 401, 1003} {
		session := historyBenchmarkSession(historyBenchmarkFixture(count, 4096))
		reader := &controlledHistoryReader{SessionHistoryReader: session.HistoryReader()}
		var cache bridgeHistoryCache
		page, total := cache.readPage(reader, 200)
		assertHistoryReaderPage(t, page, total, session.Snapshot())
		assertHistoryScratchCleared(t, reader.buffer)
		chunks := reader.chunks
		page, total = cache.readPage(reader, 200)
		assertHistoryReaderPage(t, page, total, session.Snapshot())
		if reader.chunks != chunks || reader.deltas != 0 {
			t.Fatal("unchanged reads must not revisit raw messages")
		}
	}
}

func TestBridgeHistoryReaderRetriesWithoutPublishingPartialState(t *testing.T) {
	session := historyBenchmarkSession(historyBenchmarkFixture(1000, 0))
	var cache bridgeHistoryCache
	old, _ := cache.readPage(session.HistoryReader(), 200)
	changed := session.Snapshot()
	changed[0].Content = "first rewrite"
	session.Replace(changed)
	reader := &controlledHistoryReader{SessionHistoryReader: session.HistoryReader()}
	reader.beforeChunk = func(call int) {
		if got, _ := cache.snapshot(200); !reflect.DeepEqual(got, old) {
			t.Fatal("partial build replaced the prior cache")
		}
		if call == 10 {
			next := session.Snapshot()
			next[0].Content = "rewrite during the forward pass"
			session.Replace(next)
		}
	}
	page, total := cache.readPage(reader, 200)
	assertHistoryReaderPage(t, page, total, session.Snapshot())
	if reader.begins != 2 || reader.deltas != 0 {
		t.Fatalf("expected one bounded retry: begins=%d deltas=%d", reader.begins, reader.deltas)
	}
	assertHistoryScratchCleared(t, reader.buffer)
}

func TestBridgeHistoryReaderFallsBackAfterTwoInvalidatedBuilds(t *testing.T) {
	session := historyBenchmarkSession(historyBenchmarkFixture(1000, 0))
	var cache bridgeHistoryCache
	old, _ := cache.readPage(session.HistoryReader(), 200)
	session.Replace(session.Snapshot())
	reader := &controlledHistoryReader{SessionHistoryReader: session.HistoryReader()}
	reader.beforeChunk = func(int) {
		if got, _ := cache.snapshot(200); !reflect.DeepEqual(got, old) {
			t.Fatal("failed rebuild damaged current cache")
		}
		session.Replace(session.Snapshot())
	}
	page, total := cache.readPage(reader, 200)
	assertHistoryReaderPage(t, page, total, session.Snapshot())
	if reader.begins != 2 || reader.deltas != 1 || reader.chunks != 2 {
		t.Fatalf("unbounded retry or missing atomic fallback: %+v", reader)
	}
	assertHistoryScratchCleared(t, reader.buffer)
}

func TestBridgeHistoryReaderKeepsFixedPrefixAcrossConcurrentAppend(t *testing.T) {
	session := historyBenchmarkSession(historyBenchmarkFixture(1000, 0))
	before := session.Snapshot()
	reader := &controlledHistoryReader{SessionHistoryReader: session.HistoryReader()}
	reader.beforeChunk = func(call int) {
		if call == 3 {
			session.AddBatch(provider.Message{Role: provider.RoleUser, Content: "new question"}, provider.Message{Role: provider.RoleAssistant, Content: "new answer"})
		}
	}
	var cache bridgeHistoryCache
	page, total := cache.readPage(reader, 200)
	assertHistoryReaderPage(t, page, total, before)
	if reader.begins != 1 || reader.deltas != 0 {
		t.Fatal("pure append should not invalidate the fixed prefix")
	}
	page, total = cache.readPage(reader, 200)
	assertHistoryReaderPage(t, page, total, session.Snapshot())
	if reader.deltas != 1 {
		t.Fatal("next read must include the appended suffix")
	}
	assertHistoryScratchCleared(t, reader.buffer)
}
