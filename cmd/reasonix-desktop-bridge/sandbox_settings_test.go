package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

[tools.shell]
prefer = "auto"
path = "/bin/bash"
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
	change := `{"bash":"off","network":false,"workspaceRoot":"/tmp/project","allowWrite":["/tmp/extra","/tmp/extra"],"shell":"bash"}`
	if got := request(http.MethodPost, change, "sandbox-change-a", true); got.Code != http.StatusOK {
		t.Fatalf("save sandbox: %d %s", got.Code, got.Body.String())
	}
	got := request(http.MethodGet, "", "", true)
	var view sandboxSettingsView
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Bash != "off" || view.Network || view.WorkspaceRoot != "/tmp/project" || len(view.AllowWrite) != 1 || view.AllowWrite[0] != "/tmp/extra" || view.Platform != runtime.GOOS || view.Shell != "bash" || view.ResolvedShell == "" {
		t.Fatalf("sandbox view = %#v", view)
	}
	if len(view.ShellCapabilities) == 0 || view.GitCapability.ID != "git" {
		t.Fatalf("sandbox response omitted the host capability inventory: %#v", view)
	}
	if _, err := json.Marshal(view); err != nil {
		t.Fatalf("sandbox capability inventory is not serializable: %v", err)
	}
	project := filepath.Join(home, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "reasonix.toml"), []byte("[sandbox]\nallow_write = [\"/tmp/project-extra\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	readProject := func(root string) sandboxSettingsView {
		r := httptest.NewRequest(http.MethodGet, "/v1/settings/sandbox?workspaceRoot="+url.QueryEscape(root), nil)
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("project sandbox read = %d %s", w.Code, w.Body.String())
		}
		var result sandboxSettingsView
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	projectView := readProject(project)
	if len(projectView.EffectiveWriteRoots) != 2 || projectView.EffectiveWriteRoots[0] != "/tmp/project" || projectView.EffectiveWriteRoots[1] != "/tmp/project-extra" || projectView.EffectiveRootsError != "" {
		t.Fatalf("project write roots = %#v, error = %q", projectView.EffectiveWriteRoots, projectView.EffectiveRootsError)
	}
	unavailable := readProject(filepath.Join(home, "missing"))
	if unavailable.Shell != "bash" || unavailable.EffectiveRootsError == "" || len(unavailable.EffectiveWriteRoots) != 0 {
		t.Fatalf("unavailable project hid global settings or stale roots: %#v", unavailable)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_setting", "future_sandbox_setting", "/tmp/project", "/tmp/extra", `prefer = "bash"`, `path = "/bin/bash"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config lost %q: %s", want, raw)
		}
	}
	if got := request(http.MethodPost, `{"bash":"off","network":true,"workspaceRoot":"/tmp/project","allowWrite":["/tmp/extra"]}`, "sandbox-legacy-client", true); got.Code != http.StatusOK {
		t.Fatalf("legacy client save: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "", "", true); got.Code != http.StatusOK {
		t.Fatalf("read after legacy save: %d", got.Code)
	} else if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil || view.Shell != "bash" || !view.Network {
		t.Fatalf("legacy save changed shell preference: view=%#v err=%v", view, err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, `{"bash":"wrong","network":true}`, "sandbox-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid request status = %d", got.Code)
	}
	if got := request(http.MethodPost, `{"bash":"off","network":false,"workspaceRoot":"/tmp/project","allowWrite":[],"shell":"fish"}`, "sandbox-shell-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid shell status = %d", got.Code)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
		t.Fatalf("invalid request mutated config: %v", err)
	}
}
