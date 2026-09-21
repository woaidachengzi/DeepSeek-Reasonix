package main

import (
	"fmt"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type providerSummaryResponse struct {
	ProtocolVersion int                    `json:"protocolVersion"`
	DefaultModel    string                 `json:"defaultModel"`
	Providers       []providerSummaryEntry `json:"providers"`
}

type providerSummaryEntry struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Kind        string `json:"kind"`
	ModelCount  int    `json:"modelCount"`
	RequiresKey bool   `json:"requiresKey"`
	Configured  bool   `json:"configured"`
}

// loadProviderSummary reads only the private user config selected by
// REASONIX_HOME. It intentionally projects away endpoints, credential names,
// keys, headers, model lists, and all provider-specific extension data.
func loadProviderSummary() (providerSummaryResponse, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return providerSummaryResponse{}, fmt.Errorf("load user provider configuration: %w", err)
	}
	credentials := configpkg.NewCredentialResolverForRoot(".")
	providers := make([]providerSummaryEntry, 0, len(cfg.Providers))
	for i := range cfg.Providers {
		provider := &cfg.Providers[i]
		requiresKey := provider.RequiresAPIKey()
		configured := !requiresKey || credentials.ResolveGlobalFirst(provider.APIKeyEnv).Set
		entry := providerSummaryEntry{
			Name:        provider.Name,
			DisplayName: strings.TrimSpace(provider.DisplayName),
			Kind:        provider.Kind,
			ModelCount:  len(provider.ModelList()),
			RequiresKey: requiresKey,
			Configured:  configured,
		}
		providers = append(providers, entry)
	}
	return providerSummaryResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		DefaultModel:    cfg.DefaultModel,
		Providers:       providers,
	}, nil
}
