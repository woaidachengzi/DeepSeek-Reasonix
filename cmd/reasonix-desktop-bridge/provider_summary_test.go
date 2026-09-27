package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
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

func TestSetProviderKeyEndpointUsesPrivateMemoryOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	const providerName = "keychain-test-provider"
	const secretValue = "desktop-keychain-test-secret"
	t.Cleanup(func() { configpkg.ClearDesktopKeychainCredential(providerName) })
	config := `default_model = "keychain-test-provider/chat"

[[providers]]
name = "keychain-test-provider"
kind = "openai"
base_url = "https://api.openai.com/v1"
api_key_env = "KEYCHAIN_TEST_PROVIDER_KEY"
models = ["chat"]
default = "chat"
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := newBridgeServer(testToken, "instance-a")
	request := func(body string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-key", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestIDHeader, "provider-key-test-"+strconv.Itoa(len(body)))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}

	if got := request(`{"providerName":"keychain-test-provider","apiKey":"desktop-keychain-test-secret"}`, false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", got, http.StatusUnauthorized)
	}
	stored := request(`{"providerName":"keychain-test-provider","apiKey":"desktop-keychain-test-secret"}`, true)
	if stored.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", stored.Code, stored.Body.String())
	}
	if strings.Contains(stored.Body.String(), secretValue) {
		t.Fatalf("store response leaked API key: %s", stored.Body.String())
	}
	var summary providerSummaryResponse
	if err := json.NewDecoder(stored.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Providers) != 1 || !summary.Providers[0].Configured {
		t.Fatalf("stored provider summary = %#v", summary)
	}
	coreConfig, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	coreConfig.Providers[0].ResolveAPIKeyForRoot(".")
	if got := coreConfig.Providers[0].APIKey(); got != secretValue {
		t.Fatal("core provider did not resolve the in-memory keychain credential")
	}
	if got := os.Getenv("KEYCHAIN_TEST_PROVIDER_KEY"); got != "" {
		t.Fatal("provider key escaped into the process environment")
	}
	configOnDisk, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configOnDisk), secretValue) {
		t.Fatal("provider key was written to config.toml")
	}

	deleted := request(`{"providerName":"keychain-test-provider","delete":true}`, true)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	if err := json.NewDecoder(deleted.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Providers) != 1 || summary.Providers[0].Configured {
		t.Fatalf("deleted provider summary = %#v", summary)
	}
	coreConfig.Providers[0].ResolveAPIKeyForRoot(".")
	if got := coreConfig.Providers[0].APIKey(); got != "" {
		t.Fatal("core provider retained a deleted keychain credential")
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

func TestModelRoleSettingsPersistWithCapabilityAndAccessChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	configPath := filepath.Join(home, "config.toml")
	config := `default_model = "local/text"
future_root_option = "keep-root"

[desktop]
provider_access = ["local", "search"]

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:9123/v1"
models = ["text", "vision"]
vision_models = ["vision"]
default = "text"
future_provider_option = "keep-provider"

[[providers]]
name = "blocked"
kind = "openai"
base_url = "http://127.0.0.1:9124/v1"
models = ["vision"]
vision_models = ["vision"]
default = "vision"

