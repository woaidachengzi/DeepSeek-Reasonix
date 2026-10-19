package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	configpkg "reasonix/internal/config"
	"reasonix/internal/netclient"
)

const speechModel = "mimo-v2.5-asr"
const speechAudioLimit = 16000*2*90 + 44
const speechTextLimit = 32000

var speechSlots = make(chan struct{}, 2)
var errSpeechProvider = errors.New("语音渠道不可用；请在语音识别设置中配置服务或选择支持 MiMo ASR 的渠道。")
var errSpeechAudio = errors.New("录音格式无效或超过 90 秒；请重新录制。")

type speechProviderView struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Preferred   bool   `json:"preferred,omitempty"`
	Description string `json:"description,omitempty"`
}
type speechCatalog struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Providers       []speechProviderView `json:"providers"`
}
type speechRequest struct {
	Provider string `json:"provider"`
	Audio    string `json:"audio"`
}
type speechTranscript struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Text            string `json:"text"`
}

func speechCandidate(entry *configpkg.ProviderEntry) bool {
	if entry == nil || (entry.Kind != "openai" && entry.Kind != "") {
		return false
	}
	for _, m := range entry.ModelList() {
		if strings.Contains(strings.ToLower(m), "mimo") {
			return true
		}
	}
	u, _ := url.Parse(entry.BaseURL)
	return u != nil && (u.Hostname() == "api.xiaomimimo.com" || u.Hostname() == "api.xiaomimimo.com.cn")
}

func (b *bridgeServer) speechProviders(w http.ResponseWriter, r *http.Request) {
	unlock := configpkg.LockUserConfigEdits()
	defer unlock()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		writeProtocolError(w, 500, "speech_config", "无法读取语音渠道；请重新打开语音输入。")
		return
	}
	result := speechCatalog{ProtocolVersion: 1, Providers: []speechProviderView{}}
	view := speechView(cfg, speechSavedKey())
	if view.Mode == "custom" && view.Endpoint != "" && view.Model != "" && validateSpeechEndpoint(view.Endpoint) {
		result.Providers = append(result.Providers, speechProviderView{Name: customSpeechPrefix + view.Revision, Label: "自定义识别服务 · " + view.Model, Preferred: view.Mode == "custom", Description: "录音将发送到你配置的自定义服务。"})
	}
	for i := range cfg.Providers {
		entry := &cfg.Providers[i]
		if !speechCandidate(entry) || !providerAccessAllowed(cfg.Desktop.ProviderAccess, entry.Name) {
			continue
		}
		entry.ResolveAPIKeyForRoot(".")
		if !entry.Configured() || entry.CredentialProxyURL() != "" {
			continue
		}
		label := strings.TrimSpace(entry.DisplayName)
		if label == "" {
			label = entry.Name
		}
		result.Providers = append(result.Providers, speechProviderView{Name: entry.Name, Label: label, Preferred: view.Mode == "mimo" && entry.Name == view.Provider, Description: "使用 MiMo ASR，需有效权限及可用额度。"})
	}
	if view.Mode == "mimo" && view.Provider == "" {
		for i := range result.Providers {
			if !strings.HasPrefix(result.Providers[i].Name, customSpeechPrefix) {
				result.Providers[i].Preferred = true
				break
			}
		}
	}
	writeJSON(w, 200, result)
}

func decodeSpeechAudio(encoded string) ([]byte, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(speechAudioLimit) {
		return nil, errSpeechAudio
	}
	wav, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(wav) < 46 || len(wav) > speechAudioLimit {
		return nil, errSpeechAudio
	}
	// One canonical PCM16 mono 16kHz WAV layout. No paths, arbitrary codecs or
	// decoder subprocesses enter this endpoint.
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || string(wav[12:16]) != "fmt " ||
		binary.LittleEndian.Uint32(wav[16:20]) != 16 || binary.LittleEndian.Uint16(wav[20:22]) != 1 ||
		binary.LittleEndian.Uint16(wav[22:24]) != 1 || binary.LittleEndian.Uint32(wav[24:28]) != 16000 ||
		binary.LittleEndian.Uint32(wav[28:32]) != 32000 || binary.LittleEndian.Uint16(wav[32:34]) != 2 ||
		binary.LittleEndian.Uint16(wav[34:36]) != 16 || string(wav[36:40]) != "data" ||
		int(binary.LittleEndian.Uint32(wav[40:44])) != len(wav)-44 ||
		int(binary.LittleEndian.Uint32(wav[4:8])) != len(wav)-8 || (len(wav)-44)%2 != 0 {
		return nil, errSpeechAudio
	}
	return wav, nil
}

