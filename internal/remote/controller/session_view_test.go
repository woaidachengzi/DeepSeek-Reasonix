package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const ownedViewPath = "/remote/中文 &+?#.jsonl"
const validSavedView = `{"protocolVersion":1,"sessionPath":"/remote/中文 &+?#.jsonl","readOnly":true,"ownership":"saved","current":false,"modelRef":"","label":"","history":[{"id":"user-entry","role":"user","content":"你好"},{"id":"answer-entry","role":"assistant","content":"answer","reasoning":"thought","toolCalls":[{"id":"t","name":"read","arguments":"{}"}],"serverSearch":[{"id":"s","query":"q","results":[{"title":"title","url":"https://example.invalid"}],"raw":{"secret":"private-replay"}}],"apiKey":"private-config"},{"id":"result-entry","role":"tool","content":"ok","toolCallId":"t","toolName":"read"}]}`

func sessionViewFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var viewCalls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			_, _ = io.Copy(io.Discard, r.Body)
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "owned" {
			t.Error("no backend cookie")
			w.WriteHeader(401)
			return
		}
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
			t.Error("unexpected mutation or auth header")
		}
		if r.URL.Path == "/sessions" {
			rows := []Session{}
			if listed {
				rows = append(rows, Session{Path: ownedViewPath})
			}
			_ = json.NewEncoder(w).Encode(rows)
			return
		}
		viewCalls.Add(1)
		if r.URL.Path != "/desktop/session-view" || len(r.URL.Query()) != 1 || r.URL.Query().Get("session") != ownedViewPath {
			t.Error("session query encoding or fixed route differed")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "owned")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &viewCalls
}

func TestClientSessionViewScopedTypedHistory(t *testing.T) {
	c, calls := sessionViewFixture(t, true, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, validSavedView) })
	v, err := c.SessionView(context.Background(), ownedViewPath)
	if err != nil || len(v.History) != 3 || v.History[1].Reasoning != "thought" || len(v.History[1].ToolCalls) != 1 || len(v.History[1].ServerSearch) != 1 || v.History[2].ToolCallID != "t" || calls.Load() != 1 {
		t.Fatalf("typed history: %+v %v", v, err)
	}
	data, _ := json.Marshal(v)
	if strings.Contains(string(data), "private-") || strings.Contains(string(data), `"raw"`) {
		t.Fatal("provider/config fields leaked")
	}
	c, calls = sessionViewFixture(t, false, func(w http.ResponseWriter, r *http.Request) { t.Error("unlisted path reached snapshot") })
	if _, err := c.SessionView(context.Background(), ownedViewPath); !errors.Is(err, ErrSessionNotListed) || calls.Load() != 0 {
		t.Fatal("arbitrary remote path accepted")
	}
	if _, err := c.SessionView(nil, ownedViewPath); !errors.Is(err, ErrClient) {
		t.Fatal("nil context accepted")
	}
	if _, err := c.SessionView(context.Background(), "bad\npath"); !errors.Is(err, ErrResponse) {
		t.Fatal("control path accepted")
	}
}
func TestClientSessionViewRejectsWrongScopeAndBudgets(t *testing.T) {
	for _, body := range []string{
		strings.Replace(validSavedView, `"id":"user-entry",`, "", 1),
		strings.Replace(validSavedView, `"user-entry"`, `""`, 1),
		strings.Replace(validSavedView, `"answer-entry"`, `"user-entry"`, 1),
		strings.Replace(validSavedView, `"user-entry"`, `"bad\nidentity"`, 1),
		strings.Replace(validSavedView, `"user-entry"`, `"`+strings.Repeat("x", 4097)+`"`, 1),
		strings.Replace(validSavedView, ownedViewPath, "/other", 1),
		strings.Replace(validSavedView, `"readOnly":true`, `"readOnly":false`, 1),
		strings.Replace(validSavedView, `"protocolVersion":1`, `"protocolVersion":2`, 1),
		strings.Replace(validSavedView, `"saved"`, `"unknown"`, 1),
		strings.Replace(validSavedView, `"modelRef":""`, `"modelRef":"foreground/model"`, 1),
		strings.Replace(validSavedView, `"role":"user"`, `"role":"unknown"`, 1),
		strings.Replace(validSavedView, `"history":[`, `"history":null,"ignored":[`, 1),
		strings.Replace(validSavedView, `"modelRef":""`, `"runtimeState":{"schemaVersion":1,"phase":"idle"},"modelRef":""`, 1),
		strings.Repeat(" ", (32<<20)+1),
	} {
		t.Run("invalid", func(t *testing.T) {
			c, _ := sessionViewFixture(t, true, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := c.SessionView(context.Background(), ownedViewPath); !errors.Is(err, ErrResponse) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
	for _, status := range []int{401, 403, 404, 502} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, calls := sessionViewFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private-diagnostic")
			})
			_, err := c.SessionView(context.Background(), ownedViewPath)
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private") || calls.Load() != 1 || c.Closed() != (status == 401 || status == 403) {
				t.Fatal("error leaked, old server fallback or wrong cookie revocation")
			}
		})
	}
}
func TestClientSessionViewOwnerCancelsBody(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	abort := make(chan struct{})
	c, _ := sessionViewFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-abort:
		}
		close(finished)
	})
	defer close(abort)
	done := make(chan error, 1)
	go func() { _, err := c.SessionView(context.Background(), ownedViewPath); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot did not start")
	}
	c.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revoked read remained live")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("server body was not canceled")
	}
}
