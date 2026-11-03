package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/provider"
)

type historyPageController struct {
	*control.Controller
	messages []provider.Message
	path     string
}

func (c *historyPageController) History() []provider.Message { return c.messages }
func (c *historyPageController) SessionPath() string         { return c.path }

func historyPageRequest(s *Server, path, etag string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	s.history(w, r)
	return w
}

func decodeHistoryPage(t *testing.T, w *httptest.ResponseRecorder) historyPage {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("history status=%d: %s", w.Code, w.Body.String())
	}
	var page historyPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestHistoryPageCursorSurvivesAppendsAndRejectsReplacement(t *testing.T) {
	ctrl := &historyPageController{Controller: control.New(control.Options{}), path: "/sessions/a.jsonl"}
	for i := range 11 {
		ctrl.messages = append(ctrl.messages, provider.Message{ID: fmt.Sprint(i), Role: provider.RoleUser, Content: fmt.Sprint(i)})
	}
	s := New(ctrl, NewBroadcaster(), config.ServeConfig{})
	first := decodeHistoryPage(t, historyPageRequest(s, "/history?limit=3", ""))
	if first.StartIndex != 8 || first.EndIndex != 11 || len(first.Messages) != 3 || !first.HasMoreBefore {
		t.Fatalf("first page: %+v", first)
	}
	ctrl.messages = append(ctrl.messages, provider.Message{ID: "appended", Role: provider.RoleAssistant, Content: "new"})
	older := decodeHistoryPage(t, historyPageRequest(s, "/history?limit=3&cursor="+first.BeforeCursor, ""))
	if older.StartIndex != 5 || older.EndIndex != 8 || older.Messages[0].Content != "5" || older.Messages[2].Content != "7" {
		t.Fatalf("append shifted old page: %+v", older)
	}
	ctrl.path = "/sessions/b.jsonl"
	if w := historyPageRequest(s, "/history?limit=3&cursor="+first.BeforeCursor, ""); w.Code != http.StatusConflict {
		t.Fatalf("foreign cursor status=%d", w.Code)
	}
	ctrl.path = "/sessions/a.jsonl"
	ctrl.messages[8].Content = "rewritten with the same ID"
	if w := historyPageRequest(s, "/history?limit=3&cursor="+first.BeforeCursor, ""); w.Code != http.StatusConflict {
		t.Fatalf("rewritten boundary status=%d", w.Code)
	}
}

func TestHistoryPagesKeepToolBatchesTogether(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "earlier"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "a", Name: "read"}, {ID: "b", Name: "read"}}},
		{Role: provider.RoleTool, ToolCallID: "a", Content: "one"},
		{Role: provider.RoleTool, ToolCallID: "b", Content: "two"},
		{Role: provider.RoleAssistant, Content: "answer"},
	}
	page, err := projectHistoryPage(messages, "source", historyPageQuery{limit: 2})
	if err != nil || page.StartIndex != 1 || len(page.Messages) != 4 || len(page.Messages[0].ToolCalls) != 2 {
		t.Fatalf("tool batch split: %+v err=%v", page, err)
	}
	withMetadata := []provider.Message{messages[1], {Role: provider.RoleAssistant, LocalOnly: true, Content: "local note"}, messages[2], messages[3]}
	paired, err := projectHistoryPage(withMetadata, "source", historyPageQuery{limit: 1})
	if err != nil || paired.StartIndex != 0 || len(paired.Messages[0].ToolCalls) != 2 {
		t.Fatalf("local metadata split tool call from result: %+v %v", paired, err)
	}
	before := 3
	if _, err := projectHistoryPage(messages, "source", historyPageQuery{limit: 2, before: &before}); err == nil {
		t.Fatal("manual cursor accepted inside a tool batch")
	}
	huge := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "batch"}}}}
	for range maxHistoryPageMessages {
		huge = append(huge, provider.Message{Role: provider.RoleTool, ToolCallID: "batch"})
	}
	if _, err := projectHistoryPage(huge, "source", historyPageQuery{limit: 1}); err == nil {
		t.Fatal("oversized indivisible batch bypassed hard page limit")
	}
}

