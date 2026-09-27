package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/skill"
)

type previewSkillView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Invocation  string `json:"invocation"`
	Scope       string `json:"scope"`
	SourcePath  string `json:"sourcePath"`
	RunAs       string `json:"runAs"`
	Enabled     bool   `json:"enabled"`
}

type previewSkillSourceView struct {
	Path              string `json:"path"`
	Scope             string `json:"scope"`
	Status            string `json:"status"`
	Enabled           bool   `json:"enabled"`
	Configured        bool   `json:"configured"`
	ConfiguredGlobal  bool   `json:"configuredGlobal"`
	ConfiguredProject bool   `json:"configuredProject"`
}

type skillsSettingsView struct {
	ProtocolVersion         int                      `json:"protocolVersion"`
	AllowImplicitInvocation bool                     `json:"allowImplicitInvocation"`
	Skills                  []previewSkillView       `json:"skills"`
	Sources                 []previewSkillSourceView `json:"sources"`
}

type skillsSettingsChange struct {
	WorkspaceRoot string `json:"workspaceRoot"`
	Scope         string `json:"scope"`  // global | project
	Action        string `json:"action"` // implicit | skill | source | add_source | remove_source
	Enabled       bool   `json:"enabled"`
	Name          string `json:"name"`
	Path          string `json:"path"`
}

func normalizeSkillsWorkspace(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", nil
	}
	if len(root) > 4096 || !filepath.IsAbs(root) {
		return "", fmt.Errorf("invalid workspace root")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("workspace root is not a directory")
	}
	return filepath.Clean(root), nil
}

func loadSkillsSettings(workspaceRoot string) (skillsSettingsView, error) {
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil {
		return skillsSettingsView{}, err
	}
	var cfg *configpkg.Config
	if root == "" {
		cfg, err = configpkg.LoadUserConfigReadOnly()
	} else {
		cfg, err = configpkg.LoadForRootWithoutCredentialsReadOnly(root)
	}
	if err != nil {
		return skillsSettingsView{}, err
	}
	userCfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return skillsSettingsView{}, err
	}
	opts := skill.Options{
		ProjectRoot: root, CustomPaths: cfg.SkillCustomPaths(),
		PluginPaths: cfg.PluginPackageSkillOwners(), PluginAgentPaths: cfg.PluginPackageAgentOwners(),
		MaxDepth: cfg.SkillMaxDepth(), Stderr: io.Discard,
	}
	allSources := skill.New(opts).Roots()
	opts.ExcludedPaths = cfg.SkillExcludedPaths()
	store := skill.New(opts)
	disabled := map[string]bool{}
	for _, name := range cfg.DisabledSkillNames() {
		disabled[configpkg.SkillNameKey(name)] = true
	}
	excluded := map[string]bool{}
	for _, path := range cfg.SkillExcludedPaths() {
		excluded[configpkg.CanonicalSkillPath(path)] = true
	}
	configured := map[string]bool{}
	for _, path := range userCfg.Skills.Paths {
		configured[configpkg.CanonicalSkillPath(path)] = true
	}
	projectConfigured := map[string]bool{}
	if root != "" {
		projectCfg, loadErr := configpkg.LoadForEditWithoutCredentialsReadOnlyStrict(filepath.Join(root, "reasonix.toml"))
		if loadErr != nil {
			return skillsSettingsView{}, loadErr
		}
		for _, path := range projectCfg.Skills.Paths {
			projectConfigured[configpkg.CanonicalSkillPath(path)] = true
		}
	}
	view := skillsSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion, AllowImplicitInvocation: cfg.ImplicitSkillInvocationEnabled(),
		Skills: []previewSkillView{}, Sources: []previewSkillSourceView{},
	}
	for _, source := range allSources {
		key := configpkg.CanonicalSkillPath(source.Dir)
		view.Sources = append(view.Sources, previewSkillSourceView{
			Path: source.Dir, Scope: string(source.Scope), Status: string(source.Status),
			Enabled: !excluded[key], Configured: configured[key] || projectConfigured[key],
			ConfiguredGlobal: configured[key], ConfiguredProject: projectConfigured[key],
		})
	}
	for _, item := range store.List() {
		view.Skills = append(view.Skills, previewSkillView{
			Name: item.Name, Description: item.Description, Invocation: "/" + item.SlashName(),
			Scope: string(item.Scope), SourcePath: item.Path, RunAs: string(item.RunAs),
			Enabled: !disabled[configpkg.SkillNameKey(item.Name)],
		})
	}
	return view, nil
}

