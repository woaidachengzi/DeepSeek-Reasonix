package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

type historyHTTPBoundedRuntime struct{ *historyHTTPFullRuntime }

func (r *historyHTTPBoundedRuntime) HistoryPage(limit int) ([]desktopbridge.HistoryMessage, int) {
	return r.core.HistoryPage(limit)
}

func newHistoryBenchmarkRuntime(core *controllerRuntime) desktopbridge.Runtime {
	return &historyHTTPBoundedRuntime{&historyHTTPFullRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/synthetic/history-bench", state: "idle"}, core: core}}
}

func TestBridgeHistoryPageHTTPTracksAppendRewriteAndReplacement(t *testing.T) {
	messages := historyBenchmarkFixture(1_000, 4096)
	executor, session, handler := historyBenchmarkHost(t, messages)
	assert := func(raw []provider.Message) {
		t.Helper()
		response := historyBenchmarkRead(t, handler)
		var got historyResponse
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		want := projectBridgeHistory(raw)
		start := max(0, len(want)-200)
		if got.Session.ID != "history-bench" || got.StartIndex != start || got.TotalMessages != len(want) || !reflect.DeepEqual(got.Messages, want[start:]) {
			t.Fatalf("HTTP projection differs: total=%d start=%d rows=%d; want total=%d start=%d", got.TotalMessages, got.StartIndex, len(got.Messages), len(want), start)
		}
		if strings.Contains(response.Body.String(), "private-tool-result") || strings.Contains(response.Body.String(), "private-fixture") {
			t.Fatal("private provider payload escaped HTTP history")
		}
	}
	assert(session.Snapshot())
	assert(session.Snapshot())
	session.AddBatch(provider.Message{Role: provider.RoleUser, Content: "follow-up"}, provider.Message{Role: provider.RoleAssistant, Content: "follow-up answer"})
	assert(session.Snapshot())
	rewritten := session.Snapshot()
	rewritten[0].Content = "edited first question"
	session.Replace(rewritten)
	assert(session.Snapshot())
	session.Replace(rewritten[:83])
	assert(session.Snapshot())
	replacement := historyBenchmarkSession(messages)
	executor.SetSession(replacement)
	assert(replacement.Snapshot())
}
