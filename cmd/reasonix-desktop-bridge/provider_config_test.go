package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
)

func providerTestPtr[T any](value T) *T { return &value }

func TestPreviewProviderPresetCatalogAndReviewedInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	bridge := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/provider-configs", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(requestIDHeader, id)
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized catalog status = %d", got.Code)
	}
	response := request(http.MethodGet, "", "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("read catalog: %d %s", response.Code, response.Body.String())
	}
	var view providerConfigList
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Presets) < 18 || strings.Contains(response.Body.String(), "MIMO_API_KEY") {
		t.Fatalf("catalog missing entries or exposed credential identifier: %s", response.Body.String())
	}
	var mimo providerPresetView
	for _, preset := range view.Presets {
		if preset.ID == "mimo-api" {
			mimo = preset
		}
	}
	if mimo.Status != "available" || len(mimo.Routes) != 1 || mimo.Routes[0].BaseURL != "https://api.xiaomimimo.com/v1" || len(mimo.Revision) != 64 {
		t.Fatalf("mimo preset review = %+v", mimo)
	}
	body := `{"presetId":"mimo-api","revision":"` + mimo.Revision + `"}`
	if got := request(http.MethodPost, body, "preset-unauthorized", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized install status = %d", got.Code)
	}
	response = request(http.MethodPost, body, "preset-install", true)
	if response.Code != http.StatusOK {
		t.Fatalf("install: %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, preset := range view.Presets {
		if preset.ID == "mimo-api" && preset.Status != "installed" {
			t.Fatalf("installed preset status = %s", preset.Status)
		}
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	installed, ok := cfg.Provider("mimo-api")
	if !ok || installed.PresetID != "mimo-api" || !installed.NoProxy || installed.APIKeyEnv != "MIMO_API_KEY" {
		t.Fatalf("preset did not preserve curated provider fields: %+v", installed)
	}
	if got := request(http.MethodPost, body, "preset-stale", true); got.Code != http.StatusBadRequest {
		t.Fatalf("stale install status = %d", got.Code)
	}
}

func TestPreviewDiscoversModelsFromSavedProviderWithoutReturningCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected model catalog path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer private-test-key" {
			t.Errorf("model catalog did not receive saved credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-z"},{"id":"model-a"}]}`))
	}))
	defer server.Close()
	config := `[[providers]]
name = "saved-provider"
display_name = "Saved Provider"
kind = "openai"
base_url = "` + server.URL + `/v1"
api_key_env = "PREVIEW_DISCOVERY_API_KEY"
models = ["existing"]
default = "existing"

[desktop]
provider_access = ["saved-provider"]
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := configpkg.StoreCredentialLines([]string{"PREVIEW_DISCOVERY_API_KEY=private-test-key"}); err != nil {
		t.Fatalf("save test credential: %v", err)
	}
	view, err := loadProviderConfigs(testToken)
	if err != nil || len(view.Providers) != 1 {
		t.Fatalf("load provider config: providers=%d err=%v", len(view.Providers), err)
	}
	bridge := newBridgeServer(testToken, "instance-provider-discovery")
	request := func(input discoverProviderModelsRequest, authorized bool) *httptest.ResponseRecorder {
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs/discover-models", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	provider := view.Providers[0]
	if got := request(discoverProviderModelsRequest{Name: provider.Name, Revision: provider.Revision}, false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized discovery status = %d", got.Code)
	}
	response := request(discoverProviderModelsRequest{Name: provider.Name, Revision: provider.Revision}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("discovery status = %d: %s", response.Code, response.Body.String())
	}
	var result discoverProviderModelsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 2 || result.Models[0] != "model-a" || result.Models[1] != "model-z" {
		t.Fatalf("discovered models = %#v", result.Models)
	}
	if strings.Contains(response.Body.String(), "private-test-key") || strings.Contains(response.Body.String(), server.URL) {
		t.Fatalf("discovery response leaked credentials or endpoint: %s", response.Body.String())
	}
	stale := request(discoverProviderModelsRequest{Name: provider.Name, Revision: "stale"}, true)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale discovery status = %d: %s", stale.Code, stale.Body.String())
	}
	// Discovery only previews results; the saved model list remains untouched.
	after, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := after.Provider(provider.Name)
	if !slices.Equal(saved.ModelList(), []string{"existing"}) {
		t.Fatalf("discovery changed saved model list: %#v", saved.ModelList())
	}
}

