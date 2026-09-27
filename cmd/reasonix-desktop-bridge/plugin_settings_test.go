package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/pluginpkg"
)

func TestPreviewPluginSettingsReadToggleAndRejectStaleOrInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := filepath.Join(home, "plugins", "sample")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"sample","description":"A package"}`
	if err := os.WriteFile(filepath.Join(root, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pluginpkg.Upsert(home, pluginpkg.InstalledPlugin{Name: "sample", Root: "plugins/sample", Source: "https://secret@example.com/plugin.git", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-a").handler()
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/plugins", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read: %d", got.Code)
	}
	read := func() previewPluginSettings {
		t.Helper()
		response := request(http.MethodGet, "", "", true)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret@") {
			t.Fatal("source credential leaked to renderer")
		}
		var view previewPluginSettings
		if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	initial := read()
	if len(initial.Plugins) != 1 || initial.Plugins[0].Status != "ready" || initial.Plugins[0].Source != "remote" || !initial.Plugins[0].Enabled {
		t.Fatalf("initial plugin = %+v", initial.Plugins)
	}
	change := func(id, revision string, enabled bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(previewPluginChange{Name: "sample", Revision: revision, Enabled: enabled})
		return request(http.MethodPost, string(body), id, true)
	}
	if got := change("plugin-disable", initial.Plugins[0].Revision, false); got.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", got.Code, got.Body.String())
	}
	if read().Plugins[0].Enabled {
		t.Fatal("plugin remained enabled")
	}
	if got := change("plugin-stale", initial.Plugins[0].Revision, true); got.Code != http.StatusConflict {
		t.Fatalf("stale revision: %d", got.Code)
	}
	current := read().Plugins[0]
	if err := os.Remove(filepath.Join(root, pluginpkg.NativeManifest)); err != nil {
		t.Fatal(err)
	}
	if got := change("plugin-invalid", current.Revision, true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid plugin enabled: %d", got.Code)
	}
	if read().Plugins[0].Status != "invalid" {
		t.Fatal("invalid plugin status not surfaced")
	}
	if got := request(http.MethodPost, `{"name":"sample","revision":"bad","enabled":true}`, "plugin-bad", true); got.Code != http.StatusBadRequest {
		t.Fatalf("bad request: %d", got.Code)
	}
}