func TestHistoryPageCursorRoundTripsMetadataBoundary(t *testing.T) {
	for _, metadata := range []provider.Message{
		{ID: "local", Role: provider.RoleAssistant, LocalOnly: true, Content: "local note"},
		{ID: "pinned", Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "<pinned_context_revision>hidden</pinned_context_revision>"},
	} {
		t.Run(metadata.ID, func(t *testing.T) {
			ctrl := &historyPageController{Controller: control.New(control.Options{}), messages: []provider.Message{
				{ID: "user", Role: provider.RoleUser, Content: "earlier user"},
				{ID: "assistant", Role: provider.RoleAssistant, Content: "earlier answer"},
				metadata,
			}}
			s := New(ctrl, NewBroadcaster(), config.ServeConfig{})
			first := decodeHistoryPage(t, historyPageRequest(s, "/history?limit=1", ""))
			if first.StartIndex != 2 || !first.HasMoreBefore || first.BeforeCursor == "" {
				t.Fatalf("missing metadata boundary: %+v", first)
			}
			if metadata.ID == "pinned" && len(first.Messages) != 0 {
				t.Fatalf("pinned metadata leaked into page: %+v", first)
			}
			older := decodeHistoryPage(t, historyPageRequest(s, "/history?limit=1&cursor="+first.BeforeCursor, ""))
			if older.StartIndex != 1 || len(older.Messages) != 1 || older.Messages[0].Content != "earlier answer" {
				t.Fatalf("generated metadata cursor did not advance: %+v", older)
			}
		})
	}
}

func TestHistoryPageETagAndLegacyArray(t *testing.T) {
	ctrl := &historyPageController{Controller: control.New(control.Options{}), messages: []provider.Message{{Role: provider.RoleUser, Content: "one"}, {Role: provider.RoleUser, Content: "two"}}}
	s := New(ctrl, NewBroadcaster(), config.ServeConfig{})
	w := historyPageRequest(s, "/history?limit=1", "")
	decodeHistoryPage(t, w)
	etag := w.Header().Get("ETag")
	if etag == "" || historyPageRequest(s, "/history?limit=1", etag).Code != http.StatusNotModified {
		t.Fatal("unchanged page did not validate")
	}
	if older := historyPageRequest(s, "/history?limit=1&before=1", etag); older.Code != 200 || older.Header().Get("ETag") == etag {
		t.Fatal("ETag incorrectly shared between pages")
	}
	var legacy []historyMessage
	if err := json.Unmarshal(historyPageRequest(s, "/history", "").Body.Bytes(), &legacy); err != nil || len(legacy) != 2 {
		t.Fatalf("legacy array changed: %v %+v", err, legacy)
	}
	ctrl.messages = nil
	page := decodeHistoryPage(t, historyPageRequest(s, "/history?limit=1", ""))
	if page.Messages == nil || page.HasMoreBefore {
		t.Fatalf("empty page: %+v", page)
	}
}

func TestHistoryPageRejectsInvalidParameters(t *testing.T) {
	s := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	for _, query := range []string{
		"limit=", "limit=-1", "limit=0", "limit=501", "limit=922337203685477580899", "limit=2&limit=2",
		"before=0", "cursor=abc", "limit=2&before=-1", "limit=2&before=1&before=1", "limit=2&before=999999", "limit=2&before=922337203685477580899",
		"limit=2&cursor=", "limit=2&cursor=bad", "limit=2&cursor=x&before=0", "limit=2&session=", "limit=2&session=a&session=b",
	} {
		if w := historyPageRequest(s, "/history?"+query, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d", query, w.Code)
		}
	}
	if w := historyPageRequest(s, "/history?limit=2&session="+url.QueryEscape("../../private.jsonl"), ""); w.Code != 404 {
		t.Fatalf("invalid session fell back to foreground: %d", w.Code)
	}
}

