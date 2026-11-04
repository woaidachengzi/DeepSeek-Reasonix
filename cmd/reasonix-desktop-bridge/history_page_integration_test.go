package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func (r *historyHTTPBoundedRuntime) HistoryBefore(before, limit int) ([]desktopbridge.HistoryMessage, int) {
	return r.core.HistoryBefore(before, limit)
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

func TestBridgeHistoryBeforeHTTPReturnsContiguousDisplayRows(t *testing.T) {
	_, session, handler := historyBenchmarkHost(t, historyBenchmarkFixture(1_000, 4096))
	want := projectBridgeHistory(session.Snapshot())
	get := func(path string, authorized bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if authorized {
			request.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	path := "/v1/sessions/history-bench/history"
	latest := historyBenchmarkRead(t, handler)
	for _, before := range []int{len(want) - 200, 201, 200, 71, 1} {
		response := get(fmt.Sprintf("%s?before=%d", path, before), true)
		var got historyResponse
		if response.Code != http.StatusOK {
			t.Fatalf("before=%d: status %d", before, response.Code)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		start := max(0, before-200)
		if got.Session.ID != "history-bench" || got.StartIndex != start || got.TotalMessages != len(want) || !reflect.DeepEqual(got.Messages, want[start:before]) {
			t.Fatalf("before=%d: projection differs: total=%d start=%d rows=%d", before, got.TotalMessages, got.StartIndex, len(got.Messages))
		}
		if strings.Contains(response.Body.String(), "private-tool-result") || strings.Contains(response.Body.String(), "private-fixture") {
			t.Fatal("private provider payload escaped older HTTP history")
		}
	}
	if next := historyBenchmarkRead(t, handler); next.Body.String() != latest.Body.String() {
		t.Fatal("older-page requests changed the newest page")
	}
	for _, query := range []string{"before", "before=0", "before=-1", "before=x", "before=1&before=2", "before=%zz", fmt.Sprintf("before=%d", len(want)+1)} {
		if response := get(path+"?"+query, true); response.Code != http.StatusBadRequest {
			t.Fatalf("%q: status %d, want 400", query, response.Code)
		}
	}
	if response := get("/v1/sessions/other/history?before=1", true); response.Code != http.StatusNotFound {
		t.Fatalf("unowned session: status %d, want 404", response.Code)
	}
	if response := get(path+"?before=1", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth: status %d, want 401", response.Code)
	}
}
