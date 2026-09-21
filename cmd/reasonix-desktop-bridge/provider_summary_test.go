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
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"https://api.deepseek.com/v1",
		"REASONIX_TAURI_SUMMARY_TEST_KEY",
		"apiKeyEnv",
		"baseUrl",
		"deepseek-reasoner",
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
