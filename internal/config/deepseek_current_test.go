package config

import (
	"reflect"
	"testing"

	"reasonix/internal/provider"
)

func TestNewDeepSeekDefaultSurvivesNormalization(t *testing.T) {
	c := Default()
	normalizeOfficialDeepSeekModels(c)
	p, ok := c.Provider("deepseek-flash")
	if !ok || p.DefaultModel() != provider.DeepSeekFlashModel || !EffectiveVision(p) {
		t.Fatalf("canonical default lost during startup normalization: %+v", p)
	}
}

func TestCurrentDeepSeekFlashAliasesAndExplicitOff(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic", "responses"} {
		base := "https://api.deepseek.com"
		if kind == "anthropic" {
			base += "/anthropic"
		}
		for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
			t.Run(kind+"/"+model, func(t *testing.T) {
				e := ProviderEntry{Name: "deepseek", Kind: kind, BaseURL: base, Model: model}
				got := NewTransientModelCapabilityResolver().Resolve(&e)
				if got.State != CapabilitySupported || !EffectiveVision(&e) {
					t.Fatalf("Flash image capability = %+v", got)
				}
				if err := ReasoningCapabilityForEntry(&e).Validate(model, "low"); err != nil {
					t.Fatal(err)
				}
				off := false
				e.visionOverride = &off
				if got := NewTransientModelCapabilityResolver().Resolve(&e); got.State != CapabilityUnsupported || EffectiveVision(&e) {
					t.Fatalf("explicit off ignored: %+v", got)
				}
			})
		}
	}
}

func TestOpenCodeGoInferenceModelDiscovery(t *testing.T) {
	for _, base := range []string{"https://opencode.ai/inference/go/openai/v1", "https://opencode.ai/inference/go/anthropic"} {
		got, err := BuildModelFetchURLs(base, "")
		if err != nil || !reflect.DeepEqual(got, []string{"https://opencode.ai/inference/go/v1/models"}) {
			t.Fatalf("%s: %v %v", base, got, err)
		}
		got, err = BuildModelFetchURLs(base, "https://catalog.example/models")
		if err != nil || !reflect.DeepEqual(got, []string{"https://catalog.example/models"}) {
			t.Fatalf("explicit model URL lost: %v %v", got, err)
		}
	}
}

func TestFlashPriceRefreshPreservesCustomPricesAndEndpoints(t *testing.T) {
	for _, currency := range []string{"CNY", "USD"} {
		old := &provider.Pricing{CacheHit: 0.014, Input: 0.44, Output: 1.32, Currency: "$"}
		if currency == "CNY" {
			old = &provider.Pricing{CacheHit: 0.10, Input: 3, Output: 9, Currency: "¥"}
		}
		custom := &provider.Pricing{CacheHit: 0.123, Input: 7, Output: 11, Currency: old.Currency}
		c := &Config{Providers: []ProviderEntry{
			{Name: "official", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", Price: old, BillingCurrency: currency, persistedOfficialCurrency: currency},
			{Name: "custom-price", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", Price: custom, BillingCurrency: currency},
			{Name: "proxy", Kind: "openai", BaseURL: "https://proxy.example/v1", Model: "deepseek-v4-flash", Price: old, BillingCurrency: currency},
		}}
		applyDeepSeekOfficialDefaultPricing(c)
		if !samePricing(c.Providers[0].Price, deepSeekV4PriceForModel(currency, "deepseek-v4-flash")) {
			t.Fatalf("official price not refreshed: %+v", c.Providers[0].Price)
		}
		if c.Providers[0].Model != "deepseek-v4-flash" || c.Providers[1].Price != custom || c.Providers[2].Price != old {
			t.Fatal("user model, custom price or proxy mutated")
		}
		applyDeepSeekOfficialDefaultPricing(c)
		if !samePricing(c.Providers[0].Price, deepSeekV4PriceForModel(currency, "deepseek-v4-flash")) {
			t.Fatal("refresh not idempotent")
		}
	}
}
