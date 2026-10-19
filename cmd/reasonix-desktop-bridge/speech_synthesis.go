package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	configpkg "reasonix/internal/config"
	"reasonix/internal/netclient"
)

const speechOutputLimit = 8 << 20

var speechVoices = map[string]bool{"mimo_default": true, "冰糖": true, "茉莉": true, "苏打": true, "白桦": true, "Mia": true, "Chloe": true, "Milo": true, "Dean": true}

type speechSynthesisRequest struct {
	Provider string `json:"provider"`
	Text     string `json:"text"`
	Voice    string `json:"voice"`
}
type speechAudio struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Audio           string `json:"audio"`
}

func (b *bridgeServer) speechSynthesize(w http.ResponseWriter, r *http.Request) {
	var input speechSynthesisRequest
	if decodeJSONBody(w, r, 16<<10, &input) != nil || !previewProviderName.MatchString(input.Provider) || (!speechVoices[input.Voice] && !voiceProfileID.MatchString(input.Voice)) || strings.TrimSpace(input.Text) == "" || len(input.Text) > 6000 || utf8.RuneCountInString(input.Text) > 600 || !utf8.ValidString(input.Text) || strings.ContainsRune(input.Text, 0) {
		writeProtocolError(w, 400, "speech_synthesis", "播报内容或音色无效；请缩短内容或重新选择音色。")
		return
	}
	select {
	case speechSlots <- struct{}{}:
		defer func() { <-speechSlots }()
	default:
		writeProtocolError(w, 429, "speech_busy", "语音服务繁忙；请等待当前请求结束后重试。")
		return
	}
	unlock := configpkg.LockUserConfigEdits()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		unlock()
		writeProtocolError(w, 500, "speech_config", "无法读取播报渠道；请检查语音设置。")
		return
	}
	entry, found := cfg.Provider(input.Provider)
	if !found || !speechCandidate(entry) || !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Provider) {
		unlock()
		writeProtocolError(w, 400, "speech_provider", "播报渠道不可用；请选择支持 MiMo TTS 的渠道。")
		return
	}
	resolved := *entry
	resolved.ResolveAPIKeyForRoot(".")
	proxy := cfg.NetworkProxySpec()
	unlock()
	if !resolved.Configured() || resolved.CredentialProxyURL() != "" {
		writeProtocolError(w, 400, "speech_provider", "播报渠道未配置；请检查 API Key 和语音合成权限。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	audio, err := synthesizeSpeech(ctx, speechTargetForProvider(&resolved), proxy, input.Text, input.Voice)
	if err != nil {
		writeSpeechFailure(w, "tts", err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	writeJSON(w, 200, speechAudio{ProtocolVersion: 1, Audio: audio})
}
func synthesizeSpeech(ctx context.Context, target speechTarget, proxy netclient.ProxySpec, text, voice string) (string, error) {
	model, resolvedVoice, err := resolveSynthesisVoice(voice)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(target.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return "", errors.New("播报地址无效；请检查所选 MiMo 渠道。")
	}
	client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 40 * time.Second})
	if err != nil {
		return "", errors.New("无法连接播报服务；请检查网络设置。")
	}
	defer client.CloseIdleConnections()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	messages := []any{map[string]string{"role": "assistant", "content": text}}
	if model == "mimo-v2.5-tts-voiceclone" {
		messages = append([]any{map[string]string{"role": "user", "content": ""}}, messages...)
	}
	body, _ := json.Marshal(map[string]any{"model": model, "stream": false, "messages": messages, "audio": map[string]string{"format": "wav", "voice": resolvedVoice}})
	request, err := http.NewRequestWithContext(ctx, "POST", target.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("无法创建播报请求；请检查所选渠道。")
	}
	for k, v := range target.Headers {
		request.Header.Set(k, v)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+target.Key)
	response, err := client.Do(request)
	if err != nil {
		return "", speechConnectionFailure(err, "语音合成失败或超时；请检查网络，文字回答仍可查看。")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 401, 403:
		return "", speechFailure("auth", "播报渠道未授权；请检查 API Key 及 MiMo TTS 权限。")
	case 400, 404, 422:
		return "", speechFailure("unsupported", "播报渠道不支持当前语音请求；请检查 MiMo TTS 权限、接口和音色。")
	case 429:
		return "", speechFailure("rate_limit", "播报渠道额度不足或请求过多；请检查额度或稍后重试。")
	}
	if response.StatusCode != 200 {
		return "", speechFailure("upstream", "语音合成服务返回异常；文字回答仍可查看，请稍后重试。")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 12<<20+1))
	if err != nil || len(raw) > 12<<20 {
		if err != nil {
			return "", speechConnectionFailure(err, "播报音频读取失败；请重试播报。")
		}
		return "", speechFailure("result", "播报音频过大；请缩短回答后重试。")
	}
	var result struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" {
		return "", speechFailure("result", "语音合成结果不完整；请稍后重试。")
	}
	encoded := result.Choices[0].Message.Audio.Data
	if len(encoded) > base64.StdEncoding.EncodedLen(speechOutputLimit) {
		return "", speechFailure("result", "播报音频过大；请缩短回答后重试。")
	}
	wav, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(wav) < 44 || len(wav) > speechOutputLimit || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return "", speechFailure("result", "播报服务未返回有效 WAV 音频；请检查渠道的语音合成支持。")
	}
	return base64.StdEncoding.EncodeToString(wav), nil
}
