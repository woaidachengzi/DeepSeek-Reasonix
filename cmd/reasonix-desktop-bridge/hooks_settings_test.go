package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/hook"
)

func TestPreviewHooksSettingsSaveAndRuntimeLoad(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	globalPath := hook.GlobalSettingsPath("")
	if err := os.WriteFile(globalPath, []byte(`{"theme":"dark","hooks":{"Stop":[{"command":"echo old","env":{"KEEP":"yes"}}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-hooks")
	request := func(method, target, body, id string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	globalURL := "/v1/settings/hooks?scope=global"
	if got := request(http.MethodGet, globalURL, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read = %d", got.Code)
	}
	read := func(target string) previewHooksSettingsView {
		t.Helper()
		w := request(http.MethodGet, target, "", "", true)
		if w.Code != http.StatusOK {
			t.Fatalf("read = %d: %s", w.Code, w.Body.String())
		}
		var view previewHooksSettingsView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	global := read(globalURL)
	if global.Scope != "global" || global.Path != globalPath || !strings.Contains(string(global.Hooks), `"KEEP"`) {
		t.Fatalf("global view = %#v", global)
	}
	post := func(id string, change previewHooksSettingsChange) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(change)
		if err != nil {
			t.Fatal(err)
		}
		return request(http.MethodPost, "/v1/settings/hooks", string(body), id, true)
	}
	globalChange := previewHooksSettingsChange{Scope: "global", Revision: global.Revision,
		Hooks: json.RawMessage(`{"PreToolUse":[{"command":"echo guard","match":"Bash","env":{"KEEP":"yes"}}]}`)}
	if w := post("hooks-global-save", globalChange); w.Code != http.StatusOK {
		t.Fatalf("save global = %d: %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(globalPath)
	if err != nil || !strings.Contains(string(raw), `"theme": "dark"`) || !strings.Contains(string(raw), `"KEEP"`) {
		t.Fatalf("global settings lost fields: %v %s", err, raw)
	}
	loaded := hook.Load(hook.LoadOptions{ProjectRoot: project})
	if len(loaded) != 1 || loaded[0].Event != hook.PreToolUse || loaded[0].Command != "echo guard" {
		t.Fatalf("runtime did not load global hook: %+v", loaded)
	}
	if w := post("hooks-stale", globalChange); w.Code != http.StatusConflict {
		t.Fatalf("stale save = %d: %s", w.Code, w.Body.String())
	}
	projectURL := "/v1/settings/hooks?scope=project&workspaceRoot=" + url.QueryEscape(project)
	projectView := read(projectURL)
	if projectView.Revision != "missing" || projectView.ProjectRoot != project {
		t.Fatalf("new project view = %#v", projectView)
	}
	projectChange := previewHooksSettingsChange{Scope: "project", WorkspaceRoot: project, Revision: projectView.Revision,
		Hooks: json.RawMessage(`{"Stop":[{"command":"echo project"}]}`)}
	if w := post("hooks-project-save", projectChange); w.Code != http.StatusOK {
		t.Fatalf("save project = %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".reasonix", "settings.json")); err != nil {
		t.Fatalf("project hooks file missing: %v", err)
	}
	loaded = hook.Load(hook.LoadOptions{ProjectRoot: project})
	if len(loaded) != 2 || loaded[0].Event != hook.Stop || loaded[0].Scope != hook.ScopeProject || loaded[1].Scope != hook.ScopeGlobal {
		t.Fatalf("runtime project/global hook order = %+v", loaded)
	}
	bad := projectChange
	bad.Revision = read(projectURL).Revision
	bad.Hooks = json.RawMessage(`{"WrongEvent":[{"command":"echo bad"}]}`)
	if w := post("hooks-invalid", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid hook event = %d: %s", w.Code, w.Body.String())
	}
	if got := read(projectURL); !strings.Contains(string(got.Hooks), "echo project") {
		t.Fatalf("invalid save changed project hooks: %s", got.Hooks)
	}
}
