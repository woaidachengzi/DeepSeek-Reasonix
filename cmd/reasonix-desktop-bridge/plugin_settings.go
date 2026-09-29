package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/hook"
	"reasonix/internal/pluginpkg"
)

type previewPluginView struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Version      string               `json:"version"`
	Source       string               `json:"source"`
	UpdateSource string               `json:"updateSource,omitempty"`
	Root         string               `json:"root"`
	ManifestKind string               `json:"manifestKind"`
	Enabled      bool                 `json:"enabled"`
	Linked       bool                 `json:"linked"`
	Status       string               `json:"status"`
	Issue        string               `json:"issue"`
	WarningCount int                  `json:"warningCount"`
	Skills       int                  `json:"skills"`
	Agents       int                  `json:"agents"`
	Commands     int                  `json:"commands"`
	Hooks        int                  `json:"hooks"`
	HookDetails  []previewPluginHook  `json:"hookDetails,omitempty"`
	MCPServers   int                  `json:"mcpServers"`
	Themes       []previewPluginTheme `json:"themes"`
	Runtime      bool                 `json:"runtime"`
	Revision     string               `json:"revision"`
}

// previewPluginHook exposes the package's hook metadata without returning
// executable command strings to the WebView.
type previewPluginHook struct {
	Event       string `json:"event"`
	Match       string `json:"match,omitempty"`
	ContextFile string `json:"contextFile,omitempty"`
	Description string `json:"description,omitempty"`
}

type previewPluginTheme struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type previewPluginSettings struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Plugins         []previewPluginView `json:"plugins"`
}

type previewPluginChange struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Enabled  bool   `json:"enabled"`
}

type previewPluginDoctorRequest struct {
	Name string `json:"name"`
}

type previewPluginDoctorView struct {
	ProtocolVersion     int                            `json:"protocolVersion"`
	Name                string                         `json:"name"`
	Compatibility       string                         `json:"compatibility"`
	MappedCapabilities  []string                       `json:"mappedCapabilities"`
	SkippedCapabilities []pluginpkg.CompatibilityIssue `json:"skippedCapabilities"`
	Warnings            []string                       `json:"warnings"`
	Error               string                         `json:"error"`
}

func diagnosePreviewPlugin(name string) (previewPluginDoctorView, error) {
	name = strings.TrimSpace(name)
	if !pluginpkg.IsValidName(name) {
		return previewPluginDoctorView{}, fmt.Errorf("invalid plugin name")
	}
	home := appconfig.ReasonixHomeDir()
	installed, found, err := pluginpkg.FindInstalled(home, name)
	if err != nil {
		return previewPluginDoctorView{}, err
	}
	if !found {
		return previewPluginDoctorView{}, os.ErrNotExist
	}
	root := pluginpkg.ResolveRoot(home, installed.Root)
	if _, err := os.Stat(root); err != nil {
		return previewPluginDoctorView{}, err
	}
	pkg, warnings, err := pluginpkg.ParseDir(root)
	if err != nil {
		return previewPluginDoctorView{}, err
	}
	view := previewPluginDoctorView{
		ProtocolVersion:     desktopbridge.ProtocolVersion,
		Name:                name,
		Compatibility:       pkg.Compatibility.Status,
		MappedCapabilities:  append([]string{}, pkg.Compatibility.Mapped...),
		SkippedCapabilities: append([]pluginpkg.CompatibilityIssue{}, pkg.Compatibility.Skipped...),
		Warnings:            append([]string{}, warnings...),
	}
	// Match the stable PluginDoctor check, but only inspect the trusted Preview
	// user config. Diagnostics never execute plugin hooks or load project config.
	if cfg, configErr := appconfig.LoadUserConfigReadOnly(); configErr == nil && cfg != nil {
		options := hook.RuntimeOptionsForShell(cfg.Tools.Shell.Prefer, cfg.Tools.Shell.Path)
		for _, issue := range hook.CheckPackageRuntime(pkg, options) {
			view.Warnings = append(view.Warnings, fmt.Sprintf("%s hook is unavailable: %v", issue.Event, issue.Err))
		}
	}
	return view, nil
}

