package main

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// Provider configuration is deliberately separate from the redacted summary.
// Existing endpoints and credential identifiers are never sent to the WebView.
type providerConfigList struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Providers       []providerConfigView `json:"providers"`
}

type providerConfigView struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
}

type saveProviderConfigRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	BaseURL     string   `json:"baseUrl"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
	UseAPIKey   bool     `json:"useApiKey"`
}

var previewProviderName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func loadProviderConfigs() (providerConfigList, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return providerConfigList{}, err
	}
	view := providerConfigList{ProtocolVersion: desktopbridge.ProtocolVersion, Providers: make([]providerConfigView, 0, len(cfg.Providers))}
	for i := range cfg.Providers {
		entry := &cfg.Providers[i]
		view.Providers = append(view.Providers, providerConfigView{
			Name: entry.Name, DisplayName: entry.DisplayName, Kind: entry.Kind,
			Models: append([]string(nil), entry.ModelList()...), Default: entry.Default,
		})
	}
	return view, nil
}

func validateProviderConfigInput(input *saveProviderConfigRequest) error {
	input.Name = strings.TrimSpace(input.Name)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Default = strings.TrimSpace(input.Default)
	if !previewProviderName.MatchString(input.Name) {
		return fmt.Errorf("provider name must use letters, digits, hyphens, or underscores")
	}
	switch input.Kind {
	case "openai", "anthropic", "responses":
	default:
		return fmt.Errorf("unsupported provider protocol")
	}
	if len(input.Models) == 0 || len(input.Models) > 100 {
		return fmt.Errorf("provide 1–100 model IDs")
	}
	seen := map[string]bool{}
	for i := range input.Models {
		input.Models[i] = strings.TrimSpace(input.Models[i])
		model := input.Models[i]
		if model == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n\x00") || seen[model] {
			return fmt.Errorf("model IDs must be distinct and nonempty")
		}
		seen[model] = true
	}
	if input.Default == "" {
		input.Default = input.Models[0]
	}
	if !seen[input.Default] {
		return fmt.Errorf("default model must appear in the model list")
	}
	if input.BaseURL != "" {
		u, err := url.Parse(input.BaseURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("endpoint must be a URL without embedded credentials or query")
		}
		if u.Scheme != "https" {
			if u.Scheme != "http" || !isLoopbackHost(u.Hostname()) {
				return fmt.Errorf("endpoint must use HTTPS or loopback HTTP")
			}
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	return net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func persistProviderConfig(input saveProviderConfigRequest) error {
	if err := validateProviderConfigInput(&input); err != nil {
		return err
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return err
	}
	baseline := cfg.ModelSettingsBaseline()
	var entry configpkg.ProviderEntry
	exists := false
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == input.Name {
			entry = cfg.Providers[i] // preserve hidden advanced fields and credential source
			exists = true
			break
		}
	}
	if exists && input.Kind != entry.Kind {
		return fmt.Errorf("changing an existing provider protocol is not supported")
	}
	if !exists && input.BaseURL == "" {
		return fmt.Errorf("new providers need an endpoint")
	}
	entry.Name = input.Name
	entry.DisplayName = input.DisplayName
	entry.Kind = input.Kind
	if input.BaseURL != "" {
		entry.BaseURL = input.BaseURL
		// An explicit new endpoint must not retain a hidden request override.
		entry.ChatURL = ""
		entry.RequestURL = ""
	}
	entry.Model = input.Models[0]
	entry.Models = append([]string(nil), input.Models...)
	entry.Default = input.Default
	if !exists && input.UseAPIKey {
		entry.APIKeyEnv = "REASONIX_" + strings.ToUpper(strings.ReplaceAll(input.Name, "-", "_")) + "_API_KEY"
	}
	if !exists && entry.RequiresAPIKey() && entry.APIKeyEnv == "" {
		return fmt.Errorf("this endpoint requires a credential source")
	}
	if err := configpkg.ValidateProviderEndpoint(&entry); err != nil {
		return err
	}
	if err := cfg.UpsertProvider(entry); err != nil {
		return err
	}
	if cfg.Desktop.ProviderAccess != nil && !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Name) {
		cfg.Desktop.ProviderAccess = append(cfg.Desktop.ProviderAccess, input.Name)
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}
