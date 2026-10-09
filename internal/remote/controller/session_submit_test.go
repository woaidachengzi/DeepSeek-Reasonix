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

func submitFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
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
		if r.URL.Path != "/desktop/session-submit" || r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("message fell back or leaked identity in URL")
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

func submitTestScope() SessionSubmitScope {
	return SessionSubmitScope{ownedViewPath, "owned-controller-epoch", 7}
}

func TestSubmitSessionTurnSingleDispatchAndPrivateTypedReceipt(t *testing.T) {
	scope, text := submitTestScope(), "用户问题\n/new is literal text"
	c, calls := submitFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			SessionSubmitScope
			Text string `json:"text"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionSubmitScope != scope || input.Text != text {
			t.Error("message changed scope or text")
		}
		_ = json.NewEncoder(w).Encode(struct {
			SessionSubmitReceipt
			Private string `json:"private"`
		}{SessionSubmitReceipt{1, scope.SessionPath, scope.RuntimeEpoch, scope.Revision, true}, "PRIVATE config/prompt"})
	})
	receipt, err := c.SubmitSessionTurn(context.Background(), scope, text)
	if err != nil || receipt.Scope() != scope || !receipt.Accepted || calls.Load() != 1 {
		t.Fatal("send receipt", receipt, err, calls.Load())
	}
	encoded, _ := json.Marshal(receipt)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("receipt leaked unknown payload")
	}
	for _, bad := range []SessionSubmitScope{{}, {ownedViewPath, "", 1}, {ownedViewPath, "epoch", 0}, {ownedViewPath, "epoch", 9_007_199_254_740_992}} {
		if _, err := c.SubmitSessionTurn(context.Background(), bad, text); !errors.Is(err, ErrResponse) {
			t.Fatal("bad scope dispatched", err)
		}
	}
	for _, bad := range []string{"", " \n ", "bad\x00text", string([]byte{0xff}), strings.Repeat("x", (512<<10)+1)} {
		if _, err := c.SubmitSessionTurn(context.Background(), scope, bad); !errors.Is(err, ErrResponse) {
			t.Fatal("bad text dispatched", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("bad input reached mutation")
	}
}

func TestSubmitSessionTurnNoRetryFallbackOrUnlistedMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"changed", 409, "PRIVATE diagnostics", ErrTurnSubmitChanged},
		{"old-server", 404, "", ErrTurnSubmitChanged},
		{"unknown", 500, "PRIVATE endpoint/key", ErrSubmitOutcomeUnknown},
		{"wrong-receipt", 200, `{"protocolVersion":1,"accepted":true,"sessionPath":"other"}`, ErrSubmitOutcomeUnknown},
		{"oversized", 200, strings.Repeat("x", (40<<10)+1), ErrSubmitOutcomeUnknown},
		{"unauthorized", 401, "PRIVATE token", ErrClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, calls := submitFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.SubmitSessionTurn(context.Background(), submitTestScope(), "question"); !errors.Is(err, tc.want) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("bad error", err)
			}
			if calls.Load() != 1 {
				t.Fatal("message was retried", calls.Load())
			}
			if tc.status == 401 && !c.Closed() {
				t.Fatal("revoked authentication retained owner")
			}
		})
	}
	c, calls := submitFixture(t, false, func(w http.ResponseWriter, r *http.Request) { t.Error("unlisted mutation dispatched") })
	if _, err := c.SubmitSessionTurn(context.Background(), submitTestScope(), "question"); !errors.Is(err, ErrSessionNotListed) || calls.Load() != 0 {
		t.Fatal("unlisted scope accepted", err)
	}
	if _, err := c.SubmitSessionTurn(nil, submitTestScope(), "question"); !errors.Is(err, ErrClient) {
		t.Fatal("nil operation accepted")
	}
}

func TestSubmitSessionTurnOwnerRevocationIsUnknownWithoutRetry(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	c, calls := submitFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	result := make(chan error, 1)
	go func() {
		_, err := c.SubmitSessionTurn(context.Background(), submitTestScope(), "question")
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("message not dispatched")
	}
	c.Close()
	select {
	case err := <-result:
		if !errors.Is(err, ErrSubmitOutcomeUnknown) || !errors.Is(err, ErrClosed) {
			t.Fatal("revocation hid uncertain result", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked send did not return")
	}
	if calls.Load() != 1 {
		t.Fatal("revoked send retried")
	}
}