func TestHistoryPageProjectsOnlySelectedMessages(t *testing.T) {
	// Every off-page assistant would allocate its own tool-call projection.
	// Comparing allocations catches projecting everything and slicing afterward.
	older := make([]provider.Message, 4000)
	for i := range older {
		older[i] = provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("old", 100), ToolCalls: []provider.ToolCall{{ID: fmt.Sprint(i), Name: "old"}}}
	}
	tail := []provider.Message{{ID: "newest", Role: provider.RoleUser, Content: "visible"}}
	all := append(older, tail...)
	query := historyPageQuery{limit: 1}
	small := testing.AllocsPerRun(5, func() { _, _ = projectHistoryPage(tail, "source", query) })
	large := testing.AllocsPerRun(5, func() { _, _ = projectHistoryPage(all, "source", query) })
	if large > small+20 {
		t.Fatalf("projection scales with unrequested history: small=%g large=%g", small, large)
	}
	page, err := projectHistoryPage(all, "source", query)
	encoded, _ := json.Marshal(page)
	if err != nil || len(page.Messages) != 1 || len(encoded) > 1000 || strings.Contains(string(encoded), "oldold") {
		t.Fatalf("unbounded page: bytes=%d err=%v", len(encoded), err)
	}
}

func TestHistoryPageUsesTheSelectedExternalAndMirroredSource(t *testing.T) {
	f := newOwnershipFixture(t)
	if status, body := f.handoffForce(t, "force"); status != http.StatusOK {
		t.Fatalf("handoff=%d %s", status, body)
	}
	extendSessionOnDisk(t, f.active, "active writer newest")
	page := decodeHistoryPage(t, historyPageRequest(f.server, "/history?limit=1", ""))
	if len(page.Messages) != 1 || page.Messages[0].Content != "active writer newest" {
		t.Fatalf("mirrored page used stale controller: %+v", page)
	}
	other := f.dir + "/other.jsonl"
	saveServeTestSession(t, other)
	extendSessionOnDisk(t, other, "external newest")
	held := &atomic.Bool{}
	held.Store(true)
	withForeignWriterLease(t, other, held)
	path := "/history?limit=1&session=" + url.QueryEscape(other)
	page = decodeHistoryPage(t, historyPageRequest(f.server, path, ""))
	if len(page.Messages) != 1 || page.Messages[0].Content != "external newest" {
		t.Fatalf("spectator page fell back to foreground: %+v", page)
	}
	held.Store(false)
	if w := historyPageRequest(f.server, path, ""); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "active writer") {
		t.Fatalf("unavailable external selector leaked foreground: %d %s", w.Code, w.Body.String())
	}
}

type blockingHistoryWriter struct {
	header  http.Header
	writing chan struct{}
	release chan struct{}
}

func (w *blockingHistoryWriter) Header() http.Header { return w.header }
func (w *blockingHistoryWriter) WriteHeader(int)     {}
func (w *blockingHistoryWriter) Write(p []byte) (int, error) {
	close(w.writing)
	<-w.release
	return len(p), nil
}

func TestHistoryPageDoesNotHoldSessionBindingDuringTransfer(t *testing.T) {
	s := New(control.New(control.Options{}), NewBroadcaster(), config.ServeConfig{})
	w := &blockingHistoryWriter{header: http.Header{}, writing: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.history(w, httptest.NewRequest("GET", "/history?limit=1", nil))
	}()
	<-w.writing
	unlocked := make(chan struct{})
	go func() {
		s.bindMu.Lock()
		s.bindMu.Unlock()
		close(unlocked)
	}()
	select {
	case <-unlocked:
	case <-time.After(time.Second):
		close(w.release)
		<-done
		t.Fatal("slow history response blocked foreground mutations")
	}
	close(w.release)
	<-done
}
