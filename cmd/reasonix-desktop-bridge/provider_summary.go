package main

import (
	"errors"
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
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName,omitempty"`
	Kind        string   `json:"kind"`
	Models      []string `json:"models"`
	ModelCount  int      `json:"modelCount"`
	RequiresKey bool     `json:"requiresKey"`
	Configured  bool     `json:"configured"`
}

type setDefaultModelRequest struct {
	Model string `json:"model"`
}

// setProviderKeyRequest is private to the native host/sidecar boundary. The
// WebView never receives the bridge token and cannot call this endpoint.
type setProviderKeyRequest struct {
	ProviderName string `json:"providerName"`
	APIKey       string `json:"apiKey,omitempty"`
	Delete       bool   `json:"delete,omitempty"`
}

var errDefaultModelUnavailable = errors.New("selected model is not configured and ready")

// loadProviderSummary reads only the private user config selected by
// REASONIX_HOME. It intentionally projects away endpoints, credential names,
// keys, headers, and all provider-specific extension data. The configured
// model IDs are included only because the host needs them for model selection.
func loadProviderSummary() (providerSummaryResponse, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return providerSummaryResponse{}, fmt.Errorf("load user provider configuration: %w", err)
	}
	providers := make([]providerSummaryEntry, 0, len(cfg.Providers))
	for i := range cfg.Providers {
		provider := &cfg.Providers[i]
		requiresKey := provider.RequiresAPIKey()
		provider.ResolveAPIKeyForRoot(".")
		configured := !requiresKey || provider.Configured()
		models := provider.ModelList()
		if models == nil {
			models = []string{}
		}
		entry := providerSummaryEntry{
			Name:        provider.Name,
			DisplayName: strings.TrimSpace(provider.DisplayName),
			Kind:        provider.Kind,
			Models:      models,
			ModelCount:  len(models),
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

func updateProviderKey(request setProviderKeyRequest) error {
	name := strings.TrimSpace(request.ProviderName)
	if name == "" {
		return errors.New("provider name is required")
	}
	if !request.Delete && strings.TrimSpace(request.APIKey) == "" {
		return errors.New("API key is required")
	}

	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return fmt.Errorf("load user provider configuration: %w", err)
	}
	for i := range cfg.Providers {
		provider := &cfg.Providers[i]
		if provider.Name != name {
			continue
		}
		if !provider.RequiresAPIKey() {
			return errors.New("provider does not require an API key")
		}
		if request.Delete {
			configpkg.ClearDesktopKeychainCredential(name)
		} else {
			configpkg.SetDesktopKeychainCredential(name, request.APIKey)
		}
		return nil
	}
	return errors.New("provider was not found")
}

// persistDefaultModel applies the existing config package's validation and
// narrow model-settings writer. It changes new-session behavior only; the
// currently open controller and its conversation are left untouched.
func persistDefaultModel(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errDefaultModelUnavailable
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return fmt.Errorf("load Preview user config")
	}
	baseline := cfg.ModelSettingsBaseline()
	entry, ok := cfg.ResolveModel(ref)
	if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
		return errDefaultModelUnavailable
	}
	entry.ResolveAPIKeyForRoot(".")
	if !entry.Configured() {
		return errDefaultModelUnavailable
	}
	resolved := entry.Name + "/" + entry.Model
	if err := cfg.SetDefaultModel(resolved); err != nil {
		return errDefaultModelUnavailable
	}
	if err := cfg.SaveModelSettingsTo(path, baseline); err != nil {
		return fmt.Errorf("save Preview default model")
	}
	return nil
}

func providerAccessAllowed(access []string, provider string) bool {
	if access == nil {
		return true
	}
	for _, allowed := range access {
		if strings.TrimSpace(allowed) == provider {
			return true
		}
	}
	return false
}