func loadPreviewPluginSettings() (previewPluginSettings, error) {
	home := appconfig.ReasonixHomeDir()
	state, err := pluginpkg.LoadState(home)
	if err != nil {
		return previewPluginSettings{}, err
	}
	view := previewPluginSettings{ProtocolVersion: desktopbridge.ProtocolVersion, Plugins: []previewPluginView{}}
	for _, installed := range state.Plugins {
		item := previewPluginView{
			Name: installed.Name, Description: installed.Description, Version: installed.Version,
			Source: pluginSourceKind(installed.Source), Root: pluginpkg.ResolveRoot(home, installed.Root),
			ManifestKind: installed.ManifestKind, Enabled: installed.Enabled,
			Revision: pluginpkg.InstalledRevision(installed),
		}
		item.Linked = filepath.Clean(item.Root) != filepath.Clean(pluginpkg.InstallRoot(home, installed.Name))
		if !item.Linked && validPreviewPluginSource(strings.TrimSpace(installed.Source)) {
			item.UpdateSource = strings.TrimSpace(installed.Source)
		}
		pkg, warnings, parseErr := pluginpkg.ParseDir(item.Root)
		if parseErr != nil {
			item.Status = "invalid"
			item.Issue = "插件文件缺失或格式不兼容"
		} else {
			item.Status = "ready"
			item.ManifestKind = pkg.ManifestKind
			summary := pkg.CapabilitySummary()
			item.Skills, item.Agents, item.Commands = summary.Skills, summary.Agents, summary.Commands
			item.Hooks, item.MCPServers, item.Runtime = summary.Hooks, summary.MCPServers, summary.Runtime
			for _, hook := range pkg.Inventory().Hooks {
				item.HookDetails = append(item.HookDetails, previewPluginHook{
					Event: hook.Event, Match: hook.Match, ContextFile: hook.ContextFile, Description: hook.Description,
				})
			}
			item.WarningCount = len(warnings)
			if item.Enabled {
				for _, theme := range pkg.Inventory().Themes {
					item.Themes = append(item.Themes, previewPluginTheme{Name: theme.Name, Path: theme.Path})
				}
			}
		}
		if item.Themes == nil {
			item.Themes = []previewPluginTheme{}
		}
		view.Plugins = append(view.Plugins, item)
	}
	return view, nil
}

func pluginSourceKind(source string) string {
	source = strings.TrimSpace(source)
	switch {
	case source == "":
		return "unknown"
	case filepath.IsAbs(source), strings.HasPrefix(source, "file:"):
		return "local"
	case strings.HasPrefix(source, "https:"), strings.HasPrefix(source, "http:"), strings.HasPrefix(source, "git@"):
		return "remote"
	default:
		return "package"
	}
}

func (b *bridgeServer) pluginSettings(w http.ResponseWriter, _ *http.Request) {
	view, err := loadPreviewPluginSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "unable to read Preview plugins")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) pluginDoctor(w http.ResponseWriter, r *http.Request) {
	var request previewPluginDoctorRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin diagnostic request")
		return
	}
	view, err := diagnosePreviewPlugin(request.Name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeProtocolError(w, http.StatusNotFound, "plugin_missing", "plugin is not installed or its files are missing")
			return
		}
		writeProtocolError(w, http.StatusBadRequest, "plugin_invalid", "plugin diagnostics could not parse this package")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changePluginSettings(w http.ResponseWriter, r *http.Request) {
	var change previewPluginChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin activation request")
		return
	}
	change.Name = strings.TrimSpace(change.Name)
	if !pluginpkg.IsValidName(change.Name) || len(change.Revision) != 64 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin activation request")
		return
	}
	if change.Enabled {
		current, found, err := pluginpkg.FindInstalled(appconfig.ReasonixHomeDir(), change.Name)
		if err != nil || !found {
			writeProtocolError(w, http.StatusConflict, "plugin_changed", "plugin registration changed; refresh and retry")
			return
		}
		if pluginpkg.InstalledRevision(current) != change.Revision {
			writeProtocolError(w, http.StatusConflict, "plugin_changed", "plugin registration changed; refresh and retry")
			return
		}
		if _, _, err := pluginpkg.ParseDir(pluginpkg.ResolveRoot(appconfig.ReasonixHomeDir(), current.Root)); err != nil {
			writeProtocolError(w, http.StatusBadRequest, "invalid_plugin", "plugin files are invalid; repair them before enabling")
			return
		}
	}
	if err := pluginpkg.SetEnabledIfRevision(appconfig.ReasonixHomeDir(), strings.TrimSpace(change.Name), change.Revision, change.Enabled); err != nil {
		writeProtocolError(w, http.StatusConflict, "plugin_changed", "plugin registration changed; refresh and retry")
		return
	}
	view, err := loadPreviewPluginSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "plugin changed but the updated list could not be read")
		return
	}
	writeJSON(w, http.StatusOK, view)
}
