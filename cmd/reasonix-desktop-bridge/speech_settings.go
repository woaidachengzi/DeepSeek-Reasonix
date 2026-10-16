package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	configpkg "reasonix/internal/config"
)

const speechCredential = "REASONIX_DESKTOP_SPEECH_API_KEY"
const customSpeechPrefix = "custom:"

type speechSettingsFields struct {
	Mode     string `json:"mode"`
	Provider string `json:"provider"`
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
}
type speechSettingsView struct {
	ProtocolVersion int `json:"protocolVersion"`
	speechSettingsFields
	KeySet   bool   `json:"keySet"`
	Revision string `json:"revision"`
}
type speechSettingsChange struct {
	speechSettingsFields
	Revision  string `json:"revision"`
	KeyAction string `json:"keyAction"`
	APIKey    string `json:"apiKey"` // write-only, never returned or stored in TOML
}

func speechFields(cfg *configpkg.Config) speechSettingsFields {
	s := cfg.Desktop.Speech
	if s.Mode == "" {
		s.Mode = "mimo"
	}
	if s.Protocol == "" {
		s.Protocol = "transcriptions"
	}
	return speechSettingsFields{Mode: s.Mode, Provider: s.Provider, Protocol: s.Protocol, Endpoint: s.Endpoint, Model: s.Model}
}
func speechSavedKey() string {
	return configpkg.ResolveCredentialForRootGlobalFirst(".", speechCredential).Value
}
func speechView(cfg *configpkg.Config, key string) speechSettingsView {
	fields := speechFields(cfg)
	data, _ := json.Marshal(fields)
	sum := sha256.Sum256(append(append(data, 0), []byte(key)...))
	return speechSettingsView{ProtocolVersion: 1, speechSettingsFields: fields, KeySet: key != "", Revision: hex.EncodeToString(sum[:])}
}
func (b *bridgeServer) speechSettings(w http.ResponseWriter, r *http.Request) {
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		writeProtocolError(w, 500, "speech_config", "无法读取语音识别设置；请修复配置文件后重试。")
		return
	}
	writeJSON(w, 200, speechView(cfg, speechSavedKey()))
}
func validateSpeechEndpoint(endpoint string) bool {
	if len(endpoint) > 2048 || strings.IndexFunc(endpoint, unicode.IsControl) >= 0 {
		return false
	}
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Opaque == ""
}
func persistSpeechSettings(input speechSettingsChange) (speechSettingsView, error) {
	invalid := errors.New("语音设置无效；请检查服务地址、接口格式和模型名称。")
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.Model = strings.TrimSpace(input.Model)
	if (input.Mode != "mimo" && input.Mode != "custom") || (input.Protocol != "mimo" && input.Protocol != "transcriptions") || len(input.Model) > 256 || strings.IndexFunc(input.Model, unicode.IsControl) >= 0 || len(input.Provider) > 64 || len(input.APIKey) > 4096 || strings.IndexFunc(input.APIKey, unicode.IsControl) >= 0 {
		return speechSettingsView{}, invalid
	}
	if input.Endpoint != "" && !validateSpeechEndpoint(input.Endpoint) {
		return speechSettingsView{}, invalid
	}
	if input.Mode == "custom" && (input.Endpoint == "" || input.Model == "") {
		return speechSettingsView{}, invalid
	}
	if input.KeyAction != "keep" && input.KeyAction != "replace" && input.KeyAction != "clear" {
		return speechSettingsView{}, invalid
	}
	if (input.KeyAction == "replace" && strings.TrimSpace(input.APIKey) == "") || (input.KeyAction != "replace" && input.APIKey != "") {
		return speechSettingsView{}, invalid
	}
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	path := configpkg.UserConfigPath()
	cfg, err := configpkg.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return speechSettingsView{}, errors.New("无法读取语音配置；请修复配置文件后重试。")
	}
	oldKey := speechSavedKey()
	previous := speechView(cfg, oldKey)
	if input.Revision != previous.Revision {
		return speechSettingsView{}, errors.New("语音设置已变化；请重新加载后再保存。")
	}
	if input.Mode == "mimo" && input.Provider != "" {
		entry, found := cfg.Provider(input.Provider)
		if !found || !speechCandidate(entry) || !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Provider) {
			return speechSettingsView{}, errSpeechProvider
		}
	}
	// A key saved for one endpoint must never silently move to another service.
	if oldKey != "" && input.Endpoint != previous.Endpoint && input.KeyAction == "keep" {
		return speechSettingsView{}, errors.New("服务地址已改变；请重新输入 API Key，或清除原密钥后保存。")
	}
	baseline := cfg.ModelSettingsBaseline()
	cfg.Desktop.Speech = configpkg.DesktopSpeechConfig{Mode: input.Mode, Provider: input.Provider, Protocol: input.Protocol, Endpoint: input.Endpoint, Model: input.Model}
	key := oldKey
	switch input.KeyAction {
	case "replace":
		key = strings.TrimSpace(input.APIKey)
		_, err = configpkg.SetCredential(speechCredential, key)
	case "clear":
		key = ""
		err = configpkg.RemoveCredential(speechCredential)
	}
	if err != nil {
		return speechSettingsView{}, errors.New("无法保存语音密钥；请检查凭据存储后重试。")
	}
	if err = cfg.SaveUserSettingsDeltaTo(path, baseline); err != nil {
		if input.KeyAction != "keep" {
			if oldKey == "" {
				_ = configpkg.RemoveCredential(speechCredential)
			} else {
				_, _ = configpkg.SetCredential(speechCredential, oldKey)
			}
		}
		return speechSettingsView{}, errors.New("语音设置保存失败；请检查配置文件权限后重试。")
	}
	return speechView(cfg, key), nil
}
func (b *bridgeServer) saveSpeechSettings(w http.ResponseWriter, r *http.Request) {
	var input speechSettingsChange
	if decodeJSONBody(w, r, 16<<10, &input) != nil {
		writeProtocolError(w, 400, "speech_settings", "语音设置格式无效；请重新填写后保存。")
		return
	}
	view, err := persistSpeechSettings(input)
	if err != nil {
		writeProtocolError(w, 400, "speech_settings", err.Error())
		return
	}
	writeJSON(w, 200, view)
}
