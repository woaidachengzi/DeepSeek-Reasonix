package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewPermissionSettingsPersistRulesAndPreserveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "local/chat"
future_setting = "keep"

[permissions]
mode = "ask"
deny = ["Bash(rm:*)"]
future_permission_setting = "keep"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:11434/v1"
models = ["chat"]
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/permissions", strings.NewReader(body))
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
	for i, body := range []string{
		`{"action":"mode","mode":"deny"}`,
		`{"action":"add","list":"allow","rule":"Bash(go test:*)"}`,
		`{"action":"remove","list":"deny","rule":"Bash(rm:*)"}`,
	} {
		got := request(http.MethodPost, body, "permission-change-"+string(rune('a'+i)), true)
		if got.Code != http.StatusOK {
			t.Fatalf("change %d: %d %s", i, got.Code, got.Body.String())
		}
	}
	viewResponse := request(http.MethodGet, "", "", true)
	var view permissionSettingsView
	if err := json.Unmarshal(viewResponse.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Mode != "deny" || len(view.Allow) != 1 || view.Allow[0] != "Bash(go test:*)" || len(view.Deny) != 0 {
		t.Fatalf("permission view = %#v", view)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_setting", "future_permission_setting", "default_model", "Bash(go test:*)"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config lost %q: %s", want, raw)
		}
	}
	if got := request(http.MethodPost, `{"action":"add","list":"allow","rule":"(noTool)"}`, "permission-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid rule status = %d", got.Code)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
		t.Fatalf("invalid change mutated config: %v", err)
	}
}
