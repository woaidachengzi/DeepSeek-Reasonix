package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/skill"
)

type previewSubagentProfile struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Scope            string   `json:"scope"`
	Invocation       string   `json:"invocation"`
	ConfiguredModel  string   `json:"configuredModel"`
	ConfiguredEffort string   `json:"configuredEffort"`
	InvocationMode   string   `json:"invocationMode"`
	Editable         bool     `json:"editable"`
	EditReason       string   `json:"editReason"`
	Revision         string   `json:"revision"`
	Body             string   `json:"body"`
	Color            string   `json:"color"`
	Model            string   `json:"model"`
	Effort           string   `json:"effort"`
	AllowedTools     []string `json:"allowedTools"`
	ReadOnly         bool     `json:"readOnly"`
}

type subagentSettingsView struct {
	ProtocolVersion    int                      `json:"protocolVersion"`
	DefaultModel       string                   `json:"defaultModel"`
	SubagentModel      string                   `json:"subagentModel"`
	SubagentEffort     string                   `json:"subagentEffort"`
	MaxDepth           int                      `json:"maxDepth"`
	MaxConcurrency     int                      `json:"maxConcurrency"`
	MaxParallelWriters int                      `json:"maxParallelWriters"`
	ModelRefs          []string                 `json:"modelRefs"`
	ModelEfforts       map[string][]string      `json:"modelEfforts"`
	Profiles           []previewSubagentProfile `json:"profiles"`
}

type subagentSettingsChange struct {
	WorkspaceRoot string                       `json:"workspaceRoot"`
	Action        string                       `json:"action"`
	Name          string                       `json:"name"`
	Value         string                       `json:"value"`
	Number        int                          `json:"number"`
	Scope         string                       `json:"scope"`
	Revision      string                       `json:"revision"`
	Profile       *previewSubagentProfileInput `json:"profile"`
}

func previewSubagentStore(root string) (*skill.Store, *configpkg.Config, error) {
	var cfg *configpkg.Config
	var err error
	if root == "" {
		cfg, err = configpkg.LoadUserConfigReadOnly()
	} else {
		cfg, err = configpkg.LoadForRootWithoutCredentialsReadOnly(root)
	}
	if err != nil {
		return nil, nil, err
	}
	return skill.New(skill.Options{
		ProjectRoot: root, CustomPaths: cfg.SkillCustomPaths(), ExcludedPaths: cfg.SkillExcludedPaths(),
		PluginPaths: cfg.PluginPackageSkillOwners(), PluginAgentPaths: cfg.PluginPackageAgentOwners(),
		MaxDepth: cfg.SkillMaxDepth(), Stderr: io.Discard,
	}), cfg, nil
}

func subagentOverride(overrides map[string]string, name string) string {
	for _, key := range boot.SubagentModelKeys(name) {
		if value := strings.TrimSpace(overrides[key]); value != "" {
			return value
		}
	}
	return ""
}

func clearSubagentOverride(overrides map[string]string, name string) {
	for _, key := range boot.SubagentModelKeys(name) {
		delete(overrides, key)
	}
}

func loadSubagentSettings(workspaceRoot string) (subagentSettingsView, error) {
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil {
		return subagentSettingsView{}, err
	}
	var cfg *configpkg.Config
	if root == "" {
		cfg, err = configpkg.LoadUserConfigReadOnly()
	} else {
		cfg, err = configpkg.LoadForRootWithoutCredentialsReadOnly(root)
	}
	if err != nil {
		return subagentSettingsView{}, err
	}
	global, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return subagentSettingsView{}, err
	}
	store, _, err := previewSubagentStore(root)
	if err != nil {
		return subagentSettingsView{}, err
	}
	total, writers := agent.NormalizeConcurrencyLimits(cfg.Agent.MaxSubagentConcurrency, cfg.Agent.MaxParallelWriters)
	depth := cfg.Agent.MaxSubagentDepth
	if depth != 1 {
		depth = 2
	}
	view := subagentSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion, DefaultModel: cfg.DefaultModel,
		SubagentModel: cfg.Agent.SubagentModel, SubagentEffort: cfg.Agent.SubagentEffort,
		MaxDepth: depth, MaxConcurrency: total, MaxParallelWriters: writers,
		ModelRefs: []string{}, ModelEfforts: map[string][]string{}, Profiles: []previewSubagentProfile{},
	}
	for _, provider := range global.Providers {
		for _, model := range provider.Models {
			ref := provider.Name + "/" + model
			view.ModelRefs = append(view.ModelRefs, ref)
			if entry, ok := global.ResolveModel(ref); ok {
				capability := configpkg.EffortCapabilityForEntry(entry)
				if capability.Supported {
					view.ModelEfforts[ref] = append([]string{}, capability.Levels...)
				}
			}
		}
	}
	sort.Strings(view.ModelRefs)
	for _, item := range store.List() {
		if item.RunAs != skill.RunSubagent {
			continue
		}
		profile := previewSubagentProfile{
			Name: item.Name, Description: item.Description, Scope: string(item.Scope), Invocation: "/" + item.SlashName(),
			ConfiguredModel:  subagentOverride(cfg.Agent.SubagentModels, item.Name),
			ConfiguredEffort: subagentOverride(cfg.Agent.SubagentEfforts, item.Name),
			InvocationMode:   item.Invocation, Color: item.Color, Model: item.Model,
			Effort: item.Effort, ReadOnly: item.ReadOnly,
			AllowedTools: append([]string{}, item.AllowedTools...),
		}
		if err := skill.ValidateEditableSubagentProfile(item); err != nil {
			profile.EditReason = err.Error()
		} else if revision, err := subagentProfileRevision(item.Path); err != nil {
			profile.EditReason = err.Error()
		} else {
			profile.Editable, profile.Revision, profile.Body = true, revision, item.Body
		}
		view.Profiles = append(view.Profiles, profile)
	}
	return view, nil
}

