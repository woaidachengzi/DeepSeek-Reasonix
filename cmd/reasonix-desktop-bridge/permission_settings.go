package main

import (
	"fmt"
	"path/filepath"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type permissionProjectOverrides struct {
	Mode  bool `json:"mode"`
	Allow bool `json:"allow"`
	Ask   bool `json:"ask"`
	Deny  bool `json:"deny"`
}

type permissionSettingsView struct {
	ProtocolVersion  int                        `json:"protocolVersion"`
	Scope            string                     `json:"scope"`
	Mode             string                     `json:"mode"`
	Allow            []string                   `json:"allow"`
	Ask              []string                   `json:"ask"`
	Deny             []string                   `json:"deny"`
	ProjectOverrides permissionProjectOverrides `json:"projectOverrides"`
}

type permissionSettingsChange struct {
	Action        string `json:"action"` // mode | add | remove
	Scope         string `json:"scope,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	Mode          string `json:"mode,omitempty"`
	List          string `json:"list,omitempty"`
	Rule          string `json:"rule,omitempty"`
}

func permissionSettingsFromConfig(cfg *configpkg.Config, scope string, overrides permissionProjectOverrides) permissionSettingsView {
	mode := cfg.Permissions.Mode
	if mode != "ask" && mode != "allow" && mode != "deny" {
		mode = "ask"
	}
	copyRules := func(rules []string) []string { return append([]string{}, rules...) }
	return permissionSettingsView{
		ProtocolVersion:  desktopbridge.ProtocolVersion,
		Scope:            scope,
		Mode:             mode,
		Allow:            copyRules(cfg.Permissions.Allow),
		Ask:              copyRules(cfg.Permissions.Ask),
		Deny:             copyRules(cfg.Permissions.Deny),
		ProjectOverrides: overrides,
	}
}

func loadPermissionSettings(workspaceRoot, scope string) (permissionSettingsView, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	if scope != "global" && scope != "project" {
		return permissionSettingsView{}, fmt.Errorf("invalid permission settings scope")
	}
	if scope == "project" {
		root, err := normalizeSkillsWorkspace(workspaceRoot)
		if err != nil || root == "" {
			return permissionSettingsView{}, fmt.Errorf("project permission settings require a valid workspace")
		}
		cfg, err := configpkg.LoadForRootWithoutCredentialsReadOnly(root)
		if err != nil {
			return permissionSettingsView{}, err
		}
		projectCfg, err := configpkg.LoadForEditWithoutCredentialsReadOnlyStrict(filepath.Join(root, "reasonix.toml"))
		if err != nil {
			return permissionSettingsView{}, err
		}
		return permissionSettingsFromConfig(cfg, scope, permissionProjectOverrides{
			Mode:  projectCfg.ProjectPermissionKeyDeclared("mode"),
			Allow: projectCfg.ProjectPermissionKeyDeclared("allow"),
			Ask:   projectCfg.ProjectPermissionKeyDeclared("ask"),
			Deny:  projectCfg.ProjectPermissionKeyDeclared("deny"),
		}), nil
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return permissionSettingsView{}, err
	}
	return permissionSettingsFromConfig(cfg, scope, permissionProjectOverrides{}), nil
}

func persistPermissionChange(change permissionSettingsChange) (permissionSettingsView, error) {
	change.Action = strings.TrimSpace(change.Action)
	change.List = strings.TrimSpace(change.List)
	change.Rule = strings.TrimSpace(change.Rule)
	if change.Action == "mode" {
		if change.Mode != "ask" && change.Mode != "allow" && change.Mode != "deny" {
			return permissionSettingsView{}, fmt.Errorf("invalid permission fallback mode")
		}
	} else if change.Action != "add" && change.Action != "remove" {
		return permissionSettingsView{}, fmt.Errorf("invalid permission change")
	}
	if change.Action != "mode" && (change.List != "allow" && change.List != "ask" && change.List != "deny" || change.Rule == "" || len(change.Rule) > 2048) {
		return permissionSettingsView{}, fmt.Errorf("invalid permission rule request")
	}
	change.Scope = strings.TrimSpace(change.Scope)
	if change.Scope == "" {
		change.Scope = "global"
	}
	if change.Scope != "global" && change.Scope != "project" {
		return permissionSettingsView{}, fmt.Errorf("invalid permission settings scope")
	}
	if change.Scope == "project" {
		root, err := normalizeSkillsWorkspace(change.WorkspaceRoot)
		if err != nil || root == "" {
			return permissionSettingsView{}, fmt.Errorf("project permission settings require a valid workspace")
		}
		userCfg, err := configpkg.LoadUserConfigReadOnly()
		if err != nil {
			return permissionSettingsView{}, err
		}
		path := filepath.Join(root, "reasonix.toml")
		err = configpkg.EditProjectConfigFileWithoutCredentials(path, func(cfg *configpkg.Config) error {
			key := change.List
			if change.Action == "mode" {
				key = "mode"
			}
			if !cfg.ProjectPermissionKeyDeclared(key) {
				switch key {
				case "mode":
					cfg.Permissions.Mode = userCfg.Permissions.Mode
				case "allow":
					cfg.Permissions.Allow = append([]string{}, userCfg.Permissions.Allow...)
				case "ask":
					cfg.Permissions.Ask = append([]string{}, userCfg.Permissions.Ask...)
				case "deny":
					cfg.Permissions.Deny = append([]string{}, userCfg.Permissions.Deny...)
				}
			}
			var err error
			switch change.Action {
			case "mode":
				err = cfg.SetPermissionMode(change.Mode)
			case "add":
				err = cfg.AddPermissionRule(change.List, change.Rule)
			case "remove":
				_, err = cfg.RemovePermissionRule(change.List, change.Rule)
			}
			if err != nil {
				return err
			}
			return cfg.KeepProjectPermissionKey(key)
		})
		if err != nil {
			return permissionSettingsView{}, err
		}
		return loadPermissionSettings(root, change.Scope)
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return permissionSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return permissionSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	switch change.Action {
	case "mode":
		err = cfg.SetPermissionMode(change.Mode)
	case "add":
		err = cfg.AddPermissionRule(change.List, change.Rule)
	case "remove":
		_, err = cfg.RemovePermissionRule(change.List, change.Rule)
	}
	if err != nil {
		return permissionSettingsView{}, err
	}
	if err := cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		return permissionSettingsView{}, err
	}
	return permissionSettingsFromConfig(cfg, change.Scope, permissionProjectOverrides{}), nil
}
