package serve

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/provider"
	"reasonix/internal/remote/controller"
)

// Uses Serve's real token gate, not a replacement authentication handler.
// API bodies here are fixtures: this does not prove SSH/controller lifecycle.
func TestSharedControllerTransportWithActualServeAuthGate(t *testing.T) {
	const token = "owned-controller-token"
	ag := newAuthGate(config.ServeConfig{AuthMode: "token", Token: token})
	var authenticated atomic.Int32
	server := httptest.NewServer(ag.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated.Add(1)
		if r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("token leaked into URL/header")
		}
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"kind\":\"ready\"}\n\n")
		} else {
			_, _ = io.WriteString(w, "[]")
		}
	})))
	defer server.Close()
	client, err := controller.NewHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	get := func(path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(body)
	}
	if status, _ := get("/status"); status != http.StatusUnauthorized || authenticated.Load() != 0 {
		t.Fatal("unauthenticated API accepted")
	}
	if err := controller.Handshake(ctx, client, server.URL, "wrong-owned-token"); !errors.Is(err, controller.ErrHandshake) {
		t.Fatal("wrong token accepted")
	}
	if err := controller.Handshake(ctx, client, server.URL, token); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/sessions", "/status", "/history", "/events"} {
		status, body := get(path)
		if status != http.StatusOK || body == "" || strings.Contains(body, token) {
			t.Fatal("authenticated API/SSE contract differs")
		}
	}
	if authenticated.Load() != 4 {
		t.Fatal("auth bootstrap reached application or extra API request")
	}
}

// Unlike the transport fixture above, this reads Serve's production catalogue
// backed by real control.Controller and owned transcript files. No provider,
// remote installation, SSH host or user profile is used by this test.
func TestSharedControllerClientReadsActualServeCatalogue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, "profile"))
	if err := os.MkdirAll(filepath.Dir(config.UserConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.UserConfigPath(), []byte("default_model = \"owned-unconfigured-model\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := newOwnershipFixture(t)
	f.server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "owned-token"})
	f.server.titleProv = nil
	// Handler captures the auth gate; assemble after setting the token policy.
	server := httptest.NewServer(f.server.Handler())
	defer server.Close()
	other := filepath.Join(f.dir, "other.jsonl")
	s := agent.NewSession("sys")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "中文第一行\n第二行"})
	if err := s.Save(other); err != nil {
		t.Fatal(err)
	}
	c, err := controller.Connect(context.Background(), context.Background(), server.URL, "owned-token")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	rows, err := c.Sessions(context.Background())
	if err != nil || len(rows) != 2 {
		t.Fatalf("production catalogue: %v %v", rows, err)
	}
	found := false
	multiline := false
	for _, row := range rows {
		if row.Path == agent.CanonicalSessionPath(f.active) {
			found = row.Current && row.Name == "active" && row.Title == "hi" && row.Turns == 1 && !row.Running
		}
		if row.Path == agent.CanonicalSessionPath(other) {
			multiline = row.Title == "中文第一行\n第二行" && !row.Current
		}
	}
	if !found {
		t.Fatalf("current catalogue row mismatch: %+v", rows)
	}
	if f.server.ctl().SessionPath() != f.active {
		t.Fatal("read rebound the remote controller")
	}
	if !multiline {
		t.Fatal("multiline fallback title not preserved")
	}
}