func persistSubagentSettings(change subagentSettingsChange) (subagentSettingsView, error) {
	root, err := normalizeSkillsWorkspace(change.WorkspaceRoot)
	if err != nil {
		return subagentSettingsView{}, err
	}
	change.Name = strings.TrimSpace(change.Name)
	change.Value = strings.TrimSpace(change.Value)
	if change.Action == "create_profile" || change.Action == "update_profile" || change.Action == "delete_profile" {
		return persistSubagentProfile(change, root)
	}
	if len(change.Name) > 64 || len(change.Value) > 256 {
		return subagentSettingsView{}, fmt.Errorf("invalid subagent setting")
	}
	switch change.Action {
	case "model", "effort", "depth", "concurrency", "writers", "profile_model", "profile_effort":
	default:
		return subagentSettingsView{}, fmt.Errorf("invalid subagent setting action")
	}
	if strings.HasPrefix(change.Action, "profile_") {
		if configpkg.SkillNameKey(change.Name) == "" {
			return subagentSettingsView{}, fmt.Errorf("invalid subagent profile name")
		}
		view, err := loadSubagentSettings(root)
		if err != nil {
			return subagentSettingsView{}, err
		}
		found := false
		for _, profile := range view.Profiles {
			if profile.Name == change.Name {
				found = true
				break
			}
		}
		if !found {
			return subagentSettingsView{}, fmt.Errorf("subagent profile is not discoverable")
		}
	}
	unlock := configpkg.LockUserConfigEdits()
	path := configpkg.UserConfigPath()
	if path == "" {
		unlock()
		return subagentSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		unlock()
		return subagentSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	resolveModel := func(ref string) (string, error) {
		if ref == "" {
			return "", nil
		}
		provider, ok := cfg.ResolveModel(ref)
		if !ok {
			return "", fmt.Errorf("unknown model")
		}
		return provider.Name + "/" + provider.Model, nil
	}
	normalizeEffort := func(model, effort string) (string, error) {
		if effort == "" || effort == "auto" {
			return "", nil
		}
		if model == "" {
			model = cfg.DefaultModel
		}
		provider, ok := cfg.ResolveModel(model)
		if !ok {
			return "", fmt.Errorf("unknown subagent model")
		}
		return configpkg.NormalizeEffort(provider, effort)
	}
	switch change.Action {
	case "model":
		cfg.Agent.SubagentModel, err = resolveModel(change.Value)
	case "effort":
		cfg.Agent.SubagentEffort, err = normalizeEffort(cfg.Agent.SubagentModel, change.Value)
	case "depth":
		if change.Number != 1 && change.Number != 2 {
			err = fmt.Errorf("subagent depth must be 1 or 2")
		} else {
			cfg.Agent.MaxSubagentDepth = change.Number
		}
	case "concurrency":
		if change.Number < 1 || change.Number > 32 {
			err = fmt.Errorf("subagent concurrency must be 1 to 32")
		} else {
			cfg.Agent.MaxSubagentConcurrency, cfg.Agent.MaxParallelWriters = agent.NormalizeConcurrencyLimits(change.Number, cfg.Agent.MaxParallelWriters)
		}
	case "writers":
		total, _ := agent.NormalizeConcurrencyLimits(cfg.Agent.MaxSubagentConcurrency, cfg.Agent.MaxParallelWriters)
		if change.Number < 1 || change.Number > total {
			err = fmt.Errorf("writer concurrency exceeds total")
		} else {
			cfg.Agent.MaxParallelWriters = change.Number
		}
	case "profile_model":
		var ref string
		ref, err = resolveModel(change.Value)
		if err == nil {
			if cfg.Agent.SubagentModels == nil {
				cfg.Agent.SubagentModels = map[string]string{}
			}
			clearSubagentOverride(cfg.Agent.SubagentModels, change.Name)
			if ref != "" {
				cfg.Agent.SubagentModels[change.Name] = ref
			}
		}
	case "profile_effort":
		model := subagentOverride(cfg.Agent.SubagentModels, change.Name)
		if model == "" {
			model = cfg.Agent.SubagentModel
		}
		var effort string
		effort, err = normalizeEffort(model, change.Value)
		if err == nil {
			if cfg.Agent.SubagentEfforts == nil {
				cfg.Agent.SubagentEfforts = map[string]string{}
			}
			clearSubagentOverride(cfg.Agent.SubagentEfforts, change.Name)
			if effort != "" {
				cfg.Agent.SubagentEfforts[change.Name] = effort
			}
		}
	}
	if err == nil {
		err = cfg.SaveUserSettingsDeltaTo(path, baseline)
	}
	unlock()
	if err != nil {
		return subagentSettingsView{}, err
	}
	return loadSubagentSettings(root)
}
