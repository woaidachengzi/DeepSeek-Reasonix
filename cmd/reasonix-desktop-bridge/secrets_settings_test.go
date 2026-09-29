package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/secrets"
)

func TestPreviewSecretsSettingsPersistAndApplyImmediately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Cleanup(func() {
		secrets.SetFilterSubprocessEnv(false)
		secrets.SetProtectSensitiveFiles(false)
	})
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "local/chat"
future_setting = "keep"

[secrets]
filter_subprocess_env = false
protect_sensitive_files = false
future_secret_setting = "keep"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:11434/v1"
models = ["chat"]
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-secrets-settings")
	request := func(method, body string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/secrets", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read status = %d", got.Code)
	}
	changed := request(http.MethodPost, `{"filterSubprocessEnv":true,"protectSensitiveFiles":true}`, true)
	if changed.Code != http.StatusOK {
		t.Fatalf("save secrets settings: %d %s", changed.Code, changed.Body.String())
	}
	if !secrets.FilterSubprocessEnv() || !secrets.ProtectSensitiveFiles() {
		t.Fatal("saved settings did not update runtime protection flags")
	}
	var view secretsSettingsView
	if err := json.Unmarshal(changed.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.FilterSubprocessEnv || !view.ProtectSensitiveFiles || view.ProtocolVersion == 0 {
		t.Fatalf("saved view = %+v", view)
	}
	// Updating one field leaves the other setting enabled.
	changed = request(http.MethodPost, `{"filterSubprocessEnv":false}`, true)
	if changed.Code != http.StatusOK {
		t.Fatalf("partial save: %d %s", changed.Code, changed.Body.String())
	}
	if secrets.FilterSubprocessEnv() || !secrets.ProtectSensitiveFiles() {
		t.Fatal("partial save did not apply only the requested setting")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_setting", "future_secret_setting", "default_model", "protect_sensitive_files = true"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config lost %q: %s", want, raw)
		}
	}
	if invalid := request(http.MethodPost, `{}`, true); invalid.Code != http.StatusBadRequest {
		t.Fatalf("empty change status = %d", invalid.Code)
	}
}
