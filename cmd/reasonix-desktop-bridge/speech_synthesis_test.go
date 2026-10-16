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
	"sync/atomic"
	"testing"
	"time"

	configpkg "reasonix/internal/config"
	"reasonix/internal/netclient"
)

func TestSpeechSynthesisSavedChannelAudioAndAuthorization(t *testing.T) {
	home := speechSettingsHome(t)
	var calls atomic.Int32
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model    string              `json:"model"`
			Stream   bool                `json:"stream"`
			Messages []map[string]string `json:"messages"`
			Audio    map[string]string   `json:"audio"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Model != "mimo-v2.5-tts" || input.Stream || len(input.Messages) != 1 || input.Messages[0]["role"] != "assistant" || input.Messages[0]["content"] != "你好，欢迎。" || input.Audio["format"] != "wav" || input.Audio["voice"] != "冰糖" || r.Header.Get("Authorization") != "Bearer tts-fixture-key" || r.URL.Path != "/v1/chat/completions" {
			t.Error("TTS contract mismatch")
		}
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"audio":{"data":%q}}}]}`, base64.StdEncoding.EncodeToString(speechTestWav()))
	}))
	defer server.Close()
	config := fmt.Sprintf("[[providers]]\nname='mimo-token-plan'\nkind='openai'\nmodels=['mimo-v2.6-flash']\nbase_url=%q\napi_key_env='TTS_TEST_KEY'\n[network]\nproxy_mode='off'\n", server.URL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := configpkg.SetCredential("TTS_TEST_KEY", "tts-fixture-key"); err != nil {
		t.Fatal(err)
	}
	handler := (&bridgeServer{token: testToken}).handler()
	request := func(body []byte, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/speech/synthesize", bytes.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	input := speechSynthesisRequest{Provider: "mimo-token-plan", Text: "你好，欢迎。", Voice: "冰糖"}
	body, _ := json.Marshal(input)
	if request(body, false).Code != 401 {
		t.Fatal("unauthenticated TTS admitted")
	}
	result := request(body, true)
	var audio speechAudio
	if result.Code != 200 || json.Unmarshal(result.Body.Bytes(), &audio) != nil || audio.Audio != base64.StdEncoding.EncodeToString(speechTestWav()) || strings.Contains(result.Body.String(), "tts-fixture-key") {
		t.Fatalf("TTS response invalid: %s", result.Body)
	}
	for _, invalid := range []speechSynthesisRequest{{Provider: "missing", Text: "hello", Voice: "mimo_default"}, {Provider: input.Provider, Text: "hello", Voice: "unknown"}, {Provider: input.Provider, Text: strings.Repeat("字", 601), Voice: "mimo_default"}, {Provider: "custom:abc", Text: "hello", Voice: "mimo_default"}} {
		body, _ = json.Marshal(invalid)
		if request(body, true).Code != 400 {
			t.Fatal("invalid TTS admitted")
		}
	}
	if request([]byte(`{"provider":"mimo-token-plan","text":"hello","voice":"冰糖","url":"http://other"}`), true).Code != 400 {
		t.Fatal("unknown input accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("rejected input reached upstream")
	}
}
func TestSpeechSynthesisNoRedirectRetryOrPrivateErrorLeak(t *testing.T) {
	for _, status := range []int{302, 400, 401, 403, 404, 422, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "http://127.0.0.1:1/private")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"secret":"PRIVATE","choices":[{"finish_reason":"stop","message":{"audio":{"data":"NOT WAV"}}}]}`)
			}))
			defer server.Close()
			_, err := synthesizeSpeech(context.Background(), speechTarget{Endpoint: server.URL, Key: "private-key"}, netclient.ProxySpec{Mode: netclient.ModeOff}, "hello", "mimo_default")
			if err == nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), server.URL) || calls != 1 {
				t.Fatal("unsafe error or retry", err, calls)
			}
		})
	}
}
func TestSpeechSynthesisCancellationReachesUpstream(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := synthesizeSpeech(ctx, speechTarget{Endpoint: server.URL, Key: "key"}, netclient.ProxySpec{Mode: netclient.ModeOff}, "hello", "mimo_default")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no TTS dispatch")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("TTS upstream not cancelled")
	}
	if <-done == nil {
		t.Fatal("cancelled TTS succeeded")
	}
}
