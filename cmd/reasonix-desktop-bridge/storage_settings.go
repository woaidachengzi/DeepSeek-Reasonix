package main

import (
	"net/http"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/pluginpkg"
)

type previewStorageSettingsView struct {
	ProtocolVersion int    `json:"protocolVersion"`
	ProfilePath     string `json:"profilePath"`
	StatePath       string `json:"statePath"`
	CachePath       string `json:"cachePath"`
	ExtensionsPath  string `json:"extensionsPath"`
}

func (b *bridgeServer) storageSettings(w http.ResponseWriter, _ *http.Request) {
	home := appconfig.ReasonixHomeDir()
	extensions := ""
	if home != "" {
		extensions = pluginpkg.PluginsDir(home)
	}
	writeJSON(w, http.StatusOK, previewStorageSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		ProfilePath:     home,
		StatePath:       appconfig.MemoryUserDir(),
		CachePath:       appconfig.CacheDir(),
		ExtensionsPath:  extensions,
	})
}
