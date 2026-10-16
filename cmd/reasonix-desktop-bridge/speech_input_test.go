package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
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

func speechTestWav() []byte {
	wav := make([]byte, 44+3200)
	copy(wav, "RIFF")
	copy(wav[8:], "WAVEfmt ")
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 16000)
	binary.LittleEndian.PutUint32(wav[28:], 32000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
	return wav
}
func TestSpeechWavRejectsInvalidEncodingSizesAndHeaders(t *testing.T) {
	good := speechTestWav()
	if _, err := decodeSpeechAudio(base64.StdEncoding.EncodeToString(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, []byte("RIFF"), append(append([]byte{}, good...), 0), make([]byte, speechAudioLimit+1)} {
		if _, err := decodeSpeechAudio(base64.StdEncoding.EncodeToString(bad)); err == nil {
			t.Fatal("invalid wav accepted")
		}
	}
	for _, offset := range []int{0, 8, 12, 16, 20, 22, 24, 28, 32, 34, 36, 40, 4} {
		bad := append([]byte{}, good...)
		bad[offset] ^= 1
		if _, err := decodeSpeechAudio(base64.StdEncoding.EncodeToString(bad)); err == nil {
			t.Fatalf("header at %d accepted", offset)
		}
	}
	if _, err := decodeSpeechAudio("!!!!"); err == nil {
		t.Fatal("invalid base64 accepted")
	}
}
func TestSpeechUsesSavedProviderAndOnlyRecordedAudio(t *testing.T) {
	var calls atomic.Int32
	upstream := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("X-Saved") != "saved" {
			t.Errorf("provider routing/auth mismatch: path=%q auth=%t saved=%t", r.URL.Path, r.Header.Get("Authorization") == "Bearer fixture-key", r.Header.Get("X-Saved") == "saved")
		}
		var body struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type  string `json:"type"`
					Audio struct {
						Data   string `json:"data"`
						Format string `json:"format"`
					} `json:"input_audio"`
				} `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != speechModel || body.Stream || len(body.Messages) != 1 || body.Messages[0].Role != "user" || len(body.Messages[0].Content) != 1 {
			t.Error("unexpected speech request")
		}
		part := body.Messages[0].Content[0]
		if part.Type != "input_audio" || part.Audio.Format != "wav" || part.Audio.Data != base64.StdEncoding.EncodeToString(speechTestWav()) {
			t.Error("audio changed")
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"测试语音。"}}]}`)
	}))
	defer upstream.Close()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("VOICE_FIXTURE_KEY", "fixture-key")
	config := fmt.Sprintf("[[providers]]\nname='voice'\ndisplay_name='语音测试'\nkind='openai'\nbase_url=%q\nmodels=['mimo-v2.6-flash']\napi_key_env='VOICE_FIXTURE_KEY'\n[providers.headers]\nX-Saved='saved'\n[network]\nproxy_mode='off'\n", upstream.URL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := configpkg.StoreCredentialLines([]string{"VOICE_FIXTURE_KEY=fixture-key"}); err != nil {
		t.Fatal(err)
	}
	bridge := &bridgeServer{token: testToken}
	handler := bridge.handler()
	request := func(method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	if got := request("GET", "/v1/speech/providers", nil, false); got.Code != 401 {
		t.Fatalf("unauthenticated catalogue=%d", got.Code)
	}
	catalog := request("GET", "/v1/speech/providers", nil, true)
	if catalog.Code != 200 || !strings.Contains(catalog.Body.String(), "语音测试") || strings.Contains(catalog.Body.String(), "fixture-key") || strings.Contains(catalog.Body.String(), upstream.URL) {
		t.Fatalf("catalogue privacy: %d %s", catalog.Code, catalog.Body)
	}
	body, _ := json.Marshal(speechRequest{Provider: "voice", Audio: base64.StdEncoding.EncodeToString(speechTestWav())})
	if got := request("POST", "/v1/speech/transcribe", body, false); got.Code != 401 {
		t.Fatal("unauthenticated transcription admitted")
	}
	good := request("POST", "/v1/speech/transcribe", body, true)
	if good.Code != 200 || !strings.Contains(good.Body.String(), "测试语音。") {
		t.Fatalf("transcription: %d %s", good.Code, good.Body)
	}
	if got := request("POST", "/v1/speech/transcribe", []byte(`{"provider":"voice","audio":"invalid","url":"http://private"}`), true); got.Code != 400 {
		t.Fatal("unknown input accepted")
	}
	bad, _ := json.Marshal(speechRequest{Provider: "missing", Audio: base64.StdEncoding.EncodeToString(speechTestWav())})
	if got := request("POST", "/v1/speech/transcribe", bad, true); got.Code != 400 {
		t.Fatal("missing provider accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("rejected input reached upstream")
	}
	after, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	if string(after) != config {
		t.Fatal("recognition modified config")
	}
}
func TestSpeechNoRedirectRetryOrPrivateErrorLeak(t *testing.T) {
	var redirected atomic.Int32
	target := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, status := range []int{302, 401, 403, 404, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls int
			server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				fmt.Fprint(w, `{"secret":"PRIVATE", "choices":[{"finish_reason":"length","message":{"content":"partial"}}]}`)
			}))
			defer server.Close()
			entry := (&configpkg.ProviderEntry{BaseURL: server.URL, Kind: "openai"}).WithAPIKeyForProbe("key")
			_, err := recognizeSpeech(context.Background(), &entry, netclient.ProxySpec{Mode: netclient.ModeOff}, speechTestWav())
			if err == nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), server.URL) || calls != 1 {
				t.Fatal("error handling/retry unsafe", err, calls)
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect sent audio/credentials to another host")
	}
}
func TestSpeechCancellationReachesUpstream(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := (&configpkg.ProviderEntry{BaseURL: server.URL, Kind: "openai"}).WithAPIKeyForProbe("key")
	done := make(chan error, 1)
	go func() {
		_, err := recognizeSpeech(ctx, &entry, netclient.ProxySpec{Mode: netclient.ModeOff}, speechTestWav())
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no dispatch")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream cancellation missing")
	}
	if <-done == nil {
		t.Fatal("cancelled recognition succeeded")
	}
}
