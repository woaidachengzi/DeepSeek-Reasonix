package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
	"reasonix/internal/netclient"
)

func speechSettingsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("future_option='preserve'\n[network]\nproxy_mode='off'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return home
}
func currentSpeechView(t *testing.T) speechSettingsView {
	t.Helper()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	return speechView(cfg, speechSavedKey())
}
func TestSpeechSettingsPersistSecretsRevisionsAndUnknownFields(t *testing.T) {
	home := speechSettingsHome(t)
	initial := currentSpeechView(t)
	if initial.Mode != "mimo" || initial.KeySet {
		t.Fatal("legacy default changed")
	}
	input := speechSettingsChange{speechSettingsFields: speechSettingsFields{Mode: "custom", Protocol: "transcriptions", Endpoint: "http://127.0.0.1:8000/v1/audio/transcriptions", Model: "local-model"}, Revision: initial.Revision, KeyAction: "replace", APIKey: "private-speech-key"}
	saved, err := persistSpeechSettings(input)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.KeySet || saved.Revision == initial.Revision {
		t.Fatal("missing revision/secret state")
	}
	encoded, _ := json.Marshal(saved)
	raw, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	if strings.Contains(string(encoded), input.APIKey) || strings.Contains(string(raw), input.APIKey) || !strings.Contains(string(raw), "preserve") {
		t.Fatal("secret leak or unrelated config overwritten")
	}
	loaded := currentSpeechView(t)
	if loaded != saved {
		t.Fatalf("settings lost after reload: %+v", loaded)
	}
	if _, err = persistSpeechSettings(input); err == nil {
		t.Fatal("stale settings overwritten")
	}
	input.Revision = saved.Revision
	input.KeyAction = "keep"
	input.APIKey = ""
	input.Endpoint = "http://127.0.0.1:9000/v1/audio/transcriptions"
	if _, err = persistSpeechSettings(input); err == nil {
		t.Fatal("old key moved to new endpoint")
	}
	if speechSavedKey() != "private-speech-key" {
		t.Fatal("rejected save changed key")
	}
	input.KeyAction = "clear"
	saved, err = persistSpeechSettings(input)
	if err != nil || saved.KeySet || speechSavedKey() != "" {
		t.Fatal("clear failed", err)
	}
	if !strings.Contains(string(raw), "[desktop.speech]") {
		t.Fatal("missing speech section")
	}
	// Subsequent settings/template writers must preserve the speech configuration.
	cfg, _ := configpkg.LoadUserConfigReadOnly()
	unlock := configpkg.LockUserConfigEdits()
	err = cfg.SaveTo(configpkg.UserConfigPath())
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	if currentSpeechView(t) != saved {
		t.Fatal("canonical save dropped speech settings")
	}
}
func TestSpeechSettingsRejectInvalidAndMalformedConfig(t *testing.T) {
	home := speechSettingsHome(t)
	for _, endpoint := range []string{"ftp://host/a", "https://key@host/a", "https://host/a?key=secret", "https://host/a#fragment", "http:///missing"} {
		input := speechSettingsChange{speechSettingsFields: speechSettingsFields{Mode: "custom", Protocol: "mimo", Endpoint: endpoint, Model: "mimo-v2.5-asr"}, Revision: currentSpeechView(t).Revision, KeyAction: "replace", APIKey: "not-saved"}
		if _, err := persistSpeechSettings(input); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
		if speechSavedKey() != "" {
			t.Fatal("validation wrote secret")
		}
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := persistSpeechSettings(speechSettingsChange{speechSettingsFields: speechSettingsFields{Mode: "mimo", Protocol: "mimo"}, KeyAction: "replace", APIKey: "not-saved"}); err == nil || speechSavedKey() != "" {
		t.Fatal("malformed config overwritten or secret mutated")
	}
}
func TestSpeechCustomTranscriptionsRoutingAndRevisionFence(t *testing.T) {
	speechSettingsHome(t)
	calls := 0
	upstream := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/audio/transcriptions" || r.Header.Get("Authorization") != "" {
			t.Error("keyless custom service routing/auth mismatch")
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if !bytes.Equal(data, speechTestWav()) || header.Filename != "recording.wav" || r.FormValue("model") != "local-whisper" || r.FormValue("response_format") != "json" || r.FormValue("messages") != "" {
			t.Error("multipart request mismatch")
		}
		fmt.Fprint(w, `{"text":"本地语音测试"}`)
	}))
	defer upstream.Close()
	view := currentSpeechView(t)
	input := speechSettingsChange{speechSettingsFields: speechSettingsFields{Mode: "custom", Protocol: "transcriptions", Endpoint: upstream.URL + "/v1/audio/transcriptions", Model: "local-whisper"}, Revision: view.Revision, KeyAction: "keep"}
	view, err := persistSpeechSettings(input)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&bridgeServer{token: testToken}).handler()
	request := func(method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	for _, method := range []string{"GET", "POST"} {
		if request(method, "/v1/settings/speech", nil, false).Code != 401 {
			t.Fatal("unauthenticated settings allowed")
		}
	}
	if got := request("GET", "/v1/settings/speech", nil, true); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	catalog := request("GET", "/v1/speech/providers", nil, true)
	var entries speechCatalog
	if json.Unmarshal(catalog.Body.Bytes(), &entries) != nil || len(entries.Providers) != 1 || !entries.Providers[0].Preferred {
		t.Fatalf("custom catalogue: %s", catalog.Body)
	}
	body, _ := json.Marshal(speechRequest{Provider: entries.Providers[0].Name, Audio: base64.StdEncoding.EncodeToString(speechTestWav())})
	if got := request("POST", "/v1/speech/transcribe", body, true); got.Code != 200 || !strings.Contains(got.Body.String(), "本地语音测试") {
		t.Fatalf("custom speech: %d %s", got.Code, got.Body)
	}
	input.Revision = view.Revision
	input.Model = "changed-model"
	if _, err = persistSpeechSettings(input); err != nil {
		t.Fatal(err)
	}
	if got := request("POST", "/v1/speech/transcribe", body, true); got.Code != 400 {
		t.Fatal("stale recording sent to changed service")
	}
	if calls != 1 {
		t.Fatal("unexpected upstream request")
	}
	// Retaining a custom address while selecting MiMo must not make it an
	// implicit fallback when no MiMo channel is available.
	input.Mode = "mimo"
	input.Revision = currentSpeechView(t).Revision
	if _, err = persistSpeechSettings(input); err != nil {
		t.Fatal(err)
	}
	inactive := request("GET", "/v1/speech/providers", nil, true)
	if json.Unmarshal(inactive.Body.Bytes(), &entries) != nil || len(entries.Providers) != 0 {
		t.Fatal("inactive custom service exposed as fallback")
	}
	if got := request("POST", "/v1/settings/speech", []byte(`{"unknown":"field"}`), true); got.Code != 400 {
		t.Fatal("unknown settings accepted")
	}
}
func TestSpeechCustomMiMoFormatAndResponseMismatch(t *testing.T) {
	speechSettingsHome(t)
	for _, protocol := range []string{"mimo", "transcriptions"} {
		t.Run(protocol, func(t *testing.T) {
			server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer custom-key" {
					t.Error("custom key missing")
				}
				if protocol == "mimo" {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["model"] != "my-asr-model" {
						t.Error("model hardcoded")
					}
					fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"自定义模型"}}]}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"WRONG FORMAT"}}]}`)
				}
			}))
			defer server.Close()
			result, err := recognizeSpeechService(context.Background(), speechTarget{Endpoint: server.URL, Protocol: protocol, Model: "my-asr-model", Key: "custom-key"}, netclient.ProxySpec{Mode: netclient.ModeOff}, speechTestWav())
			if protocol == "mimo" && (err != nil || result != "自定义模型") {
				t.Fatal("custom MiMo failed", err)
			}
			if protocol == "transcriptions" && err == nil {
				t.Fatal("mismatched response accepted")
			}
		})
	}
}
