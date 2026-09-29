package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	configpkg "reasonix/internal/config"
	"reasonix/internal/netclient"
)

func TestProviderProbeUsesEphemeralKeyAndOnlyProbesListedModel(t *testing.T) {
	var calls int
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer one-time-key" {
			http.Error(w, "wrong credential", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["model"] != "probe-model" || body["tools"] != nil || body["max_tokens"] != float64(16) {
			t.Errorf("unexpected probe body: %#v", body)
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
			t.Errorf("probe included conversation context: %#v", messages)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	entry := &configpkg.ProviderEntry{Name: "probe", Kind: "openai", BaseURL: server.URL + "/v1", Models: []string{"probe-model"}}
	if err := runProviderProbeWithProxy(context.Background(), entry, "probe-model", "one-time-key", netclient.ProxySpec{}); err != nil {
		t.Fatalf("valid probe failed: %v", err)
	}
	if err := runProviderProbeWithProxy(context.Background(), entry, "unlisted-model", "one-time-key", netclient.ProxySpec{}); err == nil {
		t.Fatal("an unlisted model passed validation")
	}
	if calls != 1 {
		t.Fatalf("probe made %d requests, want one (the unlisted model must be rejected before network access)", calls)
	}
	if entry.APIKey() != "" {
		t.Fatal("the transient key was written into the provider entry")
	}
}

func TestProviderProbeFailureDoesNotEchoRemoteError(t *testing.T) {
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"private upstream details"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	entry := &configpkg.ProviderEntry{Name: "probe", Kind: "openai", BaseURL: server.URL + "/v1", Models: []string{"probe-model"}}
	err := runProviderProbeWithProxy(context.Background(), entry, "probe-model", "bad-key", netclient.ProxySpec{})
	if err == nil || err.Error() != errProviderProbeFailed.Error() {
		t.Fatalf("probe failure should be sanitized, got %v", err)
	}
}

func newProviderProbeTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}
