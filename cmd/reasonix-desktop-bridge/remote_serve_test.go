package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
