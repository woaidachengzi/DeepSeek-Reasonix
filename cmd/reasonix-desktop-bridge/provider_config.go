package main

import (
	"context"
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
	"time"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
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

type providerModelReasoning struct {
	Model             string                     `json:"model"`
	ReasoningProtocol string                     `json:"reasoningProtocol"`
	SupportedEfforts  []string                   `json:"supportedEfforts"`
	DefaultEffort     string                     `json:"defaultEffort"`
	Options           []provider.ReasoningOption `json:"options,omitempty"`
}

type providerConfigView struct {
	ModelReasoning []providerModelReasoning `json:"modelReasoning"`
	Name           string                   `json:"name"`
	DisplayName    string                   `json:"displayName"`
	Kind           string                   `json:"kind"`
	Models         []string                 `json:"models"`
	Default        string                   `json:"default"`
	ModelsURLSet   bool                     `json:"modelsUrlSet"`
	NoProxy        bool                     `json:"noProxy"`
	ContextWindow  int                      `json:"contextWindow"`
	ResponsesMode  string                   `json:"responsesMode"`
	BalanceURLSet  bool                     `json:"balanceUrlSet"`
	Removable      bool                     `json:"removable"`
	Hidden         bool                     `json:"hidden"`
	Revision       string                   `json:"revision"`
}

var errPreviewProviderChanged = errors.New("provider settings changed; reload before deleting")
var errPreviewProviderDiscoveryChanged = errors.New("provider settings changed; reload before discovering models")

type discoverProviderModelsRequest struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type discoverProviderModelsResponse struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Models          []string `json:"models"`
}

func discoverSavedProviderModels(input discoverProviderModelsRequest, token string) (discoverProviderModelsResponse, error) {
	if !previewProviderName.MatchString(input.Name) {
		return discoverProviderModelsResponse{}, fmt.Errorf("invalid provider identity")
	}
	unlock := configpkg.LockUserConfigEdits()
	path := configpkg.UserConfigPath()
	if path == "" {
		unlock()
		return discoverProviderModelsResponse{}, fmt.Errorf("Preview profile unavailable")
	}
	revision, err := previewProviderConfigRevision(path, token)
	if err != nil || input.Revision == "" || !hmac.Equal([]byte(revision), []byte(input.Revision)) {
		unlock()
		return discoverProviderModelsResponse{}, errPreviewProviderDiscoveryChanged
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		unlock()
		return discoverProviderModelsResponse{}, fmt.Errorf("unable to load provider settings")
	}
	entry, found := cfg.Provider(input.Name)
	if !found || !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Name) {
		unlock()
		return discoverProviderModelsResponse{}, fmt.Errorf("provider is unavailable")
	}
	entry.ResolveAPIKeyForRoot(".")
	if !entry.Configured() {
		unlock()
		return discoverProviderModelsResponse{}, fmt.Errorf("provider is not configured")
	}
	proxy := cfg.NetworkProxySpec()
	unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	models, err := entry.FetchModelsWithProxy(ctx, proxy)
	if err != nil {
		// Fetch errors may contain endpoint or transport details. Keep them in
		// the local bridge log only; never return credentials or provider URLs.
		return discoverProviderModelsResponse{}, fmt.Errorf("model discovery failed; check the saved endpoint and credentials")
	}
	if len(models) > 500 {
		models = models[:500]
	}
	// Do not return a catalog fetched against a config that changed mid-request.
	unlock = configpkg.LockUserConfigEdits()
	defer unlock()
	currentRevision, err := previewProviderConfigRevision(path, token)
	if err != nil || !hmac.Equal([]byte(currentRevision), []byte(input.Revision)) {
		return discoverProviderModelsResponse{}, errPreviewProviderDiscoveryChanged
	}
	return discoverProviderModelsResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Models: models}, nil
}

type deleteProviderConfigRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Kind        string   `json:"kind"`
	Models      []string `json:"models"`
	Default     string   `json:"default"`
	Revision    string   `json:"revision"`
	Restore     bool     `json:"restore"`
}

