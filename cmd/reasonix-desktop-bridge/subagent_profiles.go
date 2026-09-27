package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"reasonix/internal/command"
	configpkg "reasonix/internal/config"
	"reasonix/internal/skill"
)

// Profile edits are serialized inside the Preview sidecar. Revisions also
// reject a stale settings form after an external editor changes a file.
var subagentProfileEditMu sync.Mutex

type previewSubagentProfileInput struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"systemPrompt"`
	Color        string   `json:"color"`
	Model        string   `json:"model"`
	Effort       string   `json:"effort"`
	AllowedTools []string `json:"allowedTools"`
	ReadOnly     bool     `json:"readOnly"`
}

func subagentProfileRevision(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func editableProfileScope(raw, root string) (skill.Scope, error) {
	switch strings.TrimSpace(raw) {
	case "global", "":
		return skill.ScopeGlobal, nil
	case "project":
		if root == "" {
			return "", fmt.Errorf("project profile requires a workspace")
		}
		return skill.ScopeProject, nil
	default:
		return "", fmt.Errorf("unsupported subagent profile scope")
	}
}

func safeProfileCreateRoot(root string, scope skill.Scope, name string) error {
	var base string
	var parts []string
	if scope == skill.ScopeProject {
		base, parts = root, []string{".reasonix", skill.SkillsDirname, name}
	} else {
		base, parts = configpkg.ReasonixHomeDir(), []string{skill.SkillsDirname, name}
	}
	if base == "" {
		return fmt.Errorf("profile root is unavailable")
	}
	current := base
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile path uses a symbolic link")
		}
		if !info.IsDir() {
			return fmt.Errorf("profile path is not a directory")
		}
	}
	return nil
}

func safeProfileDeletePath(path string) error {
	if filepath.Base(path) != skill.SkillFile {
		return fmt.Errorf("profile file layout cannot be deleted here")
	}
	dir := filepath.Dir(path)
	for _, candidate := range []string{filepath.Dir(filepath.Dir(dir)), filepath.Dir(dir), dir, path} {
		info, err := os.Lstat(candidate)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile path uses a symbolic link")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0].Name() != skill.SkillFile {
		return fmt.Errorf("profile directory contains other files; edit it as a skill folder")
	}
	return nil
}

func validateProfileInput(input *previewSubagentProfileInput, cfg *configpkg.Config) (skill.SkillFileOptions, error) {
	if input == nil {
		return skill.SkillFileOptions{}, fmt.Errorf("profile content is required")
	}
	name, desc, prompt := strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.SystemPrompt)
	if !skill.IsValidName(name) || desc == "" || prompt == "" || len(desc) > 512 || len(prompt) > 64<<10 || len(input.Color) > 64 || len(input.AllowedTools) > 128 {
		return skill.SkillFileOptions{}, fmt.Errorf("invalid profile name, description or prompt")
	}
	for _, value := range []string{desc, input.Color} {
		for _, r := range value {
			if unicode.IsControl(r) {
				return skill.SkillFileOptions{}, fmt.Errorf("profile metadata contains a control character")
			}
		}
	}
	model := strings.TrimSpace(input.Model)
	if model != "" {
		entry, ok := cfg.ResolveModel(model)
		if !ok {
			return skill.SkillFileOptions{}, fmt.Errorf("unknown profile model")
		}
		model = entry.Name + "/" + entry.Model
	}
	effort := strings.TrimSpace(input.Effort)
	if effort != "" {
		ref := model
		if ref == "" {
			ref = cfg.Agent.SubagentModel
		}
		if ref == "" {
			ref = cfg.DefaultModel
		}
		entry, ok := cfg.ResolveModel(ref)
		if !ok {
			return skill.SkillFileOptions{}, fmt.Errorf("unknown profile effort model")
		}
		var err error
		effort, err = configpkg.NormalizeEffort(entry, effort)
		if err != nil {
			return skill.SkillFileOptions{}, err
		}
	}
	tools := make([]string, 0, len(input.AllowedTools))
	seen := map[string]bool{}
	for _, raw := range input.AllowedTools {
		tool := strings.TrimSpace(raw)
		if tool == "" || len(tool) > 256 {
			return skill.SkillFileOptions{}, fmt.Errorf("invalid allowed tool")
		}
		for _, r := range tool {
			if unicode.IsControl(r) {
				return skill.SkillFileOptions{}, fmt.Errorf("invalid allowed tool")
			}
		}
		if !seen[tool] {
			tools = append(tools, tool)
			seen[tool] = true
		}
	}
	return skill.SkillFileOptions{
		Name: name, Description: desc, Body: prompt, RunAs: skill.RunSubagent,
		Model: model, Effort: effort, AllowedTools: tools, ReadOnly: input.ReadOnly,
		Color: strings.TrimSpace(input.Color), Invocation: "manual",
	}, nil
}

func persistSubagentProfile(change subagentSettingsChange, root string) (subagentSettingsView, error) {
	scope, err := editableProfileScope(change.Scope, root)
	if err != nil {
		return subagentSettingsView{}, err
	}
	subagentProfileEditMu.Lock()
	defer subagentProfileEditMu.Unlock()
	store, cfg, err := previewSubagentStore(root)
	if err != nil {
		return subagentSettingsView{}, err
	}
	if change.Action == "create_profile" {
		var canonicalRoot string
		if scope == skill.ScopeProject {
			canonicalRoot = filepath.Join(root, ".reasonix", skill.SkillsDirname)
		} else {
			canonicalRoot = filepath.Join(configpkg.ReasonixHomeDir(), skill.SkillsDirname)
		}
		discovered := false
		for _, item := range store.Roots() {
			if item.Scope == scope && filepath.Clean(item.Dir) == filepath.Clean(canonicalRoot) {
				discovered = true
				break
			}
		}
		if !discovered {
			return subagentSettingsView{}, fmt.Errorf("selected profile scope is excluded from skill discovery")
		}
	}
	name := change.Name
	if change.Action == "create_profile" {
		if change.Profile == nil {
			return subagentSettingsView{}, fmt.Errorf("profile content is required")
		}
		name = strings.TrimSpace(change.Profile.Name)
		occupied := make([]string, 0)
		for _, item := range store.List() {
			occupied = append(occupied, item.Name, item.SlashName())
		}
		commands, err := command.Load(configpkg.CommandDirsForRoot(root)...)
		if err != nil {
			return subagentSettingsView{}, err
		}
		for _, item := range commands {
			occupied = append(occupied, item.Name)
		}
		if err := skill.ValidateSubagentProfileName(name, occupied); err != nil {
			return subagentSettingsView{}, err
		}
		options, err := validateProfileInput(change.Profile, cfg)
		if err != nil {
			return subagentSettingsView{}, err
		}
		if err := safeProfileCreateRoot(root, scope, name); err != nil {
			return subagentSettingsView{}, err
		}
		_, err = store.CreateWithContent(name, scope, skill.RenderSkillFile(options))
		if err != nil {
			return subagentSettingsView{}, err
		}
	} else {
		if !skill.IsValidName(name) || len(change.Revision) != 64 {
			return subagentSettingsView{}, fmt.Errorf("invalid profile identity or revision")
		}
		item, ok := store.Read(name)
		if !ok || item.Scope != scope {
			return subagentSettingsView{}, fmt.Errorf("profile was not found in the selected scope")
		}
		if err := skill.ValidateEditableSubagentProfile(item); err != nil {
			return subagentSettingsView{}, err
		}
		current, err := subagentProfileRevision(item.Path)
		if err != nil {
			return subagentSettingsView{}, err
		}
		if current != change.Revision {
			return subagentSettingsView{}, fmt.Errorf("profile changed on disk; reload before editing")
		}
		switch change.Action {
		case "update_profile":
			if change.Profile == nil || strings.TrimSpace(change.Profile.Name) != name {
				return subagentSettingsView{}, fmt.Errorf("profile name cannot be changed")
			}
			options, err := validateProfileInput(change.Profile, cfg)
			if err != nil {
				return subagentSettingsView{}, err
			}
			err = store.UpdateContent(name, scope, skill.RenderSkillFile(options))
			if err != nil {
				return subagentSettingsView{}, err
			}
		case "delete_profile":
			if err := safeProfileDeletePath(item.Path); err != nil {
				return subagentSettingsView{}, err
			}
			if err := store.Delete(name, scope); err != nil {
				return subagentSettingsView{}, err
			}
		default:
			return subagentSettingsView{}, fmt.Errorf("invalid profile action")
		}
	}
	return loadSubagentSettings(root)
}
