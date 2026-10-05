package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
)

func TestPreviewProviderReasoningRoundTrip(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	levels := []providerModelReasoning{{Model: "chat", ReasoningProtocol: "openai", SupportedEfforts: []string{"low", "high", "xhigh", "ultra"}, DefaultEffort: "ultra"}}
	input := saveProviderConfigRequest{Name: "custom", Kind: "openai", BaseURL: "https://example.test/v1", Models: []string{"chat"}, Default: "chat", UseAPIKey: true, ModelReasoning: &levels}
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	view, err := loadProviderConfigs(testToken)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range view.Providers {
		if item.Name == "custom" {
			found = true
			r := item.ModelReasoning[0]
			if r.DefaultEffort != "ultra" || len(r.Options) != 4 || r.Options[3].ID != "ultra" {
				t.Fatalf("lost reasoning: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("provider missing")
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := cfg.ResolveModel("custom/chat")
	if !ok {
		t.Fatal("model missing")
	}
	if cap := configpkg.EffortCapabilityForEntry(resolved); !slices.Contains(cap.Levels, "ultra") || cap.Default != "ultra" {
		t.Fatalf("composer differs: %+v", cap)
	}
	// An unrelated edit must preserve reasoning overrides, including old clients.
	input.ModelReasoning = nil
	input.BaseURL = ""
	input.DisplayName = "renamed"
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	cfg, _ = configpkg.LoadUserConfigReadOnly()
	resolved, _ = cfg.ResolveModel("custom/chat")
	if configpkg.EffectiveEffort(resolved) != "ultra" {
		t.Fatal("unrelated edit lost default")
	}
	before, _ := os.ReadFile(configpkg.UserConfigPath())
	levels[0].DefaultEffort = "max"
	input.ModelReasoning = &levels
	if err := persistProviderConfig(input); err == nil {
		t.Fatal("accepted default outside declared levels")
	}
	after, _ := os.ReadFile(configpkg.UserConfigPath())
	if string(before) != string(after) {
		t.Fatal("invalid save changed config")
	}
	// Reset removes declarations and returns to adapter defaults.
	levels[0].SupportedEfforts = nil
	levels[0].DefaultEffort = ""
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	cfg, _ = configpkg.LoadUserConfigReadOnly()
	resolved, _ = cfg.ResolveModel("custom/chat")
	cap := configpkg.EffortCapabilityForEntry(resolved)
	if slices.Contains(cap.Levels, "ultra") || !slices.Contains(cap.Levels, "medium") {
		t.Fatalf("reset failed: %+v", cap)
	}
}

func TestPreviewProviderReasoningDefaultWithoutDeclaration(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	levels := []providerModelReasoning{{Model: "chat", ReasoningProtocol: "openai", DefaultEffort: "high"}}
	input := saveProviderConfigRequest{Name: "custom", Kind: "openai", BaseURL: "https://example.test/v1", Models: []string{"chat"}, UseAPIKey: true, ModelReasoning: &levels}
	if err := persistProviderConfig(input); err != nil {
		t.Fatal(err)
	}
	input.BaseURL = ""
	input.DisplayName = "renamed"
	if err := persistProviderConfig(input); err != nil {
		t.Fatalf("unrelated save rejected an inherited vocabulary: %v", err)
	}
	levels[0].DefaultEffort = "invalid"
	if err := persistProviderConfig(input); err == nil {
		t.Fatal("invalid inherited default accepted")
	}
}

func TestPreviewProviderReasoningDraftUsesAdapterAndDoesNotWrite(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	levels := []providerModelReasoning{{Model: "chat", ReasoningProtocol: "openai"}}
	input := saveProviderConfigRequest{Name: "draft", Kind: "openai", BaseURL: "https://example.test/v1", Models: []string{"chat"}, ModelReasoning: &levels}
	result, err := previewProviderReasoning(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result[0].Options) != 3 || result[0].Options[1].ID != "medium" {
		t.Fatalf("wrong OpenAI draft: %+v", result)
	}
	levels[0].ReasoningProtocol = "glm"
	result, err = previewProviderReasoning(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result[0].Options) != 2 || result[0].Options[0].ID != "enabled" {
		t.Fatalf("stale protocol draft: %+v", result)
	}
	if _, err := os.Stat(configpkg.UserConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("draft preview wrote configuration: %v", err)
	}
}

func TestPreviewProviderReasoningRouteRequiresAuth(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	server := newBridgeServer(testToken, "reasoning-preview")
	body := `{"name":"draft","kind":"openai","baseUrl":"https://example.test/v1","models":["chat"],"modelReasoning":[{"model":"chat","reasoningProtocol":"openai"}]}`
	for _, authorized := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "/v1/settings/provider-configs/reasoning", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if authorized {
			request.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		server.handler().ServeHTTP(response, request)
		expected := http.StatusUnauthorized
		if authorized {
			expected = http.StatusOK
		}
		if response.Code != expected {
			t.Fatalf("preview auth=%v: %d %s", authorized, response.Code, response.Body.String())
		}
		if authorized && (!strings.Contains(response.Body.String(), `"medium"`) || strings.Contains(response.Body.String(), "example.test")) {
			t.Fatalf("unexpected metadata: %s", response.Body.String())
		}
	}
}