func (b *bridgeServer) speechTranscribe(w http.ResponseWriter, r *http.Request) {
	var input speechRequest
	if decodeJSONBody(w, r, 4<<20, &input) != nil || (!previewProviderName.MatchString(input.Provider) && !(strings.HasPrefix(input.Provider, customSpeechPrefix) && len(input.Provider) == len(customSpeechPrefix)+64)) {
		writeProtocolError(w, 400, "speech_input", errSpeechAudio.Error())
		return
	}
	audio, err := decodeSpeechAudio(input.Audio)
	if err != nil {
		writeProtocolError(w, 400, "speech_input", err.Error())
		return
	}
	select {
	case speechSlots <- struct{}{}:
		defer func() { <-speechSlots }()
	default:
		writeProtocolError(w, 429, "speech_busy", "语音识别繁忙；请等待当前识别完成后重试。")
		return
	}
	unlock := configpkg.LockUserConfigEdits()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		unlock()
		writeProtocolError(w, 500, "speech_config", "无法读取语音配置；请重新打开语音输入。")
		return
	}
	var target speechTarget
	if strings.HasPrefix(input.Provider, customSpeechPrefix) {
		key := speechSavedKey()
		view := speechView(cfg, key)
		if input.Provider != customSpeechPrefix+view.Revision || !validateSpeechEndpoint(view.Endpoint) || view.Model == "" || (view.Protocol != "mimo" && view.Protocol != "transcriptions") {
			unlock()
			writeProtocolError(w, 400, "speech_provider", "自定义语音设置已变化或无效；请刷新语音渠道后重新录音。")
			return
		}
		target = speechTarget{Endpoint: view.Endpoint, Model: view.Model, Protocol: view.Protocol, Key: key}
	} else {
		entry, found := cfg.Provider(input.Provider)
		if !found || !speechCandidate(entry) || !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Provider) {
			unlock()
			writeProtocolError(w, 400, "speech_provider", errSpeechProvider.Error())
			return
		}
		resolved := *entry
		resolved.ResolveAPIKeyForRoot(".")
		if !resolved.Configured() || resolved.CredentialProxyURL() != "" {
			unlock()
			writeProtocolError(w, 400, "speech_provider", errSpeechProvider.Error())
			return
		}
		target = speechTargetForProvider(&resolved)
	}
	proxy := cfg.NetworkProxySpec()
	unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	text, err := recognizeSpeechService(ctx, target, proxy, audio)
	if err != nil {
		writeSpeechFailure(w, "asr", err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	writeJSON(w, 200, speechTranscript{ProtocolVersion: 1, Text: text})
}

type speechTarget struct {
	Endpoint, Model, Protocol, Key string
	Headers                        map[string]string
}

func speechTargetForProvider(entry *configpkg.ProviderEntry) speechTarget {
	endpoint := strings.TrimSpace(entry.RequestURL)
	if endpoint == "" {
		endpoint = strings.TrimSpace(entry.ChatURL)
	}
	if endpoint == "" {
		endpoint = strings.TrimRight(strings.TrimSpace(entry.BaseURL), "/") + "/chat/completions"
	}
	return speechTarget{Endpoint: endpoint, Model: speechModel, Protocol: "mimo", Key: entry.APIKey(), Headers: entry.Headers}
}
func recognizeSpeech(ctx context.Context, entry *configpkg.ProviderEntry, proxy netclient.ProxySpec, audio []byte) (string, error) {
	return recognizeSpeechService(ctx, speechTargetForProvider(entry), proxy, audio)
}
func recognizeSpeechService(ctx context.Context, target speechTarget, proxy netclient.ProxySpec, audio []byte) (string, error) {
	endpoint := target.Endpoint
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return "", errSpeechProvider
	}
	client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 40 * time.Second})
	if err != nil {
		return "", errSpeechProvider
	}
	defer client.CloseIdleConnections()
	// Never forward saved credentials or recorded audio to a redirect target.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var body bytes.Buffer
	contentType := "application/json"
	if target.Protocol == "transcriptions" {
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("model", target.Model)
		_ = writer.WriteField("response_format", "json")
		part, err := writer.CreateFormFile("file", "recording.wav")
		if err != nil {
			return "", errSpeechAudio
		}
		if _, err = part.Write(audio); err != nil {
			return "", errSpeechAudio
		}
		if writer.Close() != nil {
			return "", errSpeechAudio
		}
		contentType = writer.FormDataContentType()
	} else if target.Protocol == "mimo" {
		if err := json.NewEncoder(&body).Encode(map[string]any{"model": target.Model, "stream": false, "asr_options": map[string]string{"language": "auto"}, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_audio", "input_audio": map[string]string{"data": base64.StdEncoding.EncodeToString(audio), "format": "wav"}}}}}}); err != nil {
			return "", errSpeechAudio
		}
	} else {
		return "", errSpeechProvider
	}
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint, &body)
	if err != nil {
		return "", errSpeechProvider
	}
	for key, value := range target.Headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", contentType)
	if target.Key != "" {
		request.Header.Set("Authorization", "Bearer "+target.Key)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", speechConnectionFailure(err, "语音识别连接失败或超时；请检查网络和所选渠道后重试。")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 401, 403:
		return "", speechFailure("auth", "语音渠道未授权；请检查 API Key 及语音识别使用权限。")
	case 404:
		return "", speechFailure("unsupported", "语音接口不存在；请在语音识别设置中检查服务地址和接口格式。")
	case 429:
		return "", speechFailure("rate_limit", "语音渠道额度不足或请求过多；请检查额度或稍后重试。")
	}
	if response.StatusCode != 200 {
		return "", speechFailure("upstream", "语音服务返回异常；请稍后重试或切换语音渠道。")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 256<<10+1))
	if err != nil || len(data) > 256<<10 {
		if err != nil {
			return "", speechConnectionFailure(err, "语音识别结果读取失败；请重新录音。")
		}
		return "", speechFailure("result", "语音识别结果过大；请缩短录音后重试。")
	}
	var decoded struct {
		Text    string `json:"text"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &decoded) != nil {
		return "", speechFailure("result", "语音服务响应格式不匹配；请检查接口格式后重试。")
	}
	text := strings.TrimSpace(decoded.Text)
	if target.Protocol == "mimo" {
		if len(decoded.Choices) != 1 || decoded.Choices[0].FinishReason != "stop" {
			return "", speechFailure("result", "语音识别结果不完整；请检查接口格式或重新录音。")
		}
		text = strings.TrimSpace(decoded.Choices[0].Message.Content)
	}
	if text == "" || len(text) > speechTextLimit || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return "", speechFailure("no_text", "没有识别到有效语音；请靠近麦克风重新录制。")
	}
	return text, nil
}