type saveProviderConfigRequest struct {
	ModelReasoning  *[]providerModelReasoning `json:"modelReasoning,omitempty"`
	PresetID        string                    `json:"presetId"`
	PresetAction    string                    `json:"presetAction"`
	Revision        string                    `json:"revision"`
	Name            string                    `json:"name"`
	DisplayName     string                    `json:"displayName"`
	Kind            string                    `json:"kind"`
	BaseURL         string                    `json:"baseUrl"`
	ModelsURL       string                    `json:"modelsUrl"`
	ClearModelsURL  bool                      `json:"clearModelsUrl"`
	NoProxy         *bool                     `json:"noProxy"`
	ContextWindow   *int                      `json:"contextWindow"`
	ResponsesMode   *string                   `json:"responsesMode"`
	BalanceURL      string                    `json:"balanceUrl"`
	ClearBalanceURL bool                      `json:"clearBalanceUrl"`
	Models          []string                  `json:"models"`
	Default         string                    `json:"default"`
	UseAPIKey       bool                      `json:"useApiKey"`
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
			ModelReasoning: providerReasoningForConfig(*entry),
			Name:           entry.Name, DisplayName: entry.DisplayName, Kind: entry.Kind,
			Models: append([]string(nil), entry.ModelList()...), Default: entry.Default,
			ModelsURLSet: strings.TrimSpace(entry.ModelsURL) != "", NoProxy: entry.NoProxy,
			ContextWindow: entry.ContextWindow, ResponsesMode: entry.ResponsesMode,
			BalanceURLSet: strings.TrimSpace(entry.BalanceURL) != "",
			Removable:     providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name),
			Hidden:        !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name),
			Revision:      revision,
		})
	}
	return view, nil
}

// Expose adapter-owned levels without endpoints or credential identifiers.
func providerReasoningForConfig(entry configpkg.ProviderEntry) []providerModelReasoning {
	cfg := configpkg.Config{Providers: []configpkg.ProviderEntry{entry}}
	out := make([]providerModelReasoning, 0, len(entry.ModelList()))
	for _, model := range entry.ModelList() {
		resolved, ok := cfg.ResolveModel(entry.Name + "/" + model)
		if !ok {
			continue
		}
		ov := entry.ModelOverrides[model]
		cap := configpkg.ReasoningCapabilityForEntry(resolved)
		out = append(out, providerModelReasoning{Model: model, ReasoningProtocol: ov.ReasoningProtocol, SupportedEfforts: ov.SupportedEfforts, DefaultEffort: ov.DefaultEffort, Options: cap.Options})
	}
	return out
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
		installed, modified, conflict := 0, false, false
		for _, entry := range preset.Entries {
			view.Routes = append(view.Routes, providerPresetRouteView{Name: entry.Name, Kind: entry.Kind, BaseURL: entry.BaseURL, Models: append([]string{}, entry.ModelList()...), Default: entry.DefaultModel()})
			if existing, ok := cfg.Provider(entry.Name); ok {
				if existing.PresetID == preset.ID {
					installed++
					modified = modified || previewPresetRouteModified(*existing, entry)
				} else {
					conflict = true
				}
			}
		}
		switch {
		case conflict:
			view.Status = "name_conflict"
		case installed == len(preset.Entries):
			if modified {
				view.Status = "installed_modified"
			} else {
				view.Status = "installed"
			}
		case installed > 0:
			view.Status = "partial"
		default:
			view.Status = "available"
		}
		views = append(views, view)
	}
	return views
}

func previewPresetRouteModified(existing, preset configpkg.ProviderEntry) bool {
	return existing.Kind != preset.Kind || existing.BaseURL != preset.BaseURL || existing.ChatURL != preset.ChatURL ||
		existing.RequestURL != preset.RequestURL || existing.AuthHeader != preset.AuthHeader ||
		!slices.Equal(existing.ModelList(), preset.ModelList()) || existing.DefaultModel() != preset.DefaultModel() ||
		existing.NoProxy != preset.NoProxy || existing.ContextWindow != preset.ContextWindow
}

