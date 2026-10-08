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

func ownedClient(t *testing.T, base string) *http.Client {
	t.Helper()
	client, err := NewHTTPClient(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestBaseRequiresNumericLoopbackHTTPAndExplicitPort(t *testing.T) {
	for _, base := range []string{
		"https://127.0.0.1:1234", "http://localhost:1234", "http://192.0.2.1:1234",
		"http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536",
		"http://private-secret@127.0.0.1:1234", "http://127.0.0.1:1234/?token=private-secret",
		"http://127.0.0.1:1234/#private-secret", "http://127.0.0.1:1234/auth/token",
		"http://127.0.0.1:1234/%2f", "http://127.0.0.1:1234/?", "http://[::1%lo0]:1234",
	} {
		t.Run(base, func(t *testing.T) {
			_, err := NewHTTPClient(base)
			if !errors.Is(err, ErrOrigin) || strings.Contains(err.Error(), "private-secret") {
				t.Fatal("unsafe base accepted or echoed")
			}
		})
	}
	for _, base := range []string{"http://127.0.0.1:1234", "http://127.0.0.1:1234/", "http://[::1]:1234", "http://[::ffff:127.0.0.1]:1234"} {
		client := ownedClient(t, base)
		transport := client.Transport.(*tunnelTransport)
		if transport.inner.Proxy != nil || client.Timeout != 0 {
			t.Fatal("ambient proxy or whole SSE timeout enabled")
		}
	}
}

func TestTokenBodyThenCookieAPIAndSSEWithoutAmbientProxy(t *testing.T) {
	const secret = "owned-private-token"
	var authCalls, apiCalls, proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { proxyCalls.Add(1) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, secret) || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
			t.Error("credential left its expected body/cookie channel")
		}
		if r.URL.Path == "/auth/token" {
			authCalls.Add(1)
			var body struct {
				Token string `json:"token"`
			}
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != secret {
				t.Error("token bootstrap contract differs")
			}
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: secret, Path: "/", HttpOnly: true})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		apiCalls.Add(1)
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != secret {
			t.Error("authenticated cookie missing")
		}
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"kind\":\"ready\"}\n\n")
		} else {
			_, _ = io.WriteString(w, "[]")
		}
	}))
	defer server.Close()
	client := ownedClient(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Handshake(ctx, client, server.URL+"/", secret); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/sessions", "/events"} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || len(data) == 0 {
			t.Fatal("API/SSE body missing")
		}
	}
	if authCalls.Load() != 1 || apiCalls.Load() != 2 || proxyCalls.Load() != 0 {
		t.Fatal("unexpected auth/API/proxy call count")
	}
}

func TestRedirectAndDirectCrossPortCannotLeakCookiesOrToken(t *testing.T) {
	var sinkCalls atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sinkCalls.Add(1) }))
	defer sink.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned-secret", Path: "/", HttpOnly: true})
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := ownedClient(t, server.URL)
	if err := Handshake(context.Background(), client, server.URL, "owned-secret"); !errors.Is(err, ErrHandshake) {
		t.Fatal("redirecting bootstrap accepted")
	}
	// Jar domains ignore ports: it holds this cookie for sink as well. The
	// transport, not the jar, must prevent disclosure on a direct request.
	req, _ := http.NewRequest(http.MethodGet, sink.URL, nil)
	if _, err := client.Do(req); !errors.Is(err, ErrOrigin) {
		t.Fatal("cross-port request accepted")
	}
	req, _ = http.NewRequest(http.MethodGet, server.URL, nil)
	req.Host = "foreign.example"
	if _, err := client.Do(req); !errors.Is(err, ErrOrigin) {
		t.Fatal("foreign Host header accepted")
	}
	if err := Handshake(context.Background(), client, sink.URL, "owned-secret"); !errors.Is(err, ErrClient) {
		t.Fatal("cross-port bootstrap accepted")
	}
	if sinkCalls.Load() != 0 {
		t.Fatal("sink received credentials")
	}
}

func TestHandshakeBodyBudgetAndErrorPrivacy(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "bounded", true: "oversized"}[oversized], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				body := "private-body-sentinel"
				if oversized {
					body = strings.Repeat(body, authResponseMax)
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := ownedClient(t, server.URL)
			err := Handshake(context.Background(), client, server.URL, "owned-token-sentinel")
			want := ErrHandshake
			if oversized {
				want = ErrAuthResponse
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), server.URL) {
				t.Fatal("response budget/privacy differs")
			}
		})
	}
}

func TestHandshakeCancellationStopsStalledBody(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := ownedClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Handshake(ctx, client, server.URL, "owned-token") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("owned handshake did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrHandshake) {
			t.Fatal("cancellation classification missing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stalled body survived cancellation")
	}
}

func TestInvalidTokenOrForeignClientDoesNotMakeRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := ownedClient(t, server.URL)
	for _, token := range []string{"", "a\nb", "a b", "非ASCII", strings.Repeat("x", 4097), strings.Repeat("\\", 4096)} {
		if err := Handshake(context.Background(), client, server.URL, token); !errors.Is(err, ErrHandshake) {
			t.Fatal("invalid token accepted")
		}
	}
	for _, c := range []*http.Client{nil, http.DefaultClient, {Jar: client.Jar}} {
		if err := Handshake(context.Background(), c, server.URL, "owned-token"); !errors.Is(err, ErrClient) {
			t.Fatal("foreign client accepted")
		}
	}
	if err := Handshake(nil, client, server.URL, "owned-token"); !errors.Is(err, ErrClient) {
		t.Fatal("nil context accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid handshake reached a server")
	}
}

func TestSSERequestContextOwnsStreamCancellation(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	client := ownedClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	frame := make([]byte, len("data: ready\n\n"))
	if _, err := io.ReadFull(resp.Body, frame); err != nil || string(frame) != "data: ready\n\n" {
		t.Fatal("initial SSE frame missing")
	}
	cancel()
	if _, err := resp.Body.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal("SSE body did not observe owner cancellation")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("server SSE survived owner cancellation")
	}
}
