package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderCredentialAccountAuthenticatedScopedAndReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	content := []byte(`default_model = "custom/model"
[[providers]]
name = "custom"
kind = "openai"
base_url = "https://example.invalid/v1"
api_key_env = "SHARED_WAILS_KEY"
models = ["model"]
default = "model"
[[providers]]
name = "local"
kind = "ollama"
base_url = "http://127.0.0.1:11434"
models = ["local"]
default = "local"
`)
	configPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(configPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(home, ".env")
	env := []byte("SHARED_WAILS_KEY=fake-source-secret\n")
	if err := os.WriteFile(envPath, env, 0600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "account-test").handler()
	for _, tc := range []struct {
		name   string
		auth   bool
		status int
	}{
		{"custom", false, http.StatusUnauthorized}, {"custom", true, http.StatusOK},
		{"", true, http.StatusBadRequest}, {" custom ", true, http.StatusBadRequest},
		{"SHARED_WAILS_KEY", true, http.StatusBadRequest}, {"unknown", true, http.StatusBadRequest}, {"local", true, http.StatusBadRequest},
	} {
		r := httptest.NewRequest(http.MethodGet, "/v1/settings/provider-credential-account?provider="+url.QueryEscape(tc.name), nil)
		if tc.auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%q auth=%v: status=%d body=%s", tc.name, tc.auth, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "fake-source-secret") || strings.Contains(w.Body.String(), home) {
			t.Fatal("credential endpoint leaked a value or private path")
		}
		if tc.status == http.StatusOK && !strings.Contains(w.Body.String(), `"account":"SHARED_WAILS_KEY"`) {
			t.Fatal("provider was not mapped to its configured account")
		}
	}
	for path, original := range map[string][]byte{configPath: content, envPath: env} {
		actual, err := os.ReadFile(path)
		if err != nil || string(actual) != string(original) {
			t.Fatal("credential metadata read modified originals")
		}
	}
}
