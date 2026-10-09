package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
)

func runtimeStatesClientFixture(t *testing.T, owner context.Context, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "isolated", Path: "/"})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "isolated" {
			t.Error("missing owned cookie")
		}
		if r.Method != "GET" || r.URL.Path != "/runtime-states" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected route/fallback/mutation")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := Connect(owner, context.Background(), server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func validRuntimeStatesFixture() RuntimeStates {
	return RuntimeStates{SchemaVersion: 1, Epoch: "serve", Revision: 1, Sessions: []RuntimeSessionState{{SessionPath: "/owned/session.jsonl", Ownership: "serve", Current: true, State: event.RuntimeStateSnapshot{SchemaVersion: 1, RuntimeEpoch: "core-instance", Revision: 1, Phase: "idle"}}}}
}

func TestClientRuntimeStatesStrictReadOnlyAndUnknownFieldPrivacy(t *testing.T) {
	client := runtimeStatesClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
		raw, _ := json.Marshal(validRuntimeStatesFixture())
		_, _ = io.WriteString(w, strings.TrimSuffix(string(raw), "}")+",\"apiKey\":\"private-key\",\"history\":\"private-body\"}")
	})
	result, err := client.RuntimeStates(context.Background())
	if err != nil || len(result.Sessions) != 1 {
		t.Fatal(result, err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "private") {
		t.Fatal("unknown secret forwarded")
	}
	if _, err := client.RuntimeStates(nil); !errors.Is(err, ErrClient) {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeStates){
		func(v *RuntimeStates) { v.SchemaVersion = 0 }, func(v *RuntimeStates) { v.Epoch = "" },
		func(v *RuntimeStates) { v.Revision = 0 }, func(v *RuntimeStates) { v.Sessions = nil },
		func(v *RuntimeStates) { v.Sessions[0].State.RuntimeEpoch = "" },
		func(v *RuntimeStates) { v.Sessions[0].Ownership = "" },
		func(v *RuntimeStates) { v.Sessions[0].Ownership = "saved" },
		func(v *RuntimeStates) { v.Sessions[0].State.Revision = 0 },
		func(v *RuntimeStates) { v.Sessions[0].State.Phase = "unknown" },
		func(v *RuntimeStates) { v.Sessions[0].SessionPath = "bad\npath" },
		func(v *RuntimeStates) { v.Sessions = append(v.Sessions, v.Sessions[0]) },
		func(v *RuntimeStates) {
			v.Sessions = append(v.Sessions, v.Sessions[0])
			v.Sessions[1].SessionPath += "-other"
		},
		func(v *RuntimeStates) { v.Sessions = make([]RuntimeSessionState, 129) },
		func(v *RuntimeStates) {
			v.Sessions = append(v.Sessions, v.Sessions[0])
			v.Sessions[1].SessionPath += "-second"
			v.Sessions[1].State.RuntimeEpoch += "-second"
		},
	} {
		value := validRuntimeStatesFixture()
		mutate(&value)
		c := runtimeStatesClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(value) })
		if _, err := c.RuntimeStates(context.Background()); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid snapshot accepted", err)
		}
	}
}

func TestClientRuntimeStatesOwnerCancellationAndUnsupportedNoFallback(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, abort := make(chan struct{}), make(chan struct{})
	defer close(abort)
	client := runtimeStatesClientFixture(t, owner, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-abort:
		}
	})
	done := make(chan error, 1)
	go func() { _, err := client.RuntimeStates(context.Background()); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime state read not started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner cancellation did not stop read")
	}
	for _, code := range []int{404, 401} {
		c := runtimeStatesClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		if _, err := c.RuntimeStates(context.Background()); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		if c.Closed() != (code == 401) {
			t.Fatal("auth rejection did not retire owner")
		}
	}
}
