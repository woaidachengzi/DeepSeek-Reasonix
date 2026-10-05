package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestProjectModelPreferencesStayInWorkspaceConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	globalConfig := `default_model = "local/chat"
reasoning_language = "en"

[agent]
compact_ratio = 0.8
planner_model = "local/chat"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:1234/v1"
models = ["chat", "vision-chat"]
default = "chat"
`
	globalPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(globalPath, []byte(globalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	globalBefore, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(workspace, "reasonix.toml")
	projectConfig := "[agent]\nreasoning_language = \"zh\"\ncompact_ratio = 0.7\n\n[permissions]\nmode = \"ask\"\n"
	if err := os.WriteFile(projectPath, []byte(projectConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadProviderSummaryForScope("project", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != "project" || got.DefaultModel != "local/chat" || got.ReasoningLanguage != "zh" || got.CompactRatioPercent != 70 {
		t.Fatalf("project summary did not resolve workspace overrides: %#v", got)
	}
	bridge := newBridgeServer(testToken, "instance-project-model-test")
	query := url.Values{"scope": {"project"}, "workspaceRoot": {workspace}}
	getRequest := httptest.NewRequest(http.MethodGet, "/v1/providers?"+query.Encode(), nil)
	getRequest.Header.Set("Authorization", "Bearer "+testToken)
	getResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("scoped summary status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var routeSummary providerSummaryResponse
	if err := json.NewDecoder(getResponse.Body).Decode(&routeSummary); err != nil {
		t.Fatal(err)
	}
	if routeSummary.Scope != "project" || routeSummary.ReasoningLanguage != "zh" {
		t.Fatalf("scoped summary route returned the wrong config: %#v", routeSummary)
	}
	setBody, err := json.Marshal(setDefaultModelRequest{Model: "local/vision-chat", Scope: "project", WorkspaceRoot: workspace})
	if err != nil {
		t.Fatal(err)
	}
	setRequest := httptest.NewRequest(http.MethodPost, "/v1/settings/default-model", strings.NewReader(string(setBody)))
	setRequest.Header.Set("Authorization", "Bearer "+testToken)
	setRequest.Header.Set("Content-Type", "application/json")
	setRequest.Header.Set(requestIDHeader, "project-model-default-test")
	setResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(setResponse, setRequest)
	if setResponse.Code != http.StatusOK {
		t.Fatalf("project default-model status = %d, body = %s", setResponse.Code, setResponse.Body.String())
	}
	var setSummary providerSummaryResponse
	if err := json.NewDecoder(setResponse.Body).Decode(&setSummary); err != nil {
		t.Fatal(err)
	}
	if setSummary.Scope != "project" || setSummary.DefaultModel != "local/vision-chat" {
		t.Fatalf("project default-model response = %#v", setSummary)
	}
	if err := persistDefaultModelForScope("local/vision-chat", "project", workspace); err != nil {
		t.Fatalf("save project default model: %v", err)
	}
	if err := persistModelRole(setModelRoleRequest{Role: "planner", Model: "local/vision-chat", Scope: "project", WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("save project planner model: %v", err)
	}
	if err := persistAgentPreferences(setAgentPreferenceRequest{ReasoningLanguage: "en", Scope: "project", WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("save project reasoning language: %v", err)
	}
	if err := persistAgentPreferences(setAgentPreferenceRequest{CompactRatioPercent: 75, Scope: "project", WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("save project compaction threshold: %v", err)
	}

	got, err = loadProviderSummaryForScope("project", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultModel != "local/vision-chat" || got.PlannerModel != "local/vision-chat" || got.ReasoningLanguage != "en" || got.CompactRatioPercent != 75 {
		t.Fatalf("saved project preferences were not read back: %#v", got)
	}
	projectAfter, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{"[permissions]", `mode = "ask"`} {
		if !strings.Contains(string(projectAfter), preserved) {
			t.Fatalf("project edit lost unrelated setting %q: %s", preserved, projectAfter)
		}
	}
	globalAfter, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(globalAfter) != string(globalBefore) {
		t.Fatalf("project model preference edits changed global configuration:\nbefore:\n%s\nafter:\n%s", globalBefore, globalAfter)
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
	if summary.PlannerModel != "local/text" || summary.VisionModel != "local/vision" || summary.WebSearchModel != "search/fast" || len(summary.Providers) != 2 || !strings.EqualFold(strings.Join(summary.Providers[0].VisionModels, ","), "vision") || strings.Join(summary.Providers[1].SearchModels, ",") != "fast" {
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
	requestSequence := 0
	call := func(method, endpoint, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, endpoint, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		if method == http.MethodPost {
			requestSequence++
			request.Header.Set(requestIDHeader, "desktop-settings-test"+strings.ReplaceAll(endpoint, "/", "-")+"-"+strconv.Itoa(requestSequence))
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if got := call(http.MethodGet, "/v1/settings/desktop", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"defaultToolApprovalMode":"ask"`) || !strings.Contains(got.Body.String(), `"language":""`) || !strings.Contains(got.Body.String(), `"displayCurrency":""`) || !strings.Contains(got.Body.String(), `"terminalTheme":"auto"`) || !strings.Contains(got.Body.String(), `"appearanceConfigured":false`) {
		t.Fatalf("read: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/approval", `{"mode":"yolo"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"defaultToolApprovalMode":"yolo"`) {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/terminal-theme", `{"theme":"dark"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"terminalTheme":"dark"`) {
		t.Fatalf("save terminal theme: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/appearance", `{"theme":"light","style":"aurora"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"theme":"light"`) || !strings.Contains(got.Body.String(), `"themeStyle":"aurora"`) || !strings.Contains(got.Body.String(), `"appearanceConfigured":true`) {
		t.Fatalf("save appearance: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/language", `{"language":"en"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"language":"en"`) {
		t.Fatalf("save language: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/currency", `{"currency":"CNY"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"displayCurrency":"CNY"`) {
		t.Fatalf("save display currency: %d %s", got.Code, got.Body.String())
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`default_tool_approval_mode = "yolo"`, `language = "en"`, `currency = "CNY"`, `display_currency = "CNY"`, `terminal_theme = "dark"`, `theme = "light"`, `theme_style = "aurora"`, `future_root_option = "keep-root"`, `future_desktop_option = "keep-desktop"`} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("saved config lost %q: %s", want, updated)
		}
	}
	if err := persistDesktopApprovalMode("unsafe"); err == nil {
		t.Fatal("invalid mode was accepted")
	}
	if err := persistDesktopTerminalTheme("sepia"); err == nil {
		t.Fatal("invalid terminal theme was accepted")
	}
	if err := persistDesktopAppearance("light", "sepia"); err == nil {
		t.Fatal("invalid appearance style was accepted")
	}
	if err := persistDesktopCurrency("EUR"); err == nil {
		t.Fatal("unsupported display currency was accepted")
	}
	if got := call(http.MethodPost, "/v1/settings/desktop/language", `{"language":"fr"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid language status = %d, body = %s", got.Code, got.Body.String())
	}
}

func TestAgentPreferencesEndpointPersistsAndValidatesPreviewValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := "[agent]\nreasoning_language = \"auto\"\ncompact_ratio = 0.8\nfuture_agent_option = \"keep-agent\"\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "agent-preferences").handler()
	requestID := 0
	call := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		requestID++
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/agent-preferences", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set(requestIDHeader, "agent-preference-"+strconv.Itoa(requestID))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if got := call(`{"reasoningLanguage":"zh"}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"reasoningLanguage":"zh"`) || !strings.Contains(got.Body.String(), `"compactRatioPercent":80`) {
		t.Fatalf("reasoning language save = %d %s", got.Code, got.Body.String())
	}
	if got := call(`{"compactRatioPercent":75}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"compactRatioPercent":75`) {
		t.Fatalf("compact ratio save = %d %s", got.Code, got.Body.String())
	}
	if got := call(`{"compactRatioPercent":75.5}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"compactRatioPercent":75.5`) {
		t.Fatalf("fractional compact ratio save = %d %s", got.Code, got.Body.String())
	}
	beforeInvalid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{}`, `{"reasoningLanguage":"fr"}`, `{"compactRatioPercent":91}`, `{"compactRatioPercent":85.1}`, `{"compactRatioPercent":75.55}`, `{"reasoningLanguage":"en","compactRatioPercent":70}`} {
		if got := call(invalid); got.Code != http.StatusBadRequest {
			t.Fatalf("invalid agent setting %s status = %d: %s", invalid, got.Code, got.Body.String())
		}
	}
	afterInvalid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterInvalid) != string(beforeInvalid) {
		t.Fatal("invalid agent settings changed the config")
	}
	for _, preserved := range []string{`reasoning_language = "zh"`, `compact_ratio = 0.755`, `future_agent_option = "keep-agent"`} {
		if !strings.Contains(string(afterInvalid), preserved) {
			t.Fatalf("saved agent config lost %q: %s", preserved, afterInvalid)
		}
	}
}
