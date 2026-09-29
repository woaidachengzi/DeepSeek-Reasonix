package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sandbox"
)

type sandboxSettingsView struct {
	ProtocolVersion     int                          `json:"protocolVersion"`
	Bash                string                       `json:"bash"`
	Network             bool                         `json:"network"`
	WorkspaceRoot       string                       `json:"workspaceRoot"`
	AllowWrite          []string                     `json:"allowWrite"`
	Platform            string                       `json:"platform"`
	Shell               string                       `json:"shell"`
	ResolvedShell       string                       `json:"resolvedShell"`
	EffectiveShell      string                       `json:"effectiveShell,omitempty"`
	ShellReloadRequired bool                         `json:"shellReloadRequired"`
	ShellCapabilities   []sandbox.ShellCapability    `json:"shellCapabilities"`
	GitCapability       sandbox.ExecutableCapability `json:"gitCapability"`
	EffectiveWriteRoots []string                     `json:"effectiveWriteRoots"`
	EffectiveRootsError string                       `json:"effectiveRootsError,omitempty"`
}

type sandboxSettingsChange struct {
	Bash          string   `json:"bash"`
	Network       bool     `json:"network"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	AllowWrite    []string `json:"allowWrite"`
	Shell         *string  `json:"shell,omitempty"`
}

func sandboxSettingsFromConfig(cfg *configpkg.Config) sandboxSettingsView {
	prefer := strings.TrimSpace(cfg.Tools.Shell.Prefer)
	if prefer == "" {
		prefer = "auto"
	}
	resolved := sandbox.ResolveShell(prefer, cfg.Tools.Shell.Path, nil)
	resolvedName := resolved.Kind.String()
	if resolved.Path == "" {
		resolvedName = ""
	}
	if resolved.Kind == sandbox.ShellPowerShell && strings.HasPrefix(strings.ToLower(filepath.Base(resolved.Path)), "pwsh") {
		resolvedName = "pwsh"
	}
	return sandboxSettingsView{
		ProtocolVersion:     desktopbridge.ProtocolVersion,
		Bash:                cfg.BashMode(),
		Network:             cfg.Sandbox.Network,
		WorkspaceRoot:       cfg.Sandbox.WorkspaceRoot,
		AllowWrite:          append([]string{}, cfg.Sandbox.AllowWrite...),
		Platform:            runtime.GOOS,
		Shell:               prefer,
		ResolvedShell:       resolvedName,
		ShellCapabilities:   sandbox.ShellCapabilitiesForConfig(prefer, cfg.Tools.Shell.Path),
		GitCapability:       sandbox.GitCapabilityForConfig(prefer, cfg.Tools.Shell.Path),
		EffectiveWriteRoots: []string{},
	}
}

func loadSandboxSettings(workspaceRoot string) (sandboxSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return sandboxSettingsView{}, err
	}
	view := sandboxSettingsFromConfig(cfg)
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil {
		view.EffectiveRootsError = err.Error()
		return view, nil
	}
	if root == "" {
		return view, nil
	}
	effective, err := configpkg.LoadForRootWithoutCredentialsReadOnly(root)
	if err != nil {
		view.EffectiveRootsError = err.Error()
		return view, nil
	}
	view.EffectiveWriteRoots = effective.WriteRootsForRoot(root)
	return view, nil
}

func persistSandboxSettings(change sandboxSettingsChange) (sandboxSettingsView, error) {
	if change.Shell != nil {
		prefer := strings.ToLower(strings.TrimSpace(*change.Shell))
		switch prefer {
		case "", "auto", "bash", "powershell", "pwsh":
			if prefer == "" {
				prefer = "auto"
			}
			change.Shell = &prefer
		default:
			return sandboxSettingsView{}, fmt.Errorf("invalid shell preference")
		}
	}
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
	if change.Shell != nil {
		cfg.Tools.Shell.Prefer = *change.Shell
	}
	if err := cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		return sandboxSettingsView{}, err
	}
	return sandboxSettingsFromConfig(cfg), nil
}
