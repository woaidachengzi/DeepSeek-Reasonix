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

	"reasonix/internal/eventwire"
)

const projectionToken = "0123456789abcdef0123456789abcdef"

func TestProjectionCursorCompactOwnerScopeAndRetry(t *testing.T) {
	c, calls := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != projectionToken || r.URL.Query().Get("after") != "2" || len(r.URL.Query()) != 3 {
			t.Error("cursor leaked history or changed cut")
		}
		json.NewEncoder(w).Encode(ownedProjectionPage())
	})
	previous := ownedProjection()
	cursor, err := c.ProjectionCursor(previous)
	if err != nil || cursor.previous.History != nil || cursor.previous.UserSuffix != nil || cursor.previous.Replay.Events != nil {
		t.Fatal("cursor retained display payload", err)
	}
	encoded, _ := json.Marshal(cursor)
	if string(encoded) != "{}" {
		t.Fatal("cursor serializable", string(encoded))
	}
	previous.PageToken = "changed"
	for range 2 {
		page, err := c.SessionProjectionNext(context.Background(), ownedViewPath, cursor)
		if err != nil || page.Replay.NextAfterSequence != 3 {
			t.Fatal("immutable cursor/retry", err)
		}
	}
	before := calls.Load()
	for _, bad := range []ProjectionCursor{{}, {previous: ownedProjection()}} {
		if _, err := c.SessionProjectionNext(context.Background(), ownedViewPath, bad); !errors.Is(err, ErrProjectionReconcile) {
			t.Fatal("guessed cursor accepted")
		}
	}
	if _, err := c.SessionProjectionNext(context.Background(), "other", cursor); !errors.Is(err, ErrProjectionReconcile) {
		t.Fatal("foreign path accepted")
	}
	other, _ := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) { t.Error("foreign owner dispatched") })
	if _, err := other.SessionProjectionNext(context.Background(), ownedViewPath, cursor); !errors.Is(err, ErrProjectionReconcile) {
		t.Fatal("foreign owner accepted")
	}
	if calls.Load() != before {
		t.Fatal("invalid cursor dispatched")
	}
	c.Close()
	if _, err := c.SessionProjectionNext(context.Background(), ownedViewPath, cursor); !errors.Is(err, ErrClosed) {
		t.Fatal("closed owner accepted", err)
	}
}

func ownedProjection() SessionProjection {
	return SessionProjection{ProtocolVersion: 1, SessionPath: ownedViewPath, ReadOnly: true, Initial: true, History: []HistoryMessage{{ID: "old-user", Role: "user", Content: "old question"}}, UserSuffix: []HistoryMessage{{ID: "new-user", Role: "user", Content: "new question"}}, ActiveTurnID: "owned-turn", TurnStatus: "in_progress", ReplayAfterSequence: 0, PageToken: projectionToken, Replay: ProjectionReplay{FloorSequence: 1, LatestSequence: 3, NextAfterSequence: 2, HasMore: true, RuntimeEpoch: "owned-epoch", Events: []eventwire.Event{{Kind: "text", Text: "owned answer", TurnID: "owned-turn", Sequence: 1, Status: "in_progress", SessionPath: ownedViewPath}, {Kind: "steer", Text: "guidance", ItemID: "owned-inbox", MessageID: "owned-guidance", TurnID: "owned-turn", Sequence: 2, Status: "in_progress", SessionPath: ownedViewPath}}}}
}
func ownedProjectionPage() SessionProjection {
	view := ownedProjection()
	view.Initial = false
	view.History = []HistoryMessage{}
	view.UserSuffix = []HistoryMessage{}
	view.PageToken = ""
	view.Replay.HasMore = false
	view.Replay.NextAfterSequence = 3
	view.Replay.Events = []eventwire.Event{{Kind: "text", Text: "captured tail", TurnID: "owned-turn", Sequence: 3, Status: "in_progress", SessionPath: ownedViewPath}}
	return view
}

func projectionFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "owned" || r.Header.Get("Authorization") != "" {
			t.Error("host-only cookie missing")
		}
		if r.Method != "GET" {
			t.Error("projection performed mutation")
		}
		if r.URL.Path == "/sessions" {
			rows := []Session{}
			if listed {
				rows = append(rows, Session{Path: ownedViewPath})
			}
			json.NewEncoder(w).Encode(rows)
			return
		}
		calls.Add(1)
		if r.URL.Path != "/desktop/session-projection" || r.URL.Query().Get("session") != ownedViewPath {
			t.Error("fixed selected route lost")
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "owned")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &calls
}

