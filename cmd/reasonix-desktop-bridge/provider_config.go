package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// Provider configuration is deliberately separate from the redacted summary.
// Existing endpoints and credential identifiers are never sent to the WebView.
type providerConfigList struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Providers       []providerConfigView `json:"providers"`
	Presets         []providerPresetView `json:"presets"`
}

type providerPresetRouteView struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	BaseURL string   `json:"baseUrl"`
	Models  []string `json:"models"`
	Default string   `json:"default"`
}

type providerPresetView struct {
	ID          string                    `json:"id"`
	Label       string                    `json:"label"`
	Description string                    `json:"description"`
	Group       string                    `json:"group"`
	Recommended bool                      `json:"recommended"`
	Status      string                    `json:"status"`
	Routes      []providerPresetRouteView `json:"routes"`
	Revision    string                    `json:"revision"`
}

type providerConfigView struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
	Removable   bool     `json:"removable"`
	Revision    string   `json:"revision"`
}

var errPreviewProviderChanged = errors.New("provider settings changed; reload before deleting")

type deleteProviderConfigRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
	Revision    string   `json:"revision"`
}

type saveProviderConfigRequest struct {
	PresetID    string   `json:"presetId"`
	Revision    string   `json:"revision"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	BaseURL     string   `json:"baseUrl"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
	UseAPIKey   bool     `json:"useApiKey"`
}

var previewProviderName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func previewProviderConfigRevision(path, token string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		raw = []byte("<missing Preview config>")
	} else if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func loadProviderConfigs(token string) (providerConfigList, error) {
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return providerConfigList{}, err
	}
	revision, err := previewProviderConfigRevision(configpkg.UserConfigPath(), token)
	if err != nil {
		return providerConfigList{}, err
	}
	view := providerConfigList{ProtocolVersion: desktopbridge.ProtocolVersion, Providers: make([]providerConfigView, 0, len(cfg.Providers)), Presets: previewProviderPresets(cfg, revision)}
	for i := range cfg.Providers {
		entry := &cfg.Providers[i]
		view.Providers = append(view.Providers, providerConfigView{
			Name: entry.Name, DisplayName: entry.DisplayName, Kind: entry.Kind,
			Models: append([]string(nil), entry.ModelList()...), Default: entry.Default,
			Removable: !configpkg.IsOfficialDeepSeekProvider(entry),
			Revision:  revision,
		})
	}
	return view, nil
}

func previewProviderPresets(cfg *configpkg.Config, revision string) []providerPresetView {
	presets := configpkg.CuratedProviderPresets()
	views := make([]providerPresetView, 0, len(presets))
	for _, preset := range presets {
		catalog := configpkg.CatalogForProviderPreset(preset)
		group := catalog.BrandLabel
		if group == "" {
			group = preset.DisplayGroup
		}
		view := providerPresetView{ID: preset.ID, Label: preset.Label, Description: preset.Description, Group: group, Recommended: preset.Recommended, Revision: revision, Routes: make([]providerPresetRouteView, 0, len(preset.Entries))}
		installed, conflict := 0, false
		for _, entry := range preset.Entries {
			view.Routes = append(view.Routes, providerPresetRouteView{Name: entry.Name, Kind: entry.Kind, BaseURL: entry.BaseURL, Models: append([]string{}, entry.ModelList()...), Default: entry.DefaultModel()})
			if existing, ok := cfg.Provider(entry.Name); ok {
				if existing.PresetID == preset.ID {
					installed++
				} else {
					conflict = true
				}
			}
		}
		switch {
		case conflict:
			view.Status = "name_conflict"
		case installed == len(preset.Entries):
			view.Status = "installed"
		case installed > 0:
			view.Status = "partial"
		default:
			view.Status = "available"
		}
		views = append(views, view)
	}
	return views
}

func persistProviderPreset(input saveProviderConfigRequest, token string) error {
	preset, ok := configpkg.CuratedProviderPreset(input.PresetID)
	if !ok || len(preset.Entries) == 0 || len(preset.Entries) > 8 {
		return fmt.Errorf("unknown or unsupported provider preset")
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("Preview profile unavailable")
	}
	revision, err := previewProviderConfigRevision(path, token)
	if err != nil {
		return err
	}
	if input.Revision == "" || !hmac.Equal([]byte(revision), []byte(input.Revision)) {
		return errPreviewProviderChanged
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return err
	}
	baseline := cfg.ModelSettingsBaseline()
	missing := make([]configpkg.ProviderEntry, 0, len(preset.Entries))
	for _, entry := range preset.Entries {
		if existing, ok := cfg.Provider(entry.Name); ok {
			if existing.PresetID != preset.ID {
				return fmt.Errorf("provider name %q is already used; review it before installing this preset", entry.Name)
			}
			continue
		}
		missing = append(missing, entry)
	}
	if len(missing) == 0 {
		return fmt.Errorf("provider preset is already installed")
	}
	for _, entry := range missing {
		if entry.DisplayName == "" {
			entry.DisplayName = preset.Label
		}
		if err := cfg.UpsertProvider(entry); err != nil {
			return err
		}
		if cfg.Desktop.ProviderAccess != nil && !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
			cfg.Desktop.ProviderAccess = append(cfg.Desktop.ProviderAccess, entry.Name)
		}
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func removeProviderConfig(input deleteProviderConfigRequest, token string) error {
	if !previewProviderName.MatchString(input.Name) || len(input.Models) > 100 {
		return fmt.Errorf("invalid provider identity")
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("Preview profile unavailable")
	}
	revision, err := previewProviderConfigRevision(path, token)
	if err != nil {
		return err
	}
	if input.Revision == "" || !hmac.Equal([]byte(revision), []byte(input.Revision)) {
		return errPreviewProviderChanged
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return err
	}
	entry, found := cfg.Provider(input.Name)
	if !found {
		return errPreviewProviderChanged
	}
	if entry.DisplayName != input.DisplayName || entry.Kind != input.Kind || entry.Default != input.Default || !slices.Equal(entry.ModelList(), input.Models) {
		return errPreviewProviderChanged
	}
	if configpkg.IsOfficialDeepSeekProvider(entry) {
		return fmt.Errorf("official provider access cannot be deleted here")
	}
	baseline := cfg.ModelSettingsBaseline()
	if err := cfg.RemoveProvider(input.Name); err != nil {
		return fmt.Errorf("another configured provider is required before deleting this service")
	}
	if cfg.Desktop.ProviderAccess != nil {
		cfg.Desktop.ProviderAccess = slices.DeleteFunc(cfg.Desktop.ProviderAccess, func(name string) bool { return strings.TrimSpace(name) == input.Name })
		if resolved, ok := cfg.ResolveModel(cfg.DefaultModel); ok && !providerAccessAllowed(cfg.Desktop.ProviderAccess, resolved.Name) {
			fallback := ""
			for i := range cfg.Providers {
				candidate := &cfg.Providers[i]
				if providerAccessAllowed(cfg.Desktop.ProviderAccess, candidate.Name) && candidate.Configured() && len(candidate.ModelList()) > 0 {
					fallback = candidate.Name
					break
				}
			}
			if fallback == "" {
				return fmt.Errorf("no visible configured provider remains for the default model")
			}
			if err := cfg.SetDefaultModel(fallback); err != nil {
				return err
			}
		}
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
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
