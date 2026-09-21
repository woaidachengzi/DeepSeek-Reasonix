package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderSummaryIsAuthenticatedAndRedacted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_TAURI_SUMMARY_TEST_KEY", "ignored-process-environment-value")
	config := `default_model = "deepseek/deepseek-chat"

[[providers]]
name = "deepseek"
display_name = "DeepSeek Private Label"
kind = "openai"
base_url = "https://api.deepseek.com/v1"
api_key_env = "REASONIX_TAURI_SUMMARY_TEST_KEY"
models = ["deepseek-chat", "deepseek-reasoner"]
default = "deepseek-chat"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := newBridgeServer(testToken, "instance-a")
	handler := bridge.handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/providers", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got providerSummaryResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != 1 || got.DefaultModel != "deepseek/deepseek-chat" {
		t.Fatalf("summary = %#v", got)
	}
	var deepseek *providerSummaryEntry
	for i := range got.Providers {
		if got.Providers[i].Name == "deepseek" {
			deepseek = &got.Providers[i]
			break
		}
	}
	if deepseek == nil || deepseek.DisplayName != "DeepSeek Private Label" || deepseek.ModelCount != 2 || !deepseek.RequiresKey || deepseek.Configured {
		t.Fatalf("provider summary = %#v", deepseek)
	}
	if len(deepseek.Models) != 2 || deepseek.Models[0] != "deepseek-chat" || deepseek.Models[1] != "deepseek-reasoner" {
		t.Fatalf("provider model choices = %v", deepseek.Models)
	}
	if err := persistDefaultModel("deepseek/deepseek-chat"); !errors.Is(err, errDefaultModelUnavailable) {
		t.Fatalf("selection without stored key error = %v, want unavailable", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"https://api.deepseek.com/v1",
		"REASONIX_TAURI_SUMMARY_TEST_KEY",
		"apiKeyEnv",
		"baseUrl",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("redacted provider summary contains %q: %s", forbidden, encoded)
		}
	}

	const secretValue = "test-credential-must-not-escape"
	credentials := "REASONIX_TAURI_SUMMARY_TEST_KEY=" + secretValue + "\n"
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte(credentials), 0o600); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("configured status = %d, body = %s", response.Code, response.Body.String())
	}
	var configured providerSummaryResponse
	if err := json.NewDecoder(response.Body).Decode(&configured); err != nil {
		t.Fatal(err)
	}
	foundConfigured := false
	for _, provider := range configured.Providers {
		if provider.Name == "deepseek" && provider.Configured {
			foundConfigured = true
			break
		}
	}
	if !foundConfigured {
		t.Fatalf("configured provider summary = %#v", configured)
	}
	encoded, err = json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretValue) {
		t.Fatalf("provider summary leaked a credential value: %s", encoded)
	}
}

func TestSetDefaultModelEndpointIsAuthenticatedAndIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	config := `default_model = "local/alpha"

[desktop]
provider_access = ["local"]

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:9123/v1"
models = ["alpha", "beta"]
default = "alpha"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := newBridgeServer(testToken, "instance-a")
	request := func(authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/default-model", strings.NewReader(`{"model":"local/beta"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestIDHeader, "model-change-1")
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := request(false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", got, http.StatusUnauthorized)
	}
	first := request(true)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	firstBody := first.Body.String()
	var summary providerSummaryResponse
	if err := json.NewDecoder(first.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if summary.DefaultModel != "local/beta" {
		t.Fatalf("default model = %q, want local/beta", summary.DefaultModel)
	}
	second := request(true)
	if second.Code != http.StatusOK || second.Body.String() != firstBody {
		t.Fatalf("idempotent replay = %d %s; first response was %s", second.Code, second.Body.String(), firstBody)
	}
}

func TestPersistDefaultModelValidatesAndPreservesUnknownConfigFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	config := `default_model = "local/alpha"
future_root_option = "keep-root"

[desktop]
provider_access = ["local"]

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:9123/v1"
models = ["alpha", "beta"]
default = "alpha"
future_provider_option = "keep-provider"
`
	configPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := persistDefaultModel("local/beta"); err != nil {
		t.Fatalf("persist valid model: %v", err)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{"default_model = \"local/beta\"", "future_root_option = \"keep-root\"", "future_provider_option = \"keep-provider\""} {
		if !strings.Contains(string(updated), preserved) {
			t.Fatalf("updated config lost %q:\n%s", preserved, updated)
		}
	}

	if err := persistDefaultModel("local/not-configured"); !errors.Is(err, errDefaultModelUnavailable) {
		t.Fatalf("invalid model error = %v, want unavailable", err)
	}
	updatedAfterReject, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedAfterReject) != string(updated) {
		t.Fatal("invalid model changed the saved config")
	}
}