func TestSessionProjectionClientTypedFixedCutAndNarrowContinuation(t *testing.T) {
	c, calls := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") == "" {
			if len(q) != 1 {
				t.Error("unexpected initial fields")
			}
			encoded, _ := json.Marshal(ownedProjection())
			var typed map[string]any
			json.Unmarshal(encoded, &typed)
			typed["apiKey"] = "PRIVATE config"
			typed["history"].([]any)[0].(map[string]any)["providerBody"] = "PRIVATE replay"
			json.NewEncoder(w).Encode(typed)
		} else {
			if len(q) != 3 || q.Get("page") != projectionToken || q.Get("after") != "2" {
				t.Error("continuation leaked payload or changed cursor")
			}
			json.NewEncoder(w).Encode(ownedProjectionPage())
		}
	})
	first, err := c.SessionProjection(context.Background(), ownedViewPath)
	if err != nil || first.Replay.Events[1].MessageID != "owned-guidance" || first.Replay.Events[1].ItemID != "owned-inbox" {
		t.Fatalf("first: %v", err)
	}
	data, _ := json.Marshal(first)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("untyped provider/config data forwarded")
	}
	page, err := c.SessionProjectionPage(context.Background(), ownedViewPath, first)
	if err != nil || page.Initial || page.Replay.HasMore || page.Replay.Events[0].Sequence != 3 || calls.Load() != 2 {
		t.Fatalf("page: %v calls=%d", err, calls.Load())
	}
	before := calls.Load()
	if _, err := c.SessionProjectionPage(context.Background(), ownedViewPath, page); !errors.Is(err, ErrResponse) || calls.Load() != before {
		t.Fatal("completed page dispatched another read")
	}
	unlisted, misses := projectionFixture(t, false, func(http.ResponseWriter, *http.Request) { t.Error("unlisted projection reached server") })
	if _, err := unlisted.SessionProjection(context.Background(), ownedViewPath); !errors.Is(err, ErrSessionNotListed) || misses.Load() != 0 {
		t.Fatal("unlisted source was read")
	}
	if _, err := c.SessionProjection(nil, ownedViewPath); !errors.Is(err, ErrClient) {
		t.Fatal("nil operation admitted")
	}
}

func TestSessionProjectionClientRejectsMalformedCutsAndPrivateRows(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SessionProjection)
	}{
		{"path", func(v *SessionProjection) { v.SessionPath = "/foreign" }},
		{"version", func(v *SessionProjection) { v.ProtocolVersion = 2 }},
		{"writable", func(v *SessionProjection) { v.ReadOnly = false }},
		{"mode", func(v *SessionProjection) { v.Initial = false }},
		{"system", func(v *SessionProjection) { v.History[0].Role = "system" }},
		{"duplicate rows", func(v *SessionProjection) { v.UserSuffix[0].ID = v.History[0].ID }},
		{"suffix role", func(v *SessionProjection) { v.UserSuffix[0].Role = "assistant" }},
		{"nil history", func(v *SessionProjection) { v.History = nil }},
		{"nil events", func(v *SessionProjection) { v.Replay.Events = nil }},
		{"floor", func(v *SessionProjection) { v.Replay.FloorSequence = 0 }},
		{"future", func(v *SessionProjection) { v.Replay.LatestSequence = 9007199254740992 }},
		{"gap", func(v *SessionProjection) { v.Replay.Events[0].Sequence = 2 }},
		{"foreign frame", func(v *SessionProjection) { v.Replay.Events[0].SessionPath = "/foreign" }},
		{"terminal status", func(v *SessionProjection) { v.TurnStatus = "completed" }},
		{"unknown frame status", func(v *SessionProjection) { v.Replay.Events[0].Status = "unknown" }},
		{"no progress", func(v *SessionProjection) { v.Replay.Events = []eventwire.Event{}; v.Replay.NextAfterSequence = 0 }},
		{"invalid token", func(v *SessionProjection) { v.PageToken = "url-or-secret" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := ownedProjection()
			tc.mutate(&v)
			c, _ := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(v) })
			if _, err := c.SessionProjection(context.Background(), ownedViewPath); !errors.Is(err, ErrResponse) {
				t.Fatalf("accepted invalid cut: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*SessionProjection)
	}{
		{"epoch", func(v *SessionProjection) { v.Replay.RuntimeEpoch = "new-epoch" }},
		{"turn", func(v *SessionProjection) { v.ActiveTurnID = "new-turn"; v.Replay.Events[0].TurnID = "new-turn" }},
		{"upper cut", func(v *SessionProjection) {
			v.Replay.LatestSequence = 4
			v.Replay.HasMore = true
			v.PageToken = projectionToken
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := ownedProjectionPage()
			tc.mutate(&page)
			c, _ := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(page) })
			if _, err := c.SessionProjectionPage(context.Background(), ownedViewPath, ownedProjection()); !errors.Is(err, ErrProjectionReconcile) {
				t.Fatalf("mixed cut accepted: %v", err)
			}
		})
	}
}