func persistProviderPreset(input saveProviderConfigRequest, token string) error {
	preset, ok := configpkg.CuratedProviderPreset(input.PresetID)
	if !ok || len(preset.Entries) == 0 || len(preset.Entries) > 8 {
		return fmt.Errorf("unknown or unsupported provider preset")
	}
	if input.PresetAction != "" && input.PresetAction != "add" && input.PresetAction != "reset" {
		return fmt.Errorf("unsupported provider preset action")
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
	if input.PresetAction == "reset" {
		for _, entry := range preset.Entries {
			existing, ok := cfg.Provider(entry.Name)
			if !ok || existing.PresetID != preset.ID {
				return fmt.Errorf("preset reset requires every route to be installed from this preset")
			}
		}
		for _, entry := range preset.Entries {
			existing, _ := cfg.Provider(entry.Name)
			entry.APIKeyEnv = existing.APIKeyEnv
			if existing.DisplayName != "" {
				entry.DisplayName = existing.DisplayName
			} else if entry.DisplayName == "" {
				entry.DisplayName = preset.Label
			}
			if err := cfg.UpsertProvider(entry); err != nil {
				return err
			}
		}
		if _, ok := cfg.ResolveModel(cfg.DefaultModel); !ok {
			providerName, _, qualified := strings.Cut(cfg.DefaultModel, "/")
			if qualified {
				for _, entry := range preset.Entries {
					if entry.Name == providerName {
						if err := cfg.SetDefaultModel(entry.Name + "/" + entry.DefaultModel()); err != nil {
							return err
						}
						break
					}
				}
			}
		}
		return cfg.SaveUserSettingsDeltaTo(path, baseline)
	}
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
	baseline := cfg.ModelSettingsBaseline()
	if input.Restore {
		if providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Name) {
			return errPreviewProviderChanged
		}
		cfg.Desktop.ProviderAccess = append(cfg.Desktop.ProviderAccess, input.Name)
		return cfg.SaveUserSettingsDeltaTo(path, baseline)
	}
	if !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Name) {
		return errPreviewProviderChanged
	}
	if configpkg.IsOfficialDeepSeekProvider(entry) {
		// Canonical official entries may be recreated by config normalization.
		// Remove access instead, retaining credentials and an explicit restore path.
		if cfg.Desktop.ProviderAccess == nil {
			cfg.Desktop.ProviderAccess = make([]string, 0, len(cfg.Providers))
			for _, provider := range cfg.Providers {
				cfg.Desktop.ProviderAccess = append(cfg.Desktop.ProviderAccess, provider.Name)
			}
		}
		cfg.Desktop.ProviderAccess = slices.DeleteFunc(cfg.Desktop.ProviderAccess, func(name string) bool { return strings.TrimSpace(name) == input.Name })
		fallback := ""
		for i := range cfg.Providers {
			candidate := &cfg.Providers[i]
			candidate.ResolveAPIKeyForRoot(".")
			if providerAccessAllowed(cfg.Desktop.ProviderAccess, candidate.Name) && candidate.Configured() && len(candidate.ModelList()) > 0 {
				fallback = candidate.Name + "/" + candidate.DefaultModel()
				break
			}
		}
		retargetPreviewProviderReferences(cfg, input.Name, fallback)
		return cfg.SaveUserSettingsDeltaTo(path, baseline)
	}
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
	input.BalanceURL = strings.TrimSpace(input.BalanceURL)
	input.ModelsURL = strings.TrimSpace(input.ModelsURL)
	if input.ResponsesMode != nil {
		value := strings.ToLower(strings.TrimSpace(*input.ResponsesMode))
		input.ResponsesMode = &value
	}
	input.Default = strings.TrimSpace(input.Default)
	if input.ClearBalanceURL && input.BalanceURL != "" {
		return fmt.Errorf("choose a balance URL or clear the saved URL")
	}
	if input.ClearModelsURL && input.ModelsURL != "" {
		return fmt.Errorf("choose a model discovery URL or clear the saved URL")
	}
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
	if input.ModelReasoning != nil {
		seenReasoning := map[string]bool{}
		for _, item := range *input.ModelReasoning {
			if !seen[item.Model] || seenReasoning[item.Model] {
				return fmt.Errorf("reasoning settings must reference distinct configured models")
			}
			seenReasoning[item.Model] = true
			switch item.ReasoningProtocol {
			case "", "auto", "openai", "deepseek", "glm", "kimi-k3", "none":
			default:
				return fmt.Errorf("invalid reasoning protocol")
			}
			if len(item.SupportedEfforts) > 32 {
				return fmt.Errorf("too many reasoning levels")
			}
			levels := map[string]bool{}
			for _, level := range item.SupportedEfforts {
				if level == "auto" || len(level) > 64 || !previewProviderName.MatchString(level) || strings.ToLower(level) != level || levels[level] {
					return fmt.Errorf("reasoning levels must be distinct lowercase identifiers; use automatic for inheritance")
				}
				levels[level] = true
			}
			if item.DefaultEffort != "" && len(item.SupportedEfforts) > 0 && !levels[item.DefaultEffort] {
				return fmt.Errorf("default reasoning level must be selected in available levels")
			}
		}
	}
	if input.BaseURL != "" {
		if err := validateProviderURL(input.BaseURL); err != nil {
			return fmt.Errorf("endpoint %w", err)
		}
	}
	if input.BalanceURL != "" {
		if err := validateProviderURL(input.BalanceURL); err != nil {
			return fmt.Errorf("balance URL %w", err)
		}
	}
	if input.ModelsURL != "" {
		if err := validateProviderURL(input.ModelsURL); err != nil {
			return fmt.Errorf("model discovery URL %w", err)
		}
	}
	if input.ContextWindow != nil && (*input.ContextWindow < 0 || *input.ContextWindow > 10_000_000) {
		return fmt.Errorf("context window must be between 0 and 10000000")
	}
	if input.ResponsesMode != nil && *input.ResponsesMode != "" && *input.ResponsesMode != "auto" && *input.ResponsesMode != "stateless" && *input.ResponsesMode != "stateful" {
		return fmt.Errorf("invalid Responses API mode")
	}
	return nil
}

func validateProviderURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must be a URL without embedded credentials or query")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("must use HTTPS or loopback HTTP")
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
	if input.ClearBalanceURL {
		entry.BalanceURL = ""
	} else if input.BalanceURL != "" {
		entry.BalanceURL = input.BalanceURL
	}
	if input.ClearModelsURL {
		entry.ModelsURL = ""
	} else if input.ModelsURL != "" {
		entry.ModelsURL = input.ModelsURL
	}
	if input.NoProxy != nil {
		entry.NoProxy = *input.NoProxy
	}
	if input.ContextWindow != nil {
		entry.ContextWindow = *input.ContextWindow
	}
	if entry.Kind == "responses" && input.ResponsesMode != nil {
		entry.ResponsesMode = *input.ResponsesMode
	}
	entry.Model = input.Models[0]
	entry.Models = append([]string(nil), input.Models...)
	entry.Default = input.Default
	if input.ModelReasoning != nil {
		if entry.ModelOverrides == nil {
			entry.ModelOverrides = map[string]configpkg.ProviderModelOverride{}
		}
		for _, item := range *input.ModelReasoning {
			ov := entry.ModelOverrides[item.Model]
			ov.ReasoningProtocol = item.ReasoningProtocol
			ov.SupportedEfforts = append([]string(nil), item.SupportedEfforts...)
			ov.DefaultEffort = item.DefaultEffort
			entry.ModelOverrides[item.Model] = ov
		}
		candidate := configpkg.Config{Providers: []configpkg.ProviderEntry{entry}}
		for _, item := range *input.ModelReasoning {
			resolved, _ := candidate.ResolveModel(entry.Name + "/" + item.Model)
			cap := configpkg.ReasoningCapabilityForEntry(resolved)
			if err := cap.Validate(item.Model, configpkg.EffectiveEffort(resolved)); err != nil {
				return err
			}
			for _, level := range item.SupportedEfforts {
				if !slices.Contains(cap.IDs(), level) {
					return fmt.Errorf("model %q does not support reasoning level %q", item.Model, level)
				}
			}
		}
	}
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

// Match qualified, bare-provider and unqualified model refs before mutating
// defaults, using the same access-removal semantics as the Wails baseline.
func retargetPreviewProviderReferences(cfg *configpkg.Config, name, fallback string) {
	matches := func(ref string) bool {
		if strings.TrimSpace(ref) == "" {
			return false
		}
		if provider, _, qualified := strings.Cut(ref, "/"); qualified {
			return provider == name
		}
		if ref == name {
			return true
		}
		resolved, ok := cfg.ResolveModel(ref)
		return ok && resolved.Name == name
	}
	refs := []*string{&cfg.DefaultModel, &cfg.Agent.PlannerModel, &cfg.Agent.GuardianModel, &cfg.Agent.RecoveryModel, &cfg.Agent.SubagentModel, &cfg.Bot.Model, &cfg.Bot.QQ.Model, &cfg.Bot.Dingtalk.Model}
	for i := range cfg.Bot.Routes {
		refs = append(refs, &cfg.Bot.Routes[i].Model)
	}
	for i := range cfg.Bot.Connections {
		refs = append(refs, &cfg.Bot.Connections[i].Model)
	}
	// Resolve all references against the original default, then mutate them.
	matched := make([]*string, 0, len(refs))
	for _, ref := range refs {
		if matches(*ref) {
			matched = append(matched, ref)
		}
	}
	visionMatches := matches(cfg.Agent.VisionModel)
	for skill, ref := range cfg.Agent.SubagentModels {
		if matches(ref) {
			if fallback == "" {
				delete(cfg.Agent.SubagentModels, skill)
			} else {
				cfg.Agent.SubagentModels[skill] = fallback
			}
		}
	}
	for _, ref := range matched {
		*ref = fallback
	}
	if visionMatches {
		cfg.Agent.VisionModel = ""
	}
}