func persistSkillsSettings(change skillsSettingsChange) (skillsSettingsView, error) {
	root, err := normalizeSkillsWorkspace(change.WorkspaceRoot)
	if err != nil {
		return skillsSettingsView{}, err
	}
	change.Name = strings.TrimSpace(change.Name)
	change.Path = strings.TrimSpace(change.Path)
	if len(change.Name) > 64 || len(change.Path) > 4096 || strings.ContainsRune(change.Path, '\x00') {
		return skillsSettingsView{}, fmt.Errorf("invalid skill setting")
	}
	if change.Action != "implicit" && change.Action != "skill" && change.Action != "source" && change.Action != "add_source" && change.Action != "remove_source" {
		return skillsSettingsView{}, fmt.Errorf("invalid skill settings action")
	}
	if change.Action == "skill" && configpkg.SkillNameKey(change.Name) == "" {
		return skillsSettingsView{}, fmt.Errorf("invalid skill name")
	}
	if change.Action == "source" || change.Action == "add_source" || change.Action == "remove_source" {
		if change.Path == "" || !filepath.IsAbs(change.Path) {
			return skillsSettingsView{}, fmt.Errorf("invalid skill source path")
		}
	}
	if change.Action == "add_source" {
		info, err := os.Stat(change.Path)
		if err != nil || !info.IsDir() {
			return skillsSettingsView{}, fmt.Errorf("skill source is not a directory")
		}
	}
	if change.Scope == "" {
		change.Scope = "global" // existing Preview clients used global writes
	}
	if change.Scope != "global" && change.Scope != "project" {
		return skillsSettingsView{}, fmt.Errorf("invalid skill settings scope")
	}
	if change.Scope == "project" {
		if root == "" {
			return skillsSettingsView{}, fmt.Errorf("project skill settings require a workspace")
		}
		userCfg, err := configpkg.LoadUserConfigReadOnly()
		if err != nil {
			return skillsSettingsView{}, err
		}
		path := filepath.Join(root, "reasonix.toml")
		err = configpkg.EditProjectConfigFileWithoutCredentials(path, func(cfg *configpkg.Config) error {
			key := skillChangeKey(change.Action)
			if !cfg.ProjectSkillKeyDeclared(key) {
				seedProjectSkillSetting(cfg, userCfg, key)
			}
			// Adding a path also re-enables it. Preserve that intent when an
			// inherited or project exclusion previously hid the directory.
			reenableExcluded := false
			if change.Action == "add_source" {
				exclusions := cfg.Skills.ExcludedPaths
				if !cfg.ProjectSkillKeyDeclared("excluded_paths") {
					exclusions = userCfg.Skills.ExcludedPaths
				}
				for _, excluded := range exclusions {
					if configpkg.CanonicalSkillPath(excluded) == configpkg.CanonicalSkillPath(change.Path) {
						reenableExcluded = true
						break
					}
				}
				if reenableExcluded && !cfg.ProjectSkillKeyDeclared("excluded_paths") {
					seedProjectSkillSetting(cfg, userCfg, "excluded_paths")
				}
			}
			if err := applySkillChange(cfg, change); err != nil {
				return err
			}
			if reenableExcluded {
				if err := cfg.KeepProjectSkillKey("excluded_paths"); err != nil {
					return err
				}
			}
			return cfg.KeepProjectSkillKey(key)
		})
		if err != nil {
			return skillsSettingsView{}, err
		}
		return loadSkillsSettings(root)
	}
	unlock := configpkg.LockUserConfigEdits()
	path := configpkg.UserConfigPath()
	if path == "" {
		unlock()
		return skillsSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		unlock()
		return skillsSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	err = applySkillChange(cfg, change)
	if err == nil {
		err = cfg.SaveUserSettingsDeltaTo(path, baseline)
	}
	unlock()
	if err != nil {
		return skillsSettingsView{}, err
	}
	return loadSkillsSettings(root)
}

func skillChangeKey(action string) string {
	switch action {
	case "implicit":
		return "disable_implicit_invocation"
	case "skill":
		return "disabled_skills"
	case "source":
		return "excluded_paths"
	default:
		return "paths"
	}
}

func seedProjectSkillSetting(cfg, user *configpkg.Config, key string) {
	switch key {
	case "disable_implicit_invocation":
		cfg.Skills.DisableImplicitInvocation = user.Skills.DisableImplicitInvocation
	case "disabled_skills":
		cfg.Skills.DisabledSkills = append([]string(nil), user.Skills.DisabledSkills...)
	case "excluded_paths":
		cfg.Skills.ExcludedPaths = append([]string(nil), user.Skills.ExcludedPaths...)
	case "paths":
		cfg.Skills.Paths = append([]string(nil), user.Skills.Paths...)
	}
}

func applySkillChange(cfg *configpkg.Config, change skillsSettingsChange) error {
	switch change.Action {
	case "implicit":
		cfg.SetSkillImplicitInvocation(change.Enabled)
		return nil
	case "skill":
		return cfg.SetSkillEnabled(change.Name, change.Enabled)
	case "source":
		return cfg.SetSkillPathEnabled(change.Path, change.Enabled)
	case "add_source":
		return cfg.AddSkillPath(change.Path)
	case "remove_source":
		removed, err := cfg.RemoveSkillPath(change.Path)
		if err == nil && !removed {
			return fmt.Errorf("skill source is not a custom path in this scope")
		}
		return err
	}
	return fmt.Errorf("invalid skill settings action")
}
