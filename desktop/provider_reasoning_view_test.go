package main

import (
	"slices"
	"testing"

	"reasonix/internal/config"
)

func TestProviderModelCapabilityReasoningUsesExactOverride(t *testing.T) {
	p := config.ProviderEntry{Name: "relay", Kind: "openai", BaseURL: "https://relay.test/v1", Models: []string{"chat", "other"}, ReasoningProtocol: "openai", ModelOverrides: map[string]config.ProviderModelOverride{"chat": {SupportedEfforts: []string{"low", "high", "ultra"}, DefaultEffort: "ultra"}}}
	views := providerModelCapabilitiesForView(p, p.Models)
	if views[0].Reasoning == nil || views[0].Reasoning.Default != "ultra" || !slices.Contains(views[0].Reasoning.IDs(), "ultra") {
		t.Fatalf("lost model override: %+v", views[0])
	}
	if slices.Contains(views[1].Reasoning.IDs(), "ultra") {
		t.Fatal("override leaked into another model")
	}
	// Discovery can project a new model before it enters the saved model list.
	discovered := providerModelReasoningForView(p, "new")
	if discovered == nil || !slices.Contains(discovered.IDs(), "medium") {
		t.Fatalf("missing discovered reasoning: %+v", discovered)
	}
}
