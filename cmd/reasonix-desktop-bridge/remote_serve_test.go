package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
	"reasonix/internal/remote"
	"reasonix/internal/remote/sshtest"
)

func TestRemoteServeStartErrorPreservesReasonAndRedactsCredentials(t *testing.T) {
	w := httptest.NewRecorder()
	writeRemoteServeStartError(w, errors.New("bootstrap: npm install failed: npm: command not found\nAuthorization: Bearer abcdef0123456789abcdef\nPASSWORD=private-password\x1b\x00"))
	var body struct {
		Error struct{ Code, Message, Detail string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 502 || body.Error.Code != "remote_serve_start_failed" || !strings.Contains(body.Error.Detail, "npm: command not found") {
		t.Fatalf("missing failure reason: %s", w.Body)
	}
	for _, secret := range []string{"abcdef0123456789abcdef", "private-password", "\x1b", "\x00"} {
		if strings.Contains(body.Error.Detail, secret) {
			t.Fatalf("unsafe diagnostic contains %q", secret)
		}
	}
	w = httptest.NewRecorder()
	writeRemoteServeStartError(w, errors.New(strings.Repeat("错误", 3000)))
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len([]rune(body.Error.Detail)) != 2049 || !strings.HasSuffix(body.Error.Detail, "…") {
		t.Fatal("diagnostic is not bounded UTF-8")
	}
}

// Exercise the actual HTTP -> SSH/SFTP -> bootstrap -> error envelope without
// installing anything: all exec requests terminate in an owned SSH fixture.
func TestRemoteWorkspaceOpenReturnsBootstrapFailureDetail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, "profile"))
	root := filepath.Join(home, "remote")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	server := sshtest.Start(t, sshtest.Options{SFTPRoot: root, Exec: func(cmd string) (string, string, int) {
		if cmd == "uname -sm" {
			return "Linux x86_64\n", "", 0
		}
		if cmd == "npm i -g reasonix 2>&1" {
			return "sh: npm: command not found\n", "", 127
		}
		return "", "", 0
	}})
	host, err := remote.ResolveHost(nil, "owned@"+server.Addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := remote.New(remote.Options{Host: host, Auth: remote.AuthOptions{DisableAgent: true}, HostKeys: &remote.HostKeyPolicy{
		ManagedPath: filepath.Join(home, "known_hosts"), Prompt: func(context.Context, remote.HostKeyQuestion) (bool, error) { return true, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configpkg.UserConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configpkg.UserConfigPath(), []byte("[[remote.hosts]]\nname = \"owned\"\nhost = \"127.0.0.1\"\nuser = \"owned\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "remote-start-detail")
	t.Cleanup(bridge.remoteSessions.closeAll)
	_, ticket, err := bridge.remoteSessions.begin("owned", context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bridge.remoteSessions.putCurrent("owned", ticket, client, remote.HostKeyQuestion{}) {
		t.Fatal("publish SSH connection")
	}
	bridge.remoteSessions.finish("owned", ticket)
	for _, route := range []string{"/v1/settings/remote/controller/open", "/v1/settings/remote/serve/start"} {
		r := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"name":"owned","workspace":"~"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		if w.Code != 502 || !strings.Contains(w.Body.String(), "remote_serve_start_failed") || !strings.Contains(w.Body.String(), "npm: command not found") {
			t.Fatalf("%s hid bootstrap failure: %d %s", route, w.Code, w.Body)
		}
	}
}

func TestRemoteServeRoutesRequireAuthenticatedConnectedHost(t *testing.T) {
	bridge := newBridgeServer(testToken, "remote-serve-test")
	call := func(path, body string, authorized bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := call("/v1/settings/remote/serve/status", `{"name":"gpu","workspace":"/srv/work"}`, false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", got.Code, http.StatusUnauthorized)
	}
	if got := call("/v1/settings/remote/serve/status", `{"name":"gpu","workspace":""}`, true); got.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace = %d, want %d", got.Code, http.StatusBadRequest)
	}
	for _, path := range []string{
		"/v1/settings/remote/serve/status",
		"/v1/settings/remote/serve/start",
		"/v1/settings/remote/serve/stop",
		"/v1/settings/remote/serve/logs",
		"/v1/settings/remote/controller/open",
	} {
		if got := call(path, `{"name":"gpu","workspace":"/srv/work"}`, true); got.Code != http.StatusConflict {
			t.Fatalf("%s without SSH connection = %d, want %d", path, got.Code, http.StatusConflict)
		}
	}
	if got := call("/v1/settings/remote/serve/logs", `{"name":"gpu","workspace":"/srv/work","tailLines":501}`, true); got.Code != http.StatusConflict {
		// Connection state is checked before remote work or log options so a
		// disconnected renderer learns no detail about remote state.
		t.Fatalf("logs without SSH connection = %d, want %d", got.Code, http.StatusConflict)
	}
}

func TestRemoteControllerURLUsesLoopbackAndFragmentToken(t *testing.T) {
	token := strings.Repeat("a", 64)
	got, err := remoteControllerURL("127.0.0.1:43123", token)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:43123/#token="+token {
		t.Fatalf("controller URL = %q", got)
	}
	if _, err := remoteControllerURL("127.0.0.1", token); err == nil {
		t.Fatal("controller URL accepted endpoint without a port")
	}
	if _, err := remoteControllerURL("192.0.2.1:43123", token); err == nil {
		t.Fatal("controller URL accepted a non-loopback endpoint")
	}
	if _, err := remoteControllerURL("127.0.0.1:43123", ""); err == nil {
		t.Fatal("controller URL accepted an empty token")
	}
}
