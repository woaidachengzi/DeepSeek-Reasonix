package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreviewSandboxSettingsPersistAndPreserveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `future_setting = "keep"

[sandbox]
bash = "enforce"
network = true
future_sandbox_setting = "keep"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/sandbox", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got.Code)
	}
	change := `{"bash":"off","network":false,"workspaceRoot":"/tmp/project","allowWrite":["/tmp/extra","/tmp/extra"]}`
	if got := request(http.MethodPost, change, "sandbox-change-a", true); got.Code != http.StatusOK {
		t.Fatalf("save sandbox: %d %s", got.Code, got.Body.String())
	}
	got := request(http.MethodGet, "", "", true)
	var view sandboxSettingsView
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Bash != "off" || view.Network || view.WorkspaceRoot != "/tmp/project" || len(view.AllowWrite) != 1 || view.AllowWrite[0] != "/tmp/extra" || view.Platform != runtime.GOOS {
		t.Fatalf("sandbox view = %#v", view)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_setting", "future_sandbox_setting", "/tmp/project", "/tmp/extra"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config lost %q: %s", want, raw)
		}
	}
	if got := request(http.MethodPost, `{"bash":"wrong","network":true}`, "sandbox-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid request status = %d", got.Code)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
		t.Fatalf("invalid request mutated config: %v", err)
	}
}