func TestPreviewInstallsEveryCuratedProviderPreset(t *testing.T) {
	for _, preset := range configpkg.CuratedProviderPresets() {
		t.Run(preset.ID, func(t *testing.T) {
			t.Setenv("REASONIX_HOME", t.TempDir())
			revision, err := previewProviderConfigRevision(configpkg.UserConfigPath(), testToken)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistProviderPreset(saveProviderConfigRequest{PresetID: preset.ID, Revision: revision}, testToken); err != nil {
				t.Fatal(err)
			}
			cfg, err := configpkg.LoadUserConfigReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range preset.Entries {
				installed, ok := cfg.Provider(route.Name)
				if !ok || installed.PresetID != preset.ID {
					t.Fatalf("missing route %q after preset install", route.Name)
				}
			}
		})
	}
}

func TestPreviewPresetRefusesExistingProviderName(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	if err := persistProviderConfig(saveProviderConfigRequest{Name: "mimo-api", DisplayName: "My MiMo", Kind: "openai", BaseURL: "https://custom.example/v1", Models: []string{"chat"}, Default: "chat", UseAPIKey: true}); err != nil {
		t.Fatal(err)
	}
	view, err := loadProviderConfigs(testToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range view.Presets {
		if preset.ID != "mimo-api" {
			continue
		}
		if preset.Status != "name_conflict" {
			t.Fatalf("conflicting preset status = %q", preset.Status)
		}
		if err := persistProviderPreset(saveProviderConfigRequest{PresetID: preset.ID, Revision: preset.Revision}, testToken); err == nil {
			t.Fatal("conflicting preset overwrote an existing provider")
		}
		cfg, err := configpkg.LoadUserConfigReadOnly()
		if err != nil {
			t.Fatal(err)
		}
		existing, _ := cfg.Provider("mimo-api")
		if existing.BaseURL != "https://custom.example/v1" {
			t.Fatalf("conflicting provider changed to %q", existing.BaseURL)
		}
		return
	}
	t.Fatal("mimo preset missing")
}

func TestPreviewPresetResetRestoresCuratedRouteAndPreservesCredentialSource(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	revision, err := previewProviderConfigRevision(configpkg.UserConfigPath(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistProviderPreset(saveProviderConfigRequest{PresetID: "mimo-api", Revision: revision}, testToken); err != nil {
		t.Fatal(err)
	}
	if err := persistProviderConfig(saveProviderConfigRequest{Name: "mimo-api", DisplayName: "My MiMo", Kind: "openai", BaseURL: "https://custom.example/v1", Models: []string{"custom-chat"}, Default: "custom-chat"}); err != nil {
		t.Fatal(err)
	}
	editable, err := configpkg.LoadForEditReadOnlyStrict(configpkg.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	baseline := editable.ModelSettingsBaseline()
	if err := editable.SetDefaultModel("mimo-api/custom-chat"); err != nil {
		t.Fatal(err)
	}
	if err := editable.SaveUserSettingsDeltaTo(configpkg.UserConfigPath(), baseline); err != nil {
		t.Fatal(err)
	}
	view, err := loadProviderConfigs(testToken)
	if err != nil {
		t.Fatal(err)
	}
	var preset providerPresetView
	for _, item := range view.Presets {
		if item.ID == "mimo-api" {
			preset = item
		}
	}
	if preset.Status != "installed_modified" {
		t.Fatalf("edited preset status = %q", preset.Status)
	}
	input := saveProviderConfigRequest{PresetID: preset.ID, PresetAction: "reset", Revision: preset.Revision}
	if err := persistProviderPreset(input, testToken); err != nil {
		t.Fatal(err)
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.Provider("mimo-api")
	if !ok || entry.BaseURL != "https://api.xiaomimimo.com/v1" || entry.DefaultModel() != "mimo-v2.5-pro" || entry.APIKeyEnv != "MIMO_API_KEY" || entry.DisplayName != "My MiMo" || cfg.DefaultModel != "mimo-api/mimo-v2.5-pro" {
		t.Fatalf("reset provider = %+v", entry)
	}
	if err := persistProviderPreset(input, testToken); err == nil {
		t.Fatal("stale preset reset overwrote the new config")
	}
}

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
kind = "responses"
base_url = "https://example.com/v1"
api_key_env = "CUSTOM_SECRET_KEY"
models_url = "https://example.com/models"
no_proxy = true
context_window = 32768
responses_mode = "stateful"
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
		if !view.Providers[0].ModelsURLSet || !view.Providers[0].NoProxy || view.Providers[0].ContextWindow != 32768 {
			t.Fatalf("advanced provider settings = %#v", view.Providers[0])
		}
		for _, secret := range []string{"example.com", "CUSTOM_SECRET_KEY", "hidden-value"} {
			if strings.Contains(got.Body.String(), secret) {
				t.Fatalf("config response leaked %q", secret)
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs", strings.NewReader(`{"name":"custom","displayName":"Updated label","kind":"responses","baseUrl":"","modelsUrl":"https://example.com/model-list","noProxy":false,"contextWindow":65536,"responsesMode":"stateless","models":["old","new"],"default":"old","useApiKey":false}`))
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
	saved, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := saved.Provider("custom")
	if !ok || provider.ModelsURL != "https://example.com/model-list" || provider.NoProxy || provider.ContextWindow != 65536 || provider.ResponsesMode != "stateless" {
		t.Fatalf("advanced provider edit not persisted: %#v", provider)
	}
	// A config save from an older Preview client omits advanced request fields.
	// Those fields must remain intact instead of resetting to Go zero values.
	legacyRequest := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs", strings.NewReader(`{"name":"custom","displayName":"Legacy edit","kind":"responses","baseUrl":"","models":["old","new"],"default":"old","useApiKey":false}`))
	legacyRequest.Header.Set("Authorization", "Bearer "+testToken)
	legacyRequest.Header.Set("Content-Type", "application/json")
	legacyRequest.Header.Set(requestIDHeader, "provider-config-edit-legacy-client")
	legacyResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(legacyResponse, legacyRequest)
	if legacyResponse.Code != http.StatusOK {
		t.Fatalf("legacy edit: %d %s", legacyResponse.Code, legacyResponse.Body.String())
	}
	saved, err = configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, ok = saved.Provider("custom")
	if !ok || provider.ModelsURL != "https://example.com/model-list" || provider.NoProxy || provider.ContextWindow != 65536 || provider.ResponsesMode != "stateless" {
		t.Fatalf("legacy edit reset omitted advanced provider settings: %#v", provider)
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
	input = base
	input.ModelsURL = "http://remote.example/models"
	if err := validateProviderConfigInput(&input); err == nil {
		t.Fatal("accepted insecure remote model discovery URL")
	}
	input = base
	input.ContextWindow = providerTestPtr(10_000_001)
	if err := validateProviderConfigInput(&input); err == nil {
		t.Fatal("accepted out-of-range context window")
	}
	input = base
	input.ResponsesMode = providerTestPtr("unexpected")
	if err := validateProviderConfigInput(&input); err == nil {
		t.Fatal("accepted unknown Responses mode")
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

func TestPreviewProviderBalanceURLSavePreserveClearAndRedact(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	input := saveProviderConfigRequest{Name: "balance-demo", DisplayName: "Balance Demo", Kind: "openai", BaseURL: "https://provider.example/v1", BalanceURL: "https://provider.example/account/balance", Models: []string{"chat"}, Default: "chat"}
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	view, err := loadProviderConfigs(testToken)
	if err != nil || len(view.Providers) < 1 {
		t.Fatalf("load provider config: view=%+v err=%v", view, err)
	}
	var balanceView *providerConfigView
	for i := range view.Providers {
		if view.Providers[i].Name == input.Name {
			balanceView = &view.Providers[i]
		}
	}
	if balanceView == nil || !balanceView.BalanceURLSet {
		t.Fatalf("saved balance URL was not reflected in redacted view: %+v", view.Providers)
	}
	rawView, _ := json.Marshal(view)
	if strings.Contains(string(rawView), input.BalanceURL) {
		t.Fatalf("provider view disclosed balance URL: %s", rawView)
	}
	input.BaseURL = ""
	input.BalanceURL = ""
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := cfg.Provider(input.Name)
	if saved.BalanceURL != "https://provider.example/account/balance" {
		t.Fatalf("blank editor field changed saved balance URL: %q", saved.BalanceURL)
	}
	input.ClearBalanceURL = true
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	cfg, err = configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	saved, _ = cfg.Provider(input.Name)
	if saved.BalanceURL != "" {
		t.Fatalf("clear action left balance URL configured: %q", saved.BalanceURL)
	}
}

func TestPreviewProviderBalanceURLValidation(t *testing.T) {
	base := saveProviderConfigRequest{Name: "balance-demo", Kind: "openai", Models: []string{"chat"}, Default: "chat"}
	for _, tc := range []struct {
		name string
		url  string
		want bool
	}{
		{name: "https", url: "https://provider.example/balance", want: true},
		{name: "loopback", url: "http://127.0.0.1:8123/balance", want: true},
		{name: "remote http", url: "http://provider.example/balance"},
		{name: "embedded credentials", url: "https://user:secret@provider.example/balance"},
		{name: "query parameters", url: "https://provider.example/balance?key=secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.BalanceURL = tc.url
			err := validateProviderConfigInput(&input)
			if (err == nil) != tc.want {
				t.Fatalf("validate balance URL %q err=%v, want valid=%v", tc.url, err, tc.want)
			}
		})
	}
}

func TestProviderConfigDeleteRetargetsDefaultAndPreservesOtherSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "custom/old"
future_top_level = "keep"

[desktop]
provider_access = ["custom", "fallback"]
default_tool_approval_mode = "ask"

[[providers]]
name = "custom"
display_name = "Custom"
kind = "openai"
base_url = "https://custom.example/v1"
models = ["old"]
default = "old"

[[providers]]
name = "fallback"
kind = "openai"
base_url = "https://fallback.example/v1"
models = ["chat"]
default = "chat"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-provider-delete")
	get := func() providerConfigList {
		r := httptest.NewRequest(http.MethodGet, "/v1/settings/provider-configs", nil)
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("get configs: %d %s", w.Code, w.Body.String())
		}
		var view providerConfigList
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	post := func(id string, input providerConfigView) *httptest.ResponseRecorder {
		body, err := json.Marshal(deleteProviderConfigRequest{Name: input.Name, DisplayName: input.DisplayName, Kind: input.Kind, Models: input.Models, Default: input.Default, Revision: input.Revision})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs/delete", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(requestIDHeader, id)
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	view := get()
	if len(view.Providers) != 2 || !view.Providers[0].Removable {
		t.Fatalf("configs: %#v", view)
	}
	stale := view.Providers[0]
	stale.Models = []string{"wrong"}
	if w := post("stale-provider-delete", stale); w.Code != http.StatusConflict {
		t.Fatalf("stale delete = %d: %s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(path, []byte(strings.Replace(initial, "https://custom.example/v1", "https://changed.example/v1", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := post("hidden-provider-change", view.Providers[0]); w.Code != http.StatusConflict {
		t.Fatalf("hidden endpoint changed but delete = %d: %s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := post("provider-delete", view.Providers[0]); w.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", w.Code, w.Body.String())
	}
	result := get()
	if len(result.Providers) != 1 || result.Providers[0].Name != "fallback" {
		t.Fatalf("after delete: %#v", result)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`default_model = "fallback"`, `provider_access = ["fallback"]`, `future_top_level = "keep"`, `default_tool_approval_mode = "ask"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config missing %q: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), `name = "custom"`) {
		t.Fatalf("removed provider remained: %s", raw)
	}
	if w := post("last-provider-delete", result.Providers[0]); w.Code != http.StatusBadRequest {
		t.Fatalf("last provider delete = %d: %s", w.Code, w.Body.String())
	}
}

func TestProviderConfigDeleteProtectsHiddenFallback(t *testing.T) {
	for _, tc := range []struct {
		name          string
		config        string
		provider      string
		wantRemovable bool
	}{
		{"hidden fallback", `default_model = "custom/chat"
[desktop]
provider_access = ["custom"]
[[providers]]
name = "custom"
kind = "openai"
base_url = "https://custom.example/v1"
models = ["chat"]
default = "chat"
[[providers]]
name = "hidden"
kind = "openai"
base_url = "https://hidden.example/v1"
models = ["chat"]
default = "chat"
`, "custom", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("REASONIX_HOME", home)
			path := filepath.Join(home, "config.toml")
			if err := os.WriteFile(path, []byte(tc.config), 0o600); err != nil {
				t.Fatal(err)
			}
			view, err := loadProviderConfigs(testToken)
			if err != nil {
				t.Fatal(err)
			}
			var target providerConfigView
			for _, item := range view.Providers {
				if item.Name == tc.provider {
					target = item
				}
			}
			if target.Name == "" || target.Removable != tc.wantRemovable {
				t.Fatalf("target = %#v", target)
			}
			input := deleteProviderConfigRequest{Name: target.Name, DisplayName: target.DisplayName, Kind: target.Kind, Models: target.Models, Default: target.Default, Revision: target.Revision}
			if err := removeProviderConfig(input, testToken); err == nil {
				t.Fatal("unsafe provider deletion succeeded")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.config {
				t.Fatalf("failed deletion changed config: %s", raw)
			}
		})
	}
}

func TestOfficialProviderAccessRemovalAndRestore(t *testing.T) {
	for _, withFallback := range []bool{false, true} {
		t.Run(fmt.Sprint(withFallback), func(t *testing.T) {
			t.Setenv("REASONIX_HOME", t.TempDir())
			t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
			if _, err := configpkg.StoreCredentialLines([]string{"FALLBACK_TEST_KEY=fixture-only"}); err != nil {
				t.Fatal(err)
			}
			path := configpkg.UserConfigPath()
			body := `default_model = "deepseek-pro/chat"
future_field = "preserved"
[[providers]]
name = "deepseek-pro"
kind = "openai"
base_url = "https://api.deepseek.com"
api_key_env = "OFFICIAL_TEST_KEY"
models = ["chat"]
default = "chat"
[agent]
planner_model = "deepseek-pro/chat"
guardian_model = "deepseek-pro/chat"
recovery_model = "deepseek-pro/chat"
vision_model = "deepseek-pro/chat"
[bot]
model = "deepseek-pro/chat"
`
			if withFallback {
				body += `
[[providers]]
name = "fallback"
kind = "openai"
base_url = "https://fallback.example/v1"
api_key_env = "FALLBACK_TEST_KEY"
models = ["chat"]
default = "chat"
`
			}
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			view, err := loadProviderConfigs(testToken)
			if err != nil {
				t.Fatal(err)
			}
			var target providerConfigView
			for _, p := range view.Providers {
				if p.Name == "deepseek-pro" {
					target = p
				}
			}
			if !target.Removable || target.Hidden {
				t.Fatalf("initial view: %+v", target)
			}
			input := deleteProviderConfigRequest{Name: target.Name, DisplayName: target.DisplayName, Kind: target.Kind, Models: target.Models, Default: target.Default, Revision: target.Revision}
			if err := removeProviderConfig(input, testToken); err != nil {
				t.Fatal(err)
			}
			if err := removeProviderConfig(input, testToken); err != errPreviewProviderChanged {
				t.Fatalf("stale removal: %v", err)
			}
			cfg, err := configpkg.LoadUserConfigReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			official, ok := cfg.Provider("deepseek-pro")
			if !ok || official.APIKeyEnv != "OFFICIAL_TEST_KEY" {
				t.Fatal("official entry or credential source lost")
			}
			if providerAccessAllowed(cfg.Desktop.ProviderAccess, "deepseek-pro") {
				t.Fatal("removed official entry reappeared after reload")
			}
			if withFallback && (cfg.DefaultModel != "fallback/chat" || cfg.Agent.PlannerModel != "fallback/chat" || cfg.Agent.GuardianModel != "fallback/chat" || cfg.Agent.RecoveryModel != "fallback/chat" || cfg.Bot.Model != "fallback/chat") {
				t.Fatalf("references not retargeted: default=%q planner=%q guardian=%q recovery=%q bot=%q", cfg.DefaultModel, cfg.Agent.PlannerModel, cfg.Agent.GuardianModel, cfg.Agent.RecoveryModel, cfg.Bot.Model)
			}
			if cfg.Agent.VisionModel != "" {
				t.Fatal("vision reference retained")
			}
			summary, err := loadProviderSummary()
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range summary.Providers {
				if p.Name == "deepseek-pro" {
					t.Fatal("removed entry remains on service card")
				}
			}
			view, err = loadProviderConfigs(testToken)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range view.Providers {
				if p.Name == "deepseek-pro" {
					target = p
				}
			}
			if !target.Hidden || target.Removable {
				t.Fatal("restore view missing")
			}
			input.Revision = target.Revision
			input.Restore = true
			if err := removeProviderConfig(input, testToken); err != nil {
				t.Fatal(err)
			}
			cfg, err = configpkg.LoadUserConfigReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			if !providerAccessAllowed(cfg.Desktop.ProviderAccess, "deepseek-pro") && !providerAccessAllowed(cfg.Desktop.ProviderAccess, "deepseek") {
				t.Fatal("restore failed")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "preserved") {
				t.Fatal("unknown field lost")
			}
		})
	}
}
