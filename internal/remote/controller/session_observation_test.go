package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func observationClientFixture(t *testing.T, owner context.Context, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "observation-fixture", Path: "/"})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != "observation-fixture" || r.Method != "POST" || r.URL.Path != "/desktop/session-observation" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("observation changed owner, route or authentication")
		}
		var input SessionObservationRequest
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Scope != pendingFixtureScope() || input.ProtocolVersion != 1 {
			t.Error("scope changed")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(owner, context.Background(), s.URL, "isolated")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestSessionObservationBoundedKindsAndRejectsBody(t *testing.T) {
	for _, raw := range []string{
		`{"protocolVersion":1,"kind":"turn_done"}`,
		`{"protocolVersion":1,"kind":"turn_done","text":"private"}`,
		`{"protocolVersion":1,"kind":"notice"}`,
		`{"protocolVersion":1,"kind":"ready"}`,
		`{"protocolVersion":2,"kind":"turn_done"}`,
		strings.Repeat("x", 1025),
	} {
		t.Run(raw[:min(len(raw), 50)], func(t *testing.T) {
			c := observationClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n"+raw+"\n")
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s, err := c.ObserveSession(ctx, SessionObservationRequest{1, pendingFixtureScope()})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			frame, err := s.Next()
			valid := raw == `{"protocolVersion":1,"kind":"turn_done"}`
			if valid && (err != nil || frame.Kind != "turn_done") || !valid && err == nil {
				t.Fatal(frame, err)
			}
		})
	}
}

func TestSessionObservationOwnerCancellationInterruptsActualHTTP(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan struct{})
	c := observationClientFixture(t, owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	})
	s, err := c.ObserveSession(context.Background(), SessionObservationRequest{1, pendingFixtureScope()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	read := make(chan error, 1)
	go func() { _, err := s.Next(); read <- err }()
	cancel()
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("retired owner returned event")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked reader survived owner")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("actual HTTP request survived owner")
	}
}
