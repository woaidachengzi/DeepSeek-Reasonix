package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func TestMiMoV26PresetsAndOldCatalogUpgrade(t *testing.T) {
	for _, preset := range CuratedProviderPresets() {
		if preset.ID != "mimo-api" && preset.ID != "mimo-anthropic" && !strings.HasPrefix(preset.ID, "mimo-token-plan-") {
			continue
		}
		t.Run(preset.ID, func(t *testing.T) {
			p := preset.Entries[0]
			if p.DefaultModel() != "mimo-v2.6-flash" || !p.HasModel("mimo-v2.6-pro") {
				t.Fatalf("outdated preset: %v / %s", p.ModelList(), p.DefaultModel())
			}
			p.Models = append([]string(nil), legacyMimoV25Models...)
			p.VisionModels = append([]string(nil), legacyMimoV25VisionModels...)
			p.Default, p.DisplayName, p.APIKeyEnv = "mimo-v2.5-pro", "My service", "CUSTOM_MIMO_KEY"
			custom := &provider.Pricing{Input: 123, Currency: "¥"}
			p.Prices = map[string]*provider.Pricing{"mimo-v2.5-pro": custom}
			c := &Config{Providers: []ProviderEntry{p}, DefaultModel: p.Name + "/mimo-v2.5-pro"}
			if !normalizeLegacyMimoV26Catalog(c) {
				t.Fatal("old preset was not upgraded")
			}
			got := &c.Providers[0]
			if !got.HasModel("mimo-v2.6-flash") || !got.HasVisionModel("mimo-v2.6-pro") || got.Default != p.Default || c.DefaultModel != p.Name+"/mimo-v2.5-pro" || got.APIKeyEnv != p.APIKeyEnv || got.DisplayName != p.DisplayName || got.Prices["mimo-v2.5-pro"] != custom {
				t.Fatal("catalog upgrade lost user settings")
			}
			if price := got.PriceForModel("mimo-v2.6-flash"); price == nil || price.Input != 1 || price.Output != 2 || price.Currency != "¥" {
				t.Fatalf("v2.6 flash metadata mismatch: %+v", price)
			}
			if normalizeLegacyMimoV26Catalog(c) {
				t.Fatal("upgrade is not idempotent")
			}
		})
	}
}

func TestMiMoCatalogUpgradePreservesCustomListsAndRoutes(t *testing.T) {
	for _, name := range []string{"subset", "empty", "custom endpoint", "request override", "single model"} {
		t.Run(name, func(t *testing.T) {
			preset, _ := CuratedProviderPreset("mimo-token-plan-cn")
			p := preset.Entries[0]
			p.Models = append([]string(nil), legacyMimoV25Models...)
			switch name {
			case "subset":
				p.Models = []string{"mimo-v2.5-pro"}
			case "empty":
				p.Models = []string{}
			case "custom endpoint":
				p.BaseURL = "https://example.invalid/v1"
			case "request override":
				p.RequestURL = "https://example.invalid/chat"
			case "single model":
				p.Model = "mimo-v2.5"
			}
			before := p
			c := &Config{Providers: []ProviderEntry{p}}
			if normalizeLegacyMimoV26Catalog(c) || !reflect.DeepEqual(c.Providers[0], before) {
				t.Fatal("custom catalog was overwritten")
			}
		})
	}
}

func TestMiMoV26ReadOnlyLoadUpgradesViewWithoutWriting(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	raw := []byte(`[[providers]]
name = "mimo-token-plan-cn"
preset_id = "mimo-token-plan-cn"
kind = "openai"
base_url = "https://token-plan-cn.xiaomimimo.com/v1"
models = ["mimo-v2.5-pro", "mimo-v2.5"]
default = "mimo-v2.5-pro"
api_key_env = "MIMO_TOKEN_PLAN_API_KEY"
[desktop]
provider_access = ["mimo-token-plan-cn"]
`)
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Provider("mimo-token-plan-cn")
	if !ok || !p.HasModel("mimo-v2.6-flash") || p.DefaultModel() != "mimo-v2.5-pro" {
		t.Fatal("old saved preset did not expose new models")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(raw) {
		t.Fatal("read-only load wrote user configuration")
	}
}

func TestMiMoCatalogUpgradeKeepsCustomVisionAndProviderPrice(t *testing.T) {
	preset, _ := CuratedProviderPreset("mimo-token-plan-cn")
	p := preset.Entries[0]
	p.Models = append([]string(nil), legacyMimoV25Models...)
	p.VisionModels = []string{}
	p.Prices = nil
	p.Price = &provider.Pricing{Input: 123, Currency: "custom"}
	c := &Config{Providers: []ProviderEntry{p}}
	if !normalizeLegacyMimoV26Catalog(c) {
		t.Fatal("catalog was not upgraded")
	}
	if c.Providers[0].VisionModels == nil || len(c.Providers[0].VisionModels) != 0 || !reflect.DeepEqual(c.Providers[0].PriceForModel("mimo-v2.6-flash"), p.Price) {
		t.Fatal("custom vision/price changed")
	}
}
