package main

import (
	"fmt"
	"runtime"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type sandboxSettingsView struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Bash            string   `json:"bash"`
	Network         bool     `json:"network"`
	WorkspaceRoot   string   `json:"workspaceRoot"`
	AllowWrite      []string `json:"allowWrite"`
	Platform        string   `json:"platform"`
}

type sandboxSettingsChange struct {
	Bash          string   `json:"bash"`
	Network       bool     `json:"network"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	AllowWrite    []string `json:"allowWrite"`
}

func sandboxSettingsFromConfig(cfg *configpkg.Config) sandboxSettingsView {
	return sandboxSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Bash:            cfg.BashMode(),
		Network:         cfg.Sandbox.Network,
		WorkspaceRoot:   cfg.Sandbox.WorkspaceRoot,
		AllowWrite:      append([]string{}, cfg.Sandbox.AllowWrite...),
		Platform:        runtime.GOOS,
	}
}

func loadSandboxSettings() (sandboxSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return sandboxSettingsView{}, err
	}
	return sandboxSettingsFromConfig(cfg), nil
}

func persistSandboxSettings(change sandboxSettingsChange) (sandboxSettingsView, error) {
	if change.Bash != "enforce" && change.Bash != "off" {
		return sandboxSettingsView{}, fmt.Errorf("invalid bash sandbox mode")
	}
	if runtime.GOOS == "windows" && change.Bash != "off" {
		return sandboxSettingsView{}, fmt.Errorf("bash sandbox unavailable on Windows")
	}
	change.WorkspaceRoot = strings.TrimSpace(change.WorkspaceRoot)
	if len(change.WorkspaceRoot) > 4096 || len(change.AllowWrite) > 64 {
		return sandboxSettingsView{}, fmt.Errorf("sandbox settings exceed limits")
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(change.AllowWrite))
	for _, raw := range change.AllowWrite {
		path := strings.TrimSpace(raw)
		if path == "" || len(path) > 4096 || strings.ContainsRune(path, '\x00') {
			return sandboxSettingsView{}, fmt.Errorf("invalid writable path")
		}
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if strings.ContainsRune(change.WorkspaceRoot, '\x00') {
		return sandboxSettingsView{}, fmt.Errorf("invalid workspace path")
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return sandboxSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return sandboxSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	cfg.Sandbox.Bash = change.Bash
	cfg.Sandbox.Network = change.Network
	cfg.Sandbox.WorkspaceRoot = change.WorkspaceRoot
	cfg.Sandbox.AllowWrite = paths
	if err := cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		return sandboxSettingsView{}, err
	}
	return sandboxSettingsFromConfig(cfg), nil
}
