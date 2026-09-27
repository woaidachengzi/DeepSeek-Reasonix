package main

import (
	"fmt"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type networkSettingsView struct {
	ProtocolVersion  int    `json:"protocolVersion"`
	ProxyMode        string `json:"proxyMode"`
	NoProxy          string `json:"noProxy"`
	ProxyType        string `json:"proxyType"`
	ProxyServer      string `json:"proxyServer"`
	ProxyPort        int    `json:"proxyPort"`
	ProxyUsername    string `json:"proxyUsername"`
	ProxyURLSet      bool   `json:"proxyUrlSet"`
	ProxyPasswordSet bool   `json:"proxyPasswordSet"`
}

type networkSettingsChange struct {
	ProxyMode           string `json:"proxyMode"`
	NoProxy             string `json:"noProxy"`
	ProxyType           string `json:"proxyType"`
	ProxyServer         string `json:"proxyServer"`
	ProxyPort           int    `json:"proxyPort"`
	ProxyUsername       string `json:"proxyUsername"`
	ProxyURLAction      string `json:"proxyUrlAction"`      // keep | replace | clear
	ProxyURL            string `json:"proxyUrl"`            // write-only
	ProxyPasswordAction string `json:"proxyPasswordAction"` // keep | replace | clear
	ProxyPassword       string `json:"proxyPassword"`       // write-only
}

func networkSettingsFromConfig(cfg *configpkg.Config) networkSettingsView {
	return networkSettingsView{
		ProtocolVersion:  desktopbridge.ProtocolVersion,
		ProxyMode:        cfg.NetworkProxyMode(),
		NoProxy:          cfg.Network.NoProxy,
		ProxyType:        cfg.Network.Proxy.Type,
		ProxyServer:      cfg.Network.Proxy.Server,
		ProxyPort:        cfg.Network.Proxy.Port,
		ProxyUsername:    cfg.Network.Proxy.Username,
		ProxyURLSet:      cfg.Network.ProxyURL != "",
		ProxyPasswordSet: cfg.Network.Proxy.Password != "",
	}
}

func loadNetworkSettings() (networkSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return networkSettingsView{}, err
	}
	return networkSettingsFromConfig(cfg), nil
}

func persistNetworkSettings(change networkSettingsChange) (networkSettingsView, error) {
	if change.ProxyMode != "auto" && change.ProxyMode != "env" && change.ProxyMode != "custom" && change.ProxyMode != "off" {
		return networkSettingsView{}, fmt.Errorf("invalid proxy mode")
	}
	if change.ProxyURLAction != "keep" && change.ProxyURLAction != "replace" && change.ProxyURLAction != "clear" {
		return networkSettingsView{}, fmt.Errorf("invalid proxy URL action")
	}
	if change.ProxyPasswordAction != "keep" && change.ProxyPasswordAction != "replace" && change.ProxyPasswordAction != "clear" {
		return networkSettingsView{}, fmt.Errorf("invalid proxy password action")
	}
	if len(change.NoProxy) > 4096 || len(change.ProxyType) > 16 || len(change.ProxyServer) > 2048 || len(change.ProxyUsername) > 1024 || len(change.ProxyURL) > 4096 || len(change.ProxyPassword) > 4096 {
		return networkSettingsView{}, fmt.Errorf("network settings exceed limits")
	}
	if typ := strings.ToLower(strings.TrimSpace(change.ProxyType)); typ != "" && typ != "http" && typ != "https" && typ != "socks5" && typ != "socks5h" {
		return networkSettingsView{}, fmt.Errorf("invalid proxy type")
	}
	if change.ProxyPort < 0 || change.ProxyPort > 65535 {
		return networkSettingsView{}, fmt.Errorf("invalid proxy port")
	}
	if change.ProxyURLAction == "replace" && strings.TrimSpace(change.ProxyURL) == "" {
		return networkSettingsView{}, fmt.Errorf("empty replacement proxy URL")
	}
	if change.ProxyPasswordAction == "replace" && change.ProxyPassword == "" {
		return networkSettingsView{}, fmt.Errorf("empty replacement password")
	}
	for _, value := range []string{change.NoProxy, change.ProxyType, change.ProxyServer, change.ProxyUsername, change.ProxyURL, change.ProxyPassword} {
		if strings.ContainsRune(value, '\x00') {
			return networkSettingsView{}, fmt.Errorf("invalid network setting")
		}
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return networkSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return networkSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	next := cfg.Network
	next.ProxyMode = change.ProxyMode
	next.NoProxy = change.NoProxy
	next.Proxy.Type = change.ProxyType
	next.Proxy.Server = change.ProxyServer
	next.Proxy.Port = change.ProxyPort
	next.Proxy.Username = change.ProxyUsername
	switch change.ProxyURLAction {
	case "replace":
		next.ProxyURL = change.ProxyURL
	case "clear":
		next.ProxyURL = ""
	}
	switch change.ProxyPasswordAction {
	case "replace":
		next.Proxy.Password = change.ProxyPassword
	case "clear":
		next.Proxy.Password = ""
	}
	if err := cfg.SetNetwork(next); err != nil {
		return networkSettingsView{}, err
	}
	if err := cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		return networkSettingsView{}, err
	}
	return networkSettingsFromConfig(cfg), nil
}
