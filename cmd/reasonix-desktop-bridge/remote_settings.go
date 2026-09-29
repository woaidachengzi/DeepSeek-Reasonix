package main

import (
	"fmt"
	"net/http"
	"strings"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote"
)

type remoteSettingsView struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	ConfigPath      string               `json:"configPath"`
	SSHConfigPath   string               `json:"sshConfigPath"`
	Hosts           []remoteSettingsHost `json:"hosts"`
}

type remoteSettingsHost struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	IdentityFile   string `json:"identityFile"`
	ProxyJump      string `json:"proxyJump"`
	Workspace      string `json:"workspace"`
	ServeInstall   string `json:"serveInstall"`
	CredentialMode string `json:"credentialMode"`
	UseSSHConfig   bool   `json:"useSSHConfig"`
	PasswordSet    bool   `json:"passwordSet"`
	PassphraseSet  bool   `json:"passphraseSet"`
}

type remoteSettingsHostInput struct {
	Name             string `json:"name"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	User             string `json:"user"`
	IdentityFile     string `json:"identityFile"`
	ProxyJump        string `json:"proxyJump"`
	Workspace        string `json:"workspace"`
	ServeInstall     string `json:"serveInstall"`
	CredentialMode   string `json:"credentialMode"`
	UseSSHConfig     bool   `json:"useSSHConfig"`
	PasswordAction   string `json:"passwordAction,omitempty"`
	Password         string `json:"password,omitempty"`
	PassphraseAction string `json:"passphraseAction,omitempty"`
	Passphrase       string `json:"passphrase,omitempty"`
}

type remoteSettingsChange struct {
	Action string                  `json:"action"`
	Name   string                  `json:"name,omitempty"`
	Host   remoteSettingsHostInput `json:"host,omitempty"`
}

type remoteSSHConfigAlias struct {
	Alias string `json:"alias"`
}

type remoteSSHConfigScanView struct {
	ProtocolVersion int                    `json:"protocolVersion"`
	ConfigPath      string                 `json:"configPath"`
	Aliases         []remoteSSHConfigAlias `json:"aliases"`
}

func remoteHostView(entry configpkg.RemoteHostEntry) remoteSettingsHost {
	return remoteSettingsHost{
		Name: entry.Name, Host: entry.Host, Port: entry.Port, User: entry.User,
		IdentityFile: entry.IdentityFile, ProxyJump: entry.ProxyJump,
		Workspace: entry.Workspace, ServeInstall: entry.ServeInstallMode(),
		CredentialMode: entry.CredentialMode, UseSSHConfig: entry.UseSSHConfig,
		PasswordSet:   configpkg.ResolveCredential(entry.PasswordEnv).Set,
		PassphraseSet: configpkg.ResolveCredential(entry.PassphraseEnv).Set,
	}
}

func loadRemoteSettings() (remoteSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return remoteSettingsView{}, err
	}
	sshConfig, err := remote.LoadUserSSHConfig()
	if err != nil {
		return remoteSettingsView{}, err
	}
	view := remoteSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		ConfigPath:      configpkg.UserConfigPath(),
		SSHConfigPath:   sshConfig.Path(),
		Hosts:           make([]remoteSettingsHost, 0, len(cfg.Remote.Hosts)),
	}
	for _, host := range cfg.Remote.Hosts {
		view.Hosts = append(view.Hosts, remoteHostView(host))
	}
	return view, nil
}

func scanRemoteSSHConfig() (remoteSSHConfigScanView, error) {
	source, err := remote.LoadUserSSHConfig()
	if err != nil {
		return remoteSSHConfigScanView{}, err
	}
	view := remoteSSHConfigScanView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		ConfigPath:      source.Path(),
		Aliases:         make([]remoteSSHConfigAlias, 0),
	}
	for _, alias := range source.Aliases() {
		view.Aliases = append(view.Aliases, remoteSSHConfigAlias{Alias: alias.Alias})
	}
	return view, nil
}

func changeRemoteSettings(change remoteSettingsChange) (remoteSettingsView, error) {
	change.Action = strings.TrimSpace(change.Action)
	change.Name = strings.TrimSpace(change.Name)
	if change.Action != "upsert" && change.Action != "remove" {
		return remoteSettingsView{}, fmt.Errorf("invalid remote settings action")
	}
	if change.Action == "remove" && change.Name == "" {
		return remoteSettingsView{}, fmt.Errorf("remote host name is required")
	}
	if change.Action == "upsert" {
		change.Host.Name = strings.TrimSpace(change.Host.Name)
		change.Host.Host = strings.TrimSpace(change.Host.Host)
		change.Host.User = strings.TrimSpace(change.Host.User)
		change.Host.IdentityFile = strings.TrimSpace(change.Host.IdentityFile)
		change.Host.ProxyJump = strings.TrimSpace(change.Host.ProxyJump)
		change.Host.Workspace = strings.TrimSpace(change.Host.Workspace)
		change.Host.ServeInstall = strings.TrimSpace(change.Host.ServeInstall)
		change.Host.CredentialMode = strings.TrimSpace(change.Host.CredentialMode)
		if len(change.Host.Name) > 64 || len(change.Host.Host) > 2048 || len(change.Host.User) > 256 || len(change.Host.IdentityFile) > 4096 || len(change.Host.ProxyJump) > 2048 || len(change.Host.Workspace) > 4096 {
			return remoteSettingsView{}, fmt.Errorf("remote host setting exceeds its size limit")
		}
		if len(change.Host.Password) > 4096 || len(change.Host.Passphrase) > 4096 || strings.ContainsAny(change.Host.Password, "\r\n") || strings.ContainsAny(change.Host.Passphrase, "\r\n") {
			return remoteSettingsView{}, fmt.Errorf("remote credential exceeds its size limit or contains a newline")
		}
		for _, action := range []string{change.Host.PasswordAction, change.Host.PassphraseAction} {
			if action != "" && action != "keep" && action != "replace" && action != "clear" {
				return remoteSettingsView{}, fmt.Errorf("invalid remote credential action")
			}
		}
		if (change.Host.PasswordAction == "replace" && change.Host.Password == "") || (change.Host.PassphraseAction == "replace" && change.Host.Passphrase == "") {
			return remoteSettingsView{}, fmt.Errorf("replacement remote credentials cannot be empty")
		}
	}
	var credentialRemovals []string
	err := configpkg.EditUserConfigWithCredentialsStrict(func(cfg *configpkg.Config) ([]configpkg.CredentialChange, error) {
		if change.Action == "remove" {
			existing, ok := cfg.RemoteHost(change.Name)
			if !ok {
				return nil, fmt.Errorf("no remote host named %q", change.Name)
			}
			for _, key := range []string{existing.PasswordEnv, existing.PassphraseEnv} {
				if configpkg.IsGeneratedRemoteCredential(change.Name, key) {
					credentialRemovals = append(credentialRemovals, key)
				}
			}
			cfg.RemoveRemoteHost(change.Name)
			return configpkg.UnusedGeneratedRemoteCredentialChanges(cfg, credentialRemovals), nil
		}
		input := change.Host
		entry := configpkg.RemoteHostEntry{
			Name: input.Name, Host: input.Host, Port: input.Port, User: input.User,
			IdentityFile: input.IdentityFile, ProxyJump: input.ProxyJump,
			Workspace: input.Workspace, ServeInstall: input.ServeInstall,
			CredentialMode: input.CredentialMode, UseSSHConfig: input.UseSSHConfig,
		}
		if existing, ok := cfg.RemoteHost(entry.Name); ok {
			entry.PasswordEnv = existing.PasswordEnv
			entry.PassphraseEnv = existing.PassphraseEnv
			entry.Forwards = append([]configpkg.RemoteForwardEntry(nil), existing.Forwards...)
		}
		credentialChanges := make([]configpkg.CredentialChange, 0, 2)
		credentialRemovals := make([]string, 0, 2)
		applyCredential := func(action, secret, current string, generatedName func(string) string, set func(string)) {
			switch action {
			case "clear":
				if configpkg.IsGeneratedRemoteCredential(entry.Name, current) {
					credentialRemovals = append(credentialRemovals, current)
				}
				set("")
			case "replace":
				key := generatedName(entry.Name)
				set(key)
				credentialChanges = append(credentialChanges, configpkg.CredentialChange{Key: key, Value: secret})
			}
		}
		applyCredential(input.PasswordAction, input.Password, entry.PasswordEnv, configpkg.RemotePasswordCredentialEnvName, func(value string) { entry.PasswordEnv = value })
		applyCredential(input.PassphraseAction, input.Passphrase, entry.PassphraseEnv, configpkg.RemotePassphraseCredentialEnvName, func(value string) { entry.PassphraseEnv = value })
		if err := cfg.UpsertRemoteHost(entry); err != nil {
			return nil, err
		}
		return append(credentialChanges, configpkg.UnusedGeneratedRemoteCredentialChanges(cfg, credentialRemovals)...), nil
	})
	if err != nil {
		return remoteSettingsView{}, err
	}
	return loadRemoteSettings()
}

func (b *bridgeServer) remoteSettings(w http.ResponseWriter, _ *http.Request) {
	view, err := loadRemoteSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview remote host settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) scanRemoteSSHConfig(w http.ResponseWriter, _ *http.Request) {
	view, err := scanRemoteSSHConfig()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to scan the user's SSH configuration")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeRemoteSettings(w http.ResponseWriter, r *http.Request) {
	var change remoteSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid Preview remote host settings request")
		return
	}
	var oldHost configpkg.RemoteHostEntry
	var hadOldHost bool
	if change.Action == "upsert" {
		if cfg, err := configpkg.LoadUserConfigReadOnly(); err == nil {
			oldHost, hadOldHost = cfg.RemoteHost(strings.TrimSpace(change.Host.Name))
		}
	}
	view, err := changeRemoteSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "remote host settings could not be saved")
		return
	}
	if change.Action == "remove" {
		b.remoteSessions.disconnect(strings.TrimSpace(change.Name))
	} else if change.Action == "upsert" {
		if !hadOldHost || remoteConnectionSettingsChanged(oldHost, change.Host) {
			b.remoteSessions.disconnect(strings.TrimSpace(change.Host.Name))
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func remoteConnectionSettingsChanged(old configpkg.RemoteHostEntry, next remoteSettingsHostInput) bool {
	return old.Host != strings.TrimSpace(next.Host) || old.Port != next.Port || old.User != strings.TrimSpace(next.User) ||
		old.IdentityFile != strings.TrimSpace(next.IdentityFile) || old.ProxyJump != strings.TrimSpace(next.ProxyJump) ||
		old.UseSSHConfig != next.UseSSHConfig || next.PasswordAction == "replace" || next.PasswordAction == "clear" ||
		next.PassphraseAction == "replace" || next.PassphraseAction == "clear"
}
