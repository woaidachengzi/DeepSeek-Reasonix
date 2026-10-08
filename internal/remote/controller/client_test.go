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
)

func catalogueServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned-token", Path: "/", HttpOnly: true})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "owned-token" {
			t.Error("missing backend-owned auth cookie")
		}
		if r.URL.Path != "/sessions" || r.Method != http.MethodGet {
			t.Error("unexpected catalogue request")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestClientAttachOperationDoesNotOwnConnection(t *testing.T) {
	s := catalogueServer(t, `[{"name":"a","path":"/remote/a.jsonl","title":"中文","current":true,"apiKey":"owned-secret","url":"http://private"}]`)
	owner, revoke := context.WithCancel(context.Background())
	defer revoke()
	operation, cancel := context.WithCancel(context.Background())
	c, err := Connect(owner, operation, s.URL, "owned-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	cancel()
	entries, err := c.Sessions(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Title != "中文" {
		t.Fatalf("catalogue: %v %v", entries, err)
	}
	data, _ := json.Marshal(entries)
	if strings.Contains(string(data), "owned-secret") || strings.Contains(string(data), "private") {
		t.Fatal("unknown credential fields forwarded")
	}
	revoke()
	if _, err := c.Sessions(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal("revoked owner read succeeded")
	}
	c.Close()
	c.Close()
}

func TestClientCatalogueValidation(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `[{"path":""}]`, `[{"path":"/a","turns":-1}]`, `[{"path":"/a","mtimeMilli":9007199254740992}]`,
		`[{"path":"/a"},{"path":"/a"}]`, `[{"path":"/a","current":true},{"path":"/b","current":true}]`,
		`[{"path":"/a","title":"line\u001bcontrol"}]`, `[{"path":"/a"}] trailing`, `[{"path":"/a","turns":1.5}]`,
		strings.Repeat(" ", (8<<20)+1), `[{"path":"/a","title":"` + strings.Repeat("a", 8193) + `"}]`,
	} {
		t.Run("invalid", func(t *testing.T) {
			s := catalogueServer(t, body)
			c, err := Connect(context.Background(), context.Background(), s.URL, "owned-token")
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := c.Sessions(context.Background()); !errors.Is(err, ErrResponse) {
				t.Fatalf("response accepted: %v", err)
			}
		})
	}
	s := catalogueServer(t, `[]`)
	c, err := Connect(context.Background(), context.Background(), s.URL, "owned-token")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if entries, err := c.Sessions(context.Background()); err != nil || entries == nil || len(entries) != 0 {
		t.Fatal("empty array rejected")
	}
	if _, err := c.Sessions(nil); !errors.Is(err, ErrClient) {
		t.Fatal("nil operation accepted")
	}
	if _, err := Connect(nil, context.Background(), s.URL, "owned-token"); !errors.Is(err, ErrClient) {
		t.Fatal("nil owner accepted")
	}
}

func TestClientExpiredServeCookieRevokesConnection(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth/token" {
					w.WriteHeader(204)
					return
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private diagnostic")
			}))
			defer s.Close()
			c, err := Connect(context.Background(), context.Background(), s.URL, "owned-token")
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Sessions(context.Background())
			if !errors.Is(err, ErrUnavailable) || !c.Closed() || strings.Contains(err.Error(), "private diagnostic") {
				t.Fatal("expired cookie retained or leaked")
			}
		})
	}
}

func TestClientRevocationCancelsHandshakeAndBody(t *testing.T) {
	for _, phase := range []string{"auth", "body"} {
		t.Run(phase, func(t *testing.T) {
			entered := make(chan struct{})
			released := make(chan struct{})
			abort := make(chan struct{})
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Consume the POST body so the server's disconnect watcher can
				// read the socket. Otherwise only this fixture, not the client,
				// can remain blocked on an unread request body after cancellation.
				_, _ = io.Copy(io.Discard, r.Body)
				if r.URL.Path == "/auth/token" && phase != "auth" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if phase == "body" {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				close(entered)
				select {
				case <-r.Context().Done():
				case <-abort:
				}
				close(released)
			}))
			defer s.Close()
			defer close(abort)
			owner, revoke := context.WithCancel(context.Background())
			defer revoke()
			done := make(chan error, 1)
			go func() {
				c, err := Connect(owner, context.Background(), s.URL, "owned-token")
				if err == nil {
					defer c.Close()
					_, err = c.Sessions(context.Background())
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not enter")
			}
			revoke()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("revoked operation succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("revoked operation hung")
			}
			select {
			case <-released:
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP body still alive")
			}
		})
	}
}
