package main

import (
	"fmt"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/secrets"
)

type secretsSettingsView struct {
	ProtocolVersion       int  `json:"protocolVersion"`
	FilterSubprocessEnv   bool `json:"filterSubprocessEnv"`
	ProtectSensitiveFiles bool `json:"protectSensitiveFiles"`
}

type secretsSettingsChange struct {
	FilterSubprocessEnv   *bool `json:"filterSubprocessEnv"`
	ProtectSensitiveFiles *bool `json:"protectSensitiveFiles"`
}

func secretsSettingsFromConfig(cfg *configpkg.Config) secretsSettingsView {
	return secretsSettingsView{
		ProtocolVersion:       desktopbridge.ProtocolVersion,
		FilterSubprocessEnv:   cfg.Secrets.FilterSubprocessEnv,
		ProtectSensitiveFiles: cfg.Secrets.ProtectSensitiveFiles,
	}
}

func loadSecretsSettings() (secretsSettingsView, error) {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return secretsSettingsView{}, err
	}
	return secretsSettingsFromConfig(cfg), nil
}

func persistSecretsSettings(change secretsSettingsChange) (secretsSettingsView, error) {
	if change.FilterSubprocessEnv == nil && change.ProtectSensitiveFiles == nil {
		return secretsSettingsView{}, fmt.Errorf("at least one secrets setting is required")
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	if path == "" {
		return secretsSettingsView{}, fmt.Errorf("resolve Preview user config path")
	}
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return secretsSettingsView{}, err
	}
	baseline := cfg.ModelSettingsBaseline()
	if change.FilterSubprocessEnv != nil {
		cfg.Secrets.FilterSubprocessEnv = *change.FilterSubprocessEnv
	}
	if change.ProtectSensitiveFiles != nil {
		cfg.Secrets.ProtectSensitiveFiles = *change.ProtectSensitiveFiles
	}
	if err := cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		return secretsSettingsView{}, err
	}
	// These protections are user-global by design. Update the process-wide
	// runtime guards after the config has been durably written so running and
	// future Preview sessions use the same settings immediately.
	secrets.SetFilterSubprocessEnv(cfg.Secrets.FilterSubprocessEnv)
	secrets.SetProtectSensitiveFiles(cfg.Secrets.ProtectSensitiveFiles)
	return secretsSettingsFromConfig(cfg), nil
}
