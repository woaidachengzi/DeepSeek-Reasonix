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

func TestPreviewProjectPermissionEditInheritsGlobalRulesAndStaysProjectScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	globalPath := filepath.Join(home, "config.toml")
	global := `[permissions]
mode = "ask"
allow = ["Bash(existing:*)"]
deny = ["Bash(rm:*)"]
`
	if err := os.WriteFile(globalPath, []byte(global), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "project")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "reasonix.toml")
	project := `[permissions]
ask = ["Edit(src/**)"]
`
	if err := os.WriteFile(projectPath, []byte(project), 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := persistPermissionChange(permissionSettingsChange{
		Action: "add", Scope: "project", WorkspaceRoot: root, List: "allow", Rule: "Bash(go test:*)",
	})
	if err != nil {
		t.Fatalf("save project rule: %v", err)
	}
	if view.Scope != "project" || !view.ProjectOverrides.Allow || view.ProjectOverrides.Mode || len(view.Allow) != 2 || view.Allow[0] != "Bash(existing:*)" || view.Allow[1] != "Bash(go test:*)" {
		t.Fatalf("project permission view = %#v", view)
	}
	if len(view.Ask) != 1 || view.Ask[0] != "Edit(src/**)" || len(view.Deny) != 1 || view.Deny[0] != "Bash(rm:*)" {
		t.Fatalf("project edit did not preserve inherited/explicit lists: %#v", view)
	}
	projectRaw, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projectRaw), "default_model") || !strings.Contains(string(projectRaw), `allow = ["Bash(existing:*)", "Bash(go test:*)"]`) || !strings.Contains(string(projectRaw), `ask = ["Edit(src/**)"]`) {
		t.Fatalf("project config did not contain a minimal permission override: %s", projectRaw)
	}
	globalRaw, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(globalRaw) != global {
		t.Fatalf("global config changed during project save: %s", globalRaw)
	}
	view, err = persistPermissionChange(permissionSettingsChange{
		Action: "remove", Scope: "project", WorkspaceRoot: root, List: "allow", Rule: "Bash(existing:*)",
	})
	if err != nil {
		t.Fatalf("remove inherited rule from project override: %v", err)
	}
	if len(view.Allow) != 1 || view.Allow[0] != "Bash(go test:*)" || !view.ProjectOverrides.Allow {
		t.Fatalf("project override did not retain deliberate removal: %#v", view)
	}
}
