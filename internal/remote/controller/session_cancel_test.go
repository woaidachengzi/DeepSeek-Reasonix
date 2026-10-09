package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cancelFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "owned" || r.Header.Get("Authorization") != "" {
			t.Error("private authentication was lost")
		}
		if r.URL.Path == "/sessions" && r.Method == "GET" {
			rows := []Session{}
			if listed {
				rows = append(rows, Session{Path: ownedViewPath})
			}
			_ = json.NewEncoder(w).Encode(rows)
			return
		}
		calls.Add(1)
		if r.URL.Path != "/desktop/session-cancel" || r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("stop fell back, retried as idempotent, or leaked scope in URL")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &calls
}

func cancelTestScope() SessionCancelScope {
	return SessionCancelScope{ownedViewPath, "owned-controller-epoch", "owned-turn"}
}

func TestCancelSessionTurnSingleTypedDispatchAndReceipt(t *testing.T) {
	scope := cancelTestScope()
	c, calls := cancelFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			SessionCancelScope
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionCancelScope != scope {
			t.Error("stop request changed scope")
		}
		_ = json.NewEncoder(w).Encode(struct {
			SessionCancelReceipt
			Private string `json:"private"`
		}{SessionCancelReceipt{1, scope.SessionPath, scope.RuntimeEpoch, scope.TurnID, true}, "PRIVATE replay/config"})
	})
	receipt, err := c.CancelSessionTurn(context.Background(), scope)
	if err != nil || receipt.Scope() != scope || !receipt.Cancelled || calls.Load() != 1 {
		t.Fatal("stop receipt", receipt, err, calls.Load())
	}
	encoded, _ := json.Marshal(receipt)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("receipt leaked unknown payload")
	}
	for _, bad := range []SessionCancelScope{{}, {ownedViewPath, "", "turn"}, {ownedViewPath, "epoch", "\nturn"}} {
		if _, err := c.CancelSessionTurn(context.Background(), bad); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid scope dispatched", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("invalid input reached mutation")
	}
}

func TestCancelSessionTurnNoFallbackRetryOrUnlistedDispatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"changed", 409, "PRIVATE diagnostics", ErrTurnCancelChanged},
		{"old-server", 404, "", ErrTurnCancelChanged},
		{"unknown-server-error", 500, "PRIVATE endpoint/key", ErrCancelOutcomeUnknown},
		{"wrong-receipt", 200, `{"protocolVersion":1,"cancelled":true,"sessionPath":"other"}`, ErrCancelOutcomeUnknown},
		{"oversized", 200, strings.Repeat("x", (40<<10)+1), ErrCancelOutcomeUnknown},
		{"unauthorized", 401, "PRIVATE token", ErrClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, calls := cancelFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.CancelSessionTurn(context.Background(), cancelTestScope()); !errors.Is(err, tc.want) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("bad error", err)
			}
			if calls.Load() != 1 {
				t.Fatal("mutation was retried", calls.Load())
			}
			if tc.status == 401 && !c.Closed() {
				t.Fatal("revoked authentication retained owner")
			}
		})
	}
	c, calls := cancelFixture(t, false, func(w http.ResponseWriter, r *http.Request) { t.Error("unlisted mutation dispatched") })
	if _, err := c.CancelSessionTurn(context.Background(), cancelTestScope()); !errors.Is(err, ErrSessionNotListed) || calls.Load() != 0 {
		t.Fatal("unlisted scope accepted", err)
	}
	if _, err := c.CancelSessionTurn(nil, cancelTestScope()); !errors.Is(err, ErrClient) {
		t.Fatal("nil operation accepted")
	}
}

func TestCancelSessionTurnRevocationHasUnknownOutcomeAndNoRetry(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	c, calls := cancelFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	result := make(chan error, 1)
	go func() { _, err := c.CancelSessionTurn(context.Background(), cancelTestScope()); result <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("stop was not dispatched")
	}
	c.Close()
	select {
	case err := <-result:
		if !errors.Is(err, ErrCancelOutcomeUnknown) || !errors.Is(err, ErrClosed) {
			t.Fatal("revocation hid uncertain mutation outcome", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked stop did not return")
	}
	if calls.Load() != 1 {
		t.Fatal("revoked stop retried")
	}
}
