package main

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type providerSummaryResponse struct {
	ProtocolVersion     int                    `json:"protocolVersion"`
	Scope               string                 `json:"scope"`
	DefaultModel        string                 `json:"defaultModel"`
	PlannerModel        string                 `json:"plannerModel"`
	VisionModel         string                 `json:"visionModel"`
	WebSearchModel      string                 `json:"webSearchModel"`
	ReasoningLanguage   string                 `json:"reasoningLanguage"`
	CompactRatioPercent float64                `json:"compactRatioPercent"`
	Providers           []providerSummaryEntry `json:"providers"`
}

type providerSummaryEntry struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"displayName,omitempty"`
	Kind         string   `json:"kind"`
	Models       []string `json:"models"`
	VisionModels []string `json:"visionModels"`
	SearchModels []string `json:"searchModels"`
	ModelCount   int      `json:"modelCount"`
	RequiresKey  bool     `json:"requiresKey"`
	Configured   bool     `json:"configured"`
}

type setDefaultModelRequest struct {
	Model         string `json:"model"`
	Scope         string `json:"scope,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
}

type setSessionModelRequest struct {
	Model string `json:"model"`
}

type setModelRoleRequest struct {
	Role          string `json:"role"`
	Model         string `json:"model"`
	Scope         string `json:"scope,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
}

type setAgentPreferenceRequest struct {
	ReasoningLanguage   string  `json:"reasoningLanguage,omitempty"`
	CompactRatioPercent float64 `json:"compactRatioPercent,omitempty"`
	Scope               string  `json:"scope,omitempty"`
	WorkspaceRoot       string  `json:"workspaceRoot,omitempty"`
}

type desktopPreferencesResponse struct {
	ExternalOpener          string `json:"externalOpener"`
	ProtocolVersion         int    `json:"protocolVersion"`
	DefaultToolApprovalMode string `json:"defaultToolApprovalMode"`
	Language                string `json:"language"`
	DisplayCurrency         string `json:"displayCurrency"`
	TerminalTheme           string `json:"terminalTheme"`
	Theme                   string `json:"theme"`
	ThemeStyle              string `json:"themeStyle"`
	AppearanceConfigured    bool   `json:"appearanceConfigured"`
}

type setDesktopApprovalRequest struct {
	Mode string `json:"mode"`
}

type setDesktopCurrencyRequest struct {
	Currency string `json:"currency"`
}

func loadDesktopPreferences() (desktopPreferencesResponse, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return desktopPreferencesResponse{}, err
	}
	return desktopPreferencesResponse{
		ProtocolVersion:         desktopbridge.ProtocolVersion,
		ExternalOpener:          cfg.DesktopExternalOpener(),
		DefaultToolApprovalMode: cfg.DesktopDefaultToolApprovalMode(),
		Language:                cfg.DesktopLanguage(),
		DisplayCurrency:         cfg.DisplayCurrencyPref(),
		TerminalTheme:           cfg.DesktopTerminalTheme(),
		Theme:                   cfg.DesktopTheme(),
		ThemeStyle:              cfg.DesktopThemeStyle(),
		AppearanceConfigured:    strings.TrimSpace(cfg.Desktop.Theme) != "" || strings.TrimSpace(cfg.Desktop.ThemeStyle) != "",
	}, nil
}

func persistDesktopCurrency(currency string) error {
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
	if err := cfg.SetDisplayCurrency(currency); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func persistDesktopLanguage(language string) error {
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
	if err := cfg.SetDesktopLanguage(language); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func persistDesktopApprovalMode(mode string) error {
	if mode != "ask" && mode != "auto" && mode != "yolo" {
		return fmt.Errorf("invalid approval mode")
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
	if err := cfg.SetDesktopDefaultToolApprovalMode(mode); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func persistDesktopTerminalTheme(theme string) error {
	if theme != "auto" && theme != "dark" && theme != "light" {
		return fmt.Errorf("invalid terminal theme")
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
	if err := cfg.SetDesktopTerminalTheme(theme); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func persistDesktopAppearance(theme, style string) error {
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
	if err := cfg.SetDesktopAppearance(theme, style); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

// This restore is private to the authenticated native host. A normal setter
// cannot represent an unset appearance; rollback must also retain that state.
type desktopAppearanceSnapshot struct {
	Theme      string `json:"theme"`
	Style      string `json:"style"`
	Configured bool   `json:"configured"`
}

type restoreDesktopAppearanceRequest struct {
	Expected desktopAppearanceSnapshot `json:"expected"`
	Previous desktopAppearanceSnapshot `json:"previous"`
}

func (r restoreDesktopAppearanceRequest) valid() bool {
	return r.Expected.Configured && validAppearanceSnapshot(r.Expected) && validAppearanceSnapshot(r.Previous)
}

var errDesktopAppearanceChanged = errors.New("desktop appearance changed before rollback")

func validAppearanceSnapshot(value desktopAppearanceSnapshot) bool {
	if value.Theme != "auto" && value.Theme != "dark" && value.Theme != "light" {
		return false
	}
	if !value.Configured && (value.Theme != "auto" || value.Style != "") {
		return false
	}
	// A previous preference can contain a supported legacy Wails style that
	// the Preview's ordinary settings picker does not offer.
	cfg := &configpkg.Config{}
	cfg.Desktop.ThemeStyle = value.Style
	return cfg.DesktopThemeStyle() == value.Style
}

func restoreDesktopAppearance(request restoreDesktopAppearanceRequest) error {
	if !request.valid() {
		return fmt.Errorf("invalid desktop appearance rollback")
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
	configured := strings.TrimSpace(cfg.Desktop.Theme) != "" || strings.TrimSpace(cfg.Desktop.ThemeStyle) != ""
	if cfg.DesktopTheme() != request.Expected.Theme || cfg.DesktopThemeStyle() != request.Expected.Style || configured != request.Expected.Configured {
		return errDesktopAppearanceChanged
	}
	baseline := cfg.ModelSettingsBaseline()
	if request.Previous.Configured {
		cfg.Desktop.Theme, cfg.Desktop.ThemeStyle = request.Previous.Theme, request.Previous.Style
	} else {
		cfg.Desktop.Theme, cfg.Desktop.ThemeStyle = "", ""
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
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
	return loadProviderSummaryForScope("global", "")
}

func loadProviderSummaryForScope(scope, workspaceRoot string) (providerSummaryResponse, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	if scope != "global" && scope != "project" {
		return providerSummaryResponse{}, fmt.Errorf("invalid model settings scope")
	}
	root := "."
	var cfg *configpkg.Config
	var err error
	if scope == "project" {
		root, err = normalizeSkillsWorkspace(workspaceRoot)
		if err != nil || root == "" {
			return providerSummaryResponse{}, fmt.Errorf("project model settings require a valid workspace")
		}
		cfg, err = configpkg.LoadForRootReadOnly(root)
	} else {
		cfg, err = configpkg.LoadUserConfigReadOnly()
	}
	if err != nil {
		return providerSummaryResponse{}, fmt.Errorf("load %s provider configuration: %w", scope, err)
	}
	providers := make([]providerSummaryEntry, 0, len(cfg.Providers))
	visionResolver := configpkg.NewModelCapabilityResolver()
	for i := range cfg.Providers {
		provider := &cfg.Providers[i]
		if !providerAccessAllowed(cfg.Desktop.ProviderAccess, provider.Name) {
			continue
		}
		requiresKey := provider.RequiresAPIKey()
		provider.ResolveAPIKeyForRoot(root)
		configured := (!requiresKey || provider.Configured()) && providerAccessAllowed(cfg.Desktop.ProviderAccess, provider.Name)
		models := provider.ModelList()
		if models == nil {
			models = []string{}
		}
		visionModels := make([]string, 0)
		searchModels := make([]string, 0)
		if configured {
			for _, model := range models {
				resolved, ok := cfg.ResolveModel(provider.Name + "/" + model)
				if ok && visionResolver.Resolve(resolved).State == configpkg.CapabilitySupported {
					visionModels = append(visionModels, model)
				}
				if _, err := cfg.ResolveWebSearchModel(provider.Name + "/" + model); err == nil {
					searchModels = append(searchModels, model)
				}
			}
		}
		entry := providerSummaryEntry{
			Name:         provider.Name,
			DisplayName:  strings.TrimSpace(provider.DisplayName),
			Kind:         provider.Kind,
			Models:       models,
			VisionModels: visionModels,
			SearchModels: searchModels,
			ModelCount:   len(models),
			RequiresKey:  requiresKey,
			Configured:   configured,
		}
		providers = append(providers, entry)
	}
	return providerSummaryResponse{
		ProtocolVersion:     desktopbridge.ProtocolVersion,
		Scope:               scope,
		DefaultModel:        cfg.DefaultModel,
		PlannerModel:        cfg.Agent.PlannerModel,
		VisionModel:         cfg.Agent.VisionModel,
		WebSearchModel:      cfg.Agent.WebSearchModel,
		ReasoningLanguage:   cfg.ReasoningLanguage(),
		CompactRatioPercent: math.Round(cfg.Agent.CompactRatio*1000) / 10,
		Providers:           providers,
	}, nil
}

func persistAgentPreferences(request setAgentPreferenceRequest) error {
	if (request.ReasoningLanguage == "") == (request.CompactRatioPercent == 0) {
		return fmt.Errorf("exactly one agent preference must be provided")
	}
	scope, root, err := normalizeModelSettingsScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		return err
	}
	if scope == "project" {
		if request.ReasoningLanguage != "" && request.ReasoningLanguage != "auto" && request.ReasoningLanguage != "zh" && request.ReasoningLanguage != "en" {
			return fmt.Errorf("invalid reasoning language")
		}
		if request.CompactRatioPercent != 0 && !validCompactRatioPercent(request.CompactRatioPercent) {
			return fmt.Errorf("compact ratio percent must be between 30 and 85 in 0.1 increments")
		}
		path := filepath.Join(root, "reasonix.toml")
		return configpkg.EditProjectConfigFileWithoutCredentials(path, func(cfg *configpkg.Config) error {
			if request.ReasoningLanguage != "" {
				return cfg.SetReasoningLanguage(request.ReasoningLanguage)
			}
			return cfg.SetCompactRatio(request.CompactRatioPercent / 100)
		})
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return fmt.Errorf("load Preview user config: %w", err)
	}
	baseline := cfg.ModelSettingsBaseline()
	if request.ReasoningLanguage != "" {
		if err := cfg.SetReasoningLanguage(request.ReasoningLanguage); err != nil {
			return err
		}
	} else {
		if !validCompactRatioPercent(request.CompactRatioPercent) {
			return fmt.Errorf("compact ratio percent must be between 30 and 85 in 0.1 increments")
		}
		if err := cfg.SetCompactRatio(float64(request.CompactRatioPercent) / 100); err != nil {
			return err
		}
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}

func validCompactRatioPercent(percent float64) bool {
	return percent >= 30 && percent <= 85 && math.Abs(percent*10-math.Round(percent*10)) <= 1e-7
}

func normalizeModelSettingsScope(scope, workspaceRoot string) (string, string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	if scope == "global" {
		return scope, "", nil
	}
	if scope != "project" {
		return "", "", fmt.Errorf("invalid model settings scope")
	}
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil || root == "" {
		return "", "", fmt.Errorf("project model settings require a valid workspace")
	}
	return scope, root, nil
}

func persistModelRole(request setModelRoleRequest) error {
	if request.Role != "planner" && request.Role != "vision" && request.Role != "search" {
		return errDefaultModelUnavailable
	}
	ref := strings.TrimSpace(request.Model)
	scope, root, err := normalizeModelSettingsScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		return err
	}
	if scope == "project" {
		cfg, err := configpkg.LoadForRootReadOnly(root)
		if err != nil {
			return errDefaultModelUnavailable
		}
		if err := validateModelRole(cfg, request.Role, ref, root); err != nil {
			return err
		}
		return configpkg.EditProjectConfigFileWithoutCredentials(filepath.Join(root, "reasonix.toml"), func(projectCfg *configpkg.Config) error {
			switch request.Role {
			case "planner":
				projectCfg.Agent.PlannerModel = ref
			case "vision":
				projectCfg.Agent.VisionModel = ref
			case "search":
				projectCfg.Agent.WebSearchModel = ref
			}
			return nil
		})
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return fmt.Errorf("load Preview user config: %w", err)
	}
	baseline := cfg.ModelSettingsBaseline()
	if ref != "" && !((request.Role == "vision" || request.Role == "search") && strings.EqualFold(ref, "auto")) {
		if request.Role == "search" {
			name, _, ok := strings.Cut(ref, "/")
			if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, name) {
				return errDefaultModelUnavailable
			}
			provider, ok := cfg.Provider(name)
			if !ok {
				return errDefaultModelUnavailable
			}
			provider.ResolveAPIKeyForRoot(".")
			if _, err := cfg.ResolveWebSearchModel(ref); err != nil {
				return errDefaultModelUnavailable
			}
		}
		if request.Role != "search" {
			entry, ok := cfg.ResolveModel(ref)
			if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
				return errDefaultModelUnavailable
			}
			entry.ResolveAPIKeyForRoot(".")
			if !entry.Configured() {
				return errDefaultModelUnavailable
			}
			if request.Role == "vision" && configpkg.NewModelCapabilityResolver().Resolve(entry).State != configpkg.CapabilitySupported {
				return errDefaultModelUnavailable
			}
			ref = entry.Name + "/" + entry.Model
		}
	}
	if request.Role == "planner" {
		// Desktop supports an explicit model ref for the planner role.
		cfg.Agent.PlannerModel = ref
	} else if request.Role == "vision" {
		if strings.EqualFold(ref, "auto") {
			ref = "auto"
		}
		cfg.Agent.VisionModel = ref
	} else {
		if err := cfg.SetWebSearchModel(ref); err != nil {
			return errDefaultModelUnavailable
		}
		if err := cfg.SaveWebSearchModelTo(path); err != nil {
			return fmt.Errorf("save Preview search model: %w", err)
		}
		return nil
	}
	if err := cfg.SaveModelSettingsTo(path, baseline); err != nil {
		return fmt.Errorf("save Preview %s model: %w", request.Role, err)
	}
	return nil
}

func validateModelRole(cfg *configpkg.Config, role, ref, root string) error {
	if ref == "" || (role == "vision" || role == "search") && strings.EqualFold(ref, "auto") {
		return nil
	}
	if role == "search" {
		name, _, ok := strings.Cut(ref, "/")
		if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, name) {
			return errDefaultModelUnavailable
		}
		provider, ok := cfg.Provider(name)
		if !ok {
			return errDefaultModelUnavailable
		}
		provider.ResolveAPIKeyForRoot(root)
		if _, err := cfg.ResolveWebSearchModel(ref); err != nil {
			return errDefaultModelUnavailable
		}
		return nil
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
		return errDefaultModelUnavailable
	}
	entry.ResolveAPIKeyForRoot(root)
	if !entry.Configured() {
		return errDefaultModelUnavailable
	}
	if role == "vision" && configpkg.NewModelCapabilityResolver().Resolve(entry).State != configpkg.CapabilitySupported {
		return errDefaultModelUnavailable
	}
	return nil
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
	return persistDefaultModelForScope(ref, "global", "")
}

func persistDefaultModelForScope(ref, requestedScope, workspaceRoot string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errDefaultModelUnavailable
	}
	scope, root, err := normalizeModelSettingsScope(requestedScope, workspaceRoot)
	if err != nil {
		return err
	}
	if scope == "project" {
		cfg, err := configpkg.LoadForRootReadOnly(root)
		if err != nil {
			return errDefaultModelUnavailable
		}
		entry, ok := cfg.ResolveModel(ref)
		if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
			return errDefaultModelUnavailable
		}
		entry.ResolveAPIKeyForRoot(root)
		if !entry.Configured() {
			return errDefaultModelUnavailable
		}
		resolved := entry.Name + "/" + entry.Model
		return configpkg.EditProjectConfigFileWithoutCredentials(filepath.Join(root, "reasonix.toml"), func(projectCfg *configpkg.Config) error {
			projectCfg.DefaultModel = resolved
			return nil
		})
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

func resolveConfiguredSessionModel(workspaceRoot, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errDefaultModelUnavailable
	}
	cfg, err := configpkg.LoadForRoot(workspaceRoot)
	if err != nil {
		return "", errDefaultModelUnavailable
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
		return "", errDefaultModelUnavailable
	}
	entry.ResolveAPIKeyForRoot(".")
	if !entry.Configured() {
		return "", errDefaultModelUnavailable
	}
	return entry.Name + "/" + entry.Model, nil
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

// Host installation detection stays in Rust; the bridge persists only a stable
// preference ID using the shared config lock and narrow TOML delta writer.
func persistDesktopExternalOpener(id string) error {
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
	if err := cfg.SetDesktopExternalOpener(id); err != nil {
		return err
	}
	return cfg.SaveUserSettingsDeltaTo(path, baseline)
}