func TestSessionProjectionClientErrorsCancellationAndNoFallback(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 429, 502} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, calls := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, "PRIVATE diagnostic")
			})
			_, err := c.SessionProjection(context.Background(), ownedViewPath)
			want := ErrUnavailable
			if status == 409 {
				want = ErrProjectionReconcile
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "PRIVATE") || calls.Load() != 1 || c.Closed() != (status == 401 || status == 403) {
				t.Fatalf("bad failure policy: %v", err)
			}
		})
	}
	for _, ownerCancel := range []bool{false, true} {
		t.Run("cancel", func(t *testing.T) {
			entered, closed := make(chan struct{}), make(chan struct{})
			c, _ := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "{")
				w.(http.Flusher).Flush()
				close(entered)
				<-r.Context().Done()
				close(closed)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := c.SessionProjection(ctx, ownedViewPath); result <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("read did not start")
			}
			if ownerCancel {
				c.Close()
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if ownerCancel && !errors.Is(err, ErrClosed) {
					t.Fatalf("owner: %v", err)
				}
				if !ownerCancel && !errors.Is(err, context.Canceled) {
					t.Fatalf("operation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancel did not release read")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("body remains open")
			}
		})
	}
}

func TestSessionProjectionClientBodyBudgetsAndZeroDispatch(t *testing.T) {
	for _, body := range []string{"null", "{broken}", "\xff", strings.Repeat(" ", (30<<20)+1)} {
		t.Run("body", func(t *testing.T) {
			c, _ := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
			if _, err := c.SessionProjection(context.Background(), ownedViewPath); !errors.Is(err, ErrResponse) {
				t.Fatalf("invalid body accepted: %v", err)
			}
		})
	}
	c, calls := projectionFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		json.NewEncoder(w).Encode(ownedProjection())
	})
	if _, err := c.SessionProjection(context.Background(), ownedViewPath); !errors.Is(err, ErrResponse) {
		t.Fatal("non-JSON response accepted")
	}
	before := calls.Load()
	for _, previous := range []SessionProjection{{}, func() SessionProjection { v := ownedProjection(); v.PageToken = "secret-url"; return v }(), func() SessionProjection { v := ownedProjection(); v.SessionPath = "/foreign"; return v }()} {
		if _, err := c.SessionProjectionPage(context.Background(), ownedViewPath, previous); !errors.Is(err, ErrResponse) || calls.Load() != before {
			t.Fatal("invalid continuation dispatched read")
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.SessionProjection(cancelled, ownedViewPath); !errors.Is(err, context.Canceled) || calls.Load() != before {
		t.Fatal("precanceled projection dispatched read")
	}
	terminal := ownedProjection()
	terminal.ActiveTurnID = ""
	terminal.TurnStatus = "completed"
	terminal.UserSuffix = []HistoryMessage{}
	terminal.PageToken = ""
	terminal.ReplayAfterSequence = 3
	terminal.Replay.Events = []eventwire.Event{}
	terminal.Replay.NextAfterSequence = 3
	terminal.Replay.HasMore = false
	if !validSessionProjection(terminal, ownedViewPath, 3, true) {
		t.Fatal("settled canonical history must remain readable")
	}
	tooLarge := ownedProjection()
	tooLarge.Replay.Events[0].Text = strings.Repeat("x", sessionEventFrameMax)
	if validSessionProjection(tooLarge, ownedViewPath, 0, true) {
		t.Fatal("oversized individual replay frame accepted")
	}
}

func TestProjectionReplayDecoderBoundsObjectsBeforeTypedAllocation(t *testing.T) {
	for _, raw := range []string{`{"events":null}`, `{"events":{}}`, `{"events":[` + strings.Repeat(`{},`, 512) + `{}]}`, `{"events":[{"private":"` + strings.Repeat("x", sessionEventFrameMax) + `"}]}`} {
		var replay ProjectionReplay
		if err := json.Unmarshal([]byte(raw), &replay); !errors.Is(err, ErrResponse) {
			t.Fatal("unbounded/malformed replay allocation accepted")
		}
		if replay.Events != nil {
			t.Fatal("failed decode published a partial page")
		}
	}
	view := ownedProjection()
	data, _ := json.Marshal(view.Replay)
	var decoded ProjectionReplay
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.LatestSequence != 3 || decoded.RuntimeEpoch != "owned-epoch" || len(decoded.Events) != 2 {
		t.Fatalf("bounded metadata decode: %v", err)
	}
}
