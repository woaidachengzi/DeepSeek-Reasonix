package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
)

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

func TestProviderConfigDeleteProtectsOfficialAndHiddenFallback(t *testing.T) {
	for _, tc := range []struct {
		name          string
		config        string
		provider      string
		wantRemovable bool
	}{
		{"official", `default_model = "deepseek/chat"
[desktop]
provider_access = ["deepseek", "fallback"]
[[providers]]
name = "deepseek"
kind = "openai"
base_url = "https://api.deepseek.com"
models = ["chat"]
default = "chat"
[[providers]]
name = "fallback"
kind = "openai"
base_url = "https://fallback.example/v1"
models = ["chat"]
default = "chat"
`, "deepseek", false},
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
