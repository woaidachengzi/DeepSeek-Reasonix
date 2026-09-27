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

func TestProviderConfigEditPreservesHiddenFieldsAndUsesPreviewProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "custom/old"
future_top_level = "keep"

[desktop]
provider_access = ["custom"]

[[providers]]
name = "custom"
display_name = "Old label"
kind = "openai"
base_url = "https://example.com/v1"
api_key_env = "CUSTOM_SECRET_KEY"
models = ["old"]
default = "old"
future_provider_field = "keep"

[providers.headers]
X-Private-Header = "hidden-value"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-a")
	get := func(url string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, url, nil)
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	if got := get("/v1/settings/provider-configs"); got.Code != http.StatusOK {
		t.Fatalf("get configs: %d %s", got.Code, got.Body.String())
	} else {
		var view providerConfigList
		if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if len(view.Providers) != 1 || view.Providers[0].Name != "custom" || view.Providers[0].DisplayName != "Old label" {
			t.Fatalf("configs = %#v", view)
		}
		for _, secret := range []string{"example.com", "CUSTOM_SECRET_KEY", "hidden-value"} {
			if strings.Contains(got.Body.String(), secret) {
				t.Fatalf("config response leaked %q", secret)
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs", strings.NewReader(`{"name":"custom","displayName":"Updated label","kind":"openai","baseUrl":"","models":["old","new"],"default":"old","useApiKey":false}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(requestIDHeader, "provider-config-edit-1")
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", response.Code, response.Body.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{"future_top_level", "future_provider_field", "hidden-value", "CUSTOM_SECRET_KEY", "https://example.com/v1", "Updated label"} {
		if !strings.Contains(string(raw), preserved) {
			t.Fatalf("edited config lost %q: %s", preserved, raw)
		}
	}
	if got := get("/v1/settings/provider-configs"); !strings.Contains(got.Body.String(), "new") || strings.Contains(got.Body.String(), "hidden-value") {
		t.Fatalf("post-edit config view: %s", got.Body.String())
	}
}

func TestProviderConfigInputRejectsUnsafeEndpoints(t *testing.T) {
	base := saveProviderConfigRequest{Name: "local", Kind: "openai", Models: []string{"model"}, Default: "model"}
	for _, endpoint := range []string{"http://remote.example/v1", "https://user:secret@example.com/v1", "https://example.com/v1?key=secret", "file:///tmp/provider"} {
		input := base
		input.BaseURL = endpoint
		if err := validateProviderConfigInput(&input); err == nil {
			t.Fatalf("accepted unsafe endpoint %q", endpoint)
		}
	}
	input := base
	input.BaseURL = "http://127.0.0.1:11434/v1"
	if err := validateProviderConfigInput(&input); err != nil {
		t.Fatalf("rejected local provider: %v", err)
	}
}

func TestProviderConfigCreateAddsDesktopAccessWithoutChangingOtherSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "local/old"

[desktop]
provider_access = ["local"]
default_tool_approval_mode = "ask"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:11434/v1"
models = ["old"]
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	input := saveProviderConfigRequest{Name: "custom-remote", DisplayName: "Custom Remote", Kind: "openai", BaseURL: "https://provider.example/v1", Models: []string{"chat"}, Default: "chat", UseAPIKey: true}
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"custom-remote", "Custom Remote", "REASONIX_CUSTOM_REMOTE_API_KEY", "default_tool_approval_mode = \"ask\"", "default_model = \"local/old\""} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("created config missing %q: %s", want, raw)
		}
	}
	if !strings.Contains(string(raw), `provider_access = ["local", "custom-remote"]`) {
		t.Fatalf("new provider not added to desktop access: %s", raw)
	}
}
