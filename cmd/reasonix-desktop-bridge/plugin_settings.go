package main

import (
	"net/http"
	"path/filepath"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/pluginpkg"
)

type previewPluginView struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Version      string               `json:"version"`
	Source       string               `json:"source"`
	Root         string               `json:"root"`
	ManifestKind string               `json:"manifestKind"`
	Enabled      bool                 `json:"enabled"`
	Status       string               `json:"status"`
	Issue        string               `json:"issue"`
	WarningCount int                  `json:"warningCount"`
	Skills       int                  `json:"skills"`
	Agents       int                  `json:"agents"`
	Commands     int                  `json:"commands"`
	Hooks        int                  `json:"hooks"`
	MCPServers   int                  `json:"mcpServers"`
	Themes       []previewPluginTheme `json:"themes"`
	Runtime      bool                 `json:"runtime"`
	Revision     string               `json:"revision"`
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