[[providers]]
name = "search"
kind = "responses"
base_url = "http://127.0.0.1:9125/v1"
models = ["fast"]
default = "fast"
web_search = true
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-a")
	requestNumber := 0
	request := func(body string, authorized bool) *httptest.ResponseRecorder {
		requestNumber++
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/model-role", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestIDHeader, "role-"+strconv.Itoa(requestNumber))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if got := request(`{"role":"planner","model":"local/text"}`, false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthorized role mutation = %d", got)
	}
	for _, body := range []string{
		`{"role":"vision","model":"local/text"}`,
		`{"role":"vision","model":"blocked/vision"}`,
		`{"role":"planner","model":"blocked/vision"}`,
		`{"role":"search","model":"local/text"}`,
		`{"role":"search","model":"blocked/vision"}`,
		`{"role":"other","model":"local/text"}`,
	} {
		if response := request(body, true); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid role %s = %d %s", body, response.Code, response.Body.String())
		}
	}
	for _, body := range []string{
		`{"role":"planner","model":"local/text"}`,
		`{"role":"vision","model":"local/vision"}`,
		`{"role":"search","model":"search/fast"}`,
	} {
		if response := request(body, true); response.Code != http.StatusOK {
			t.Fatalf("save role %s = %d %s", body, response.Code, response.Body.String())
		}
	}
	loaded, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent.PlannerModel != "local/text" || loaded.Agent.VisionModel != "local/vision" || loaded.Agent.WebSearchModel != "search/fast" {
		t.Fatalf("runtime role config: planner=%q vision=%q search=%q", loaded.Agent.PlannerModel, loaded.Agent.VisionModel, loaded.Agent.WebSearchModel)
	}
	stored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), `future_root_option = "keep-root"`) || !strings.Contains(string(stored), `future_provider_option = "keep-provider"`) {
		t.Fatalf("unknown fields lost: %s", stored)
	}
	summary, err := loadProviderSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.PlannerModel != "local/text" || summary.VisionModel != "local/vision" || summary.WebSearchModel != "search/fast" || len(summary.Providers) != 3 || !strings.EqualFold(strings.Join(summary.Providers[0].VisionModels, ","), "vision") || summary.Providers[1].Configured || strings.Join(summary.Providers[2].SearchModels, ",") != "fast" {
		t.Fatalf("role summary: %#v", summary)
	}
	if response := request(`{"role":"vision","model":"auto"}`, true); response.Code != http.StatusOK {
		t.Fatalf("auto vision = %d %s", response.Code, response.Body.String())
	}
	if response := request(`{"role":"planner","model":""}`, true); response.Code != http.StatusOK {
		t.Fatalf("clear planner = %d %s", response.Code, response.Body.String())
	}
	if response := request(`{"role":"search","model":"auto"}`, true); response.Code != http.StatusOK {
		t.Fatalf("automatic search = %d %s", response.Code, response.Body.String())
	}
}

func TestSearchModelEditPreservesExistingComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	original := `# user's Preview configuration
default_model = "search/fast"

[agent]
# keep search routing explanation
web_search_model = "auto"
future_agent_option = 42

[[providers]]
name = "search"
kind = "responses"
base_url = "http://127.0.0.1:9125/v1"
api_key_env = "REASONIX_PREVIEW_SEARCH_TEST_KEY"
models = ["fast"]
default = "fast"
web_search = true
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("REASONIX_PREVIEW_SEARCH_TEST_KEY=test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := loadProviderSummary()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Providers) != 1 || !summary.Providers[0].Configured || strings.Join(summary.Providers[0].SearchModels, ",") != "fast" {
		t.Fatalf("credential-backed search candidates: %#v", summary.Providers)
	}
	if encoded, err := json.Marshal(summary); err != nil || strings.Contains(string(encoded), "test-secret") {
		t.Fatal("search summary exposed a credential")
	}
	if err := persistModelRole(setModelRoleRequest{Role: "search", Model: "search/fast"}); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# user's Preview configuration", "# keep search routing explanation", "future_agent_option = 42", `web_search_model = "search/fast"`} {
		if !strings.Contains(string(stored), want) {
			t.Fatalf("search edit lost %q: %s", want, stored)
		}
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

func TestDesktopApprovalEndpointPersistsNarrowChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := "future_root_option = \"keep-root\"\n[desktop]\ndefault_tool_approval_mode = \"ask\"\nfuture_desktop_option = \"keep-desktop\"\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance").handler()
	call := func(method, endpoint, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, endpoint, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		if method == http.MethodPost {
			request.Header.Set(requestIDHeader, "desktop-approval-test-0001")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if got := call(http.MethodGet, "/v1/settings/desktop", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"defaultToolApprovalMode":"ask"`) {
		t.Fatalf("read: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/approval", `{"mode":"yolo"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"defaultToolApprovalMode":"yolo"`) {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`default_tool_approval_mode = "yolo"`, `future_root_option = "keep-root"`, `future_desktop_option = "keep-desktop"`} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("saved config lost %q: %s", want, updated)
		}
	}
	if err := persistDesktopApprovalMode("unsafe"); err == nil {
		t.Fatal("invalid mode was accepted")
	}
}
