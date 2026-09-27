package main

import (
	"fmt"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type permissionSettingsView struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Mode            string   `json:"mode"`
	Allow           []string `json:"allow"`
	Ask             []string `json:"ask"`
	Deny            []string `json:"deny"`
}

type permissionSettingsChange struct {
	Action string `json:"action"` // mode | add | remove
	Mode   string `json:"mode,omitempty"`
	List   string `json:"list,omitempty"`
	Rule   string `json:"rule,omitempty"`
}

func permissionSettingsFromConfig(cfg *configpkg.Config) permissionSettingsView {
	mode := cfg.Permissions.Mode
	if mode != "ask" && mode != "allow" && mode != "deny" {
		mode = "ask"
	}
	copyRules := func(rules []string) []string { return append([]string{}, rules...) }
	return permissionSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Mode:            mode,
		Allow:           copyRules(cfg.Permissions.Allow),
		Ask:             copyRules(cfg.Permissions.Ask),
		Deny:            copyRules(cfg.Permissions.Deny),
	}
}

func loadPermissionSettings() (permissionSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return permissionSettingsView{}, err
	}
	return permissionSettingsFromConfig(cfg), nil
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
	return permissionSettingsFromConfig(cfg), nil
}
