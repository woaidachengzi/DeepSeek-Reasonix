package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
)

func voiceProfileRequest(handler http.Handler, method, path string, input any, authorized bool) *httptest.ResponseRecorder {
	body, _ := json.Marshal(input)
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if authorized {
		r.Header.Set("Authorization", "Bearer "+testToken)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestSpeechVoiceLibraryPrivatePersistenceAndDeletion(t *testing.T) {
	home := speechSettingsHome(t)
	handler := (&bridgeServer{token: testToken}).handler()
	sample := base64.StdEncoding.EncodeToString(speechTestWav())
	input := speechVoiceImport{Name: "测试声音", Format: "wav", Audio: sample}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		path := "/v1/speech/voices"
		if method == "DELETE" {
			path += "/clone:" + strings.Repeat("a", 32)
		}
		if got := voiceProfileRequest(handler, method, path, input, false); got.Code != 401 {
			t.Fatalf("%s authorization=%d", method, got.Code)
		}
	}
	added := voiceProfileRequest(handler, "POST", "/v1/speech/voices", input, true)
	var catalog speechVoiceCatalog
	if added.Code != 200 || json.Unmarshal(added.Body.Bytes(), &catalog) != nil || len(catalog.Voices) != 1 {
		t.Fatalf("import failed: %d %s", added.Code, added.Body)
	}
	id := catalog.Voices[0].ID
	if !voiceProfileID.MatchString(id) || strings.Contains(added.Body.String(), sample) || strings.Contains(added.Body.String(), home) {
		t.Fatal("catalog exposed private reference data")
	}
	path := filepath.Join(home, "voices", strings.TrimPrefix(id, "clone:")+".json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("sample permissions: %v %v", info, err)
	}
	listed := voiceProfileRequest((&bridgeServer{token: testToken}).handler(), "GET", "/v1/speech/voices", nil, true)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), id) || strings.Contains(listed.Body.String(), "audio") {
		t.Fatal("catalog did not survive a new bridge instance or exposed sample")
	}
	model, reference, err := resolveSynthesisVoice(id)
	if err != nil || model != "mimo-v2.5-tts-voiceclone" || reference != "data:audio/wav;base64,"+sample {
		t.Fatal("saved reference changed", err)
	}
	if got := voiceProfileRequest(handler, "POST", "/v1/speech/voices", input, true); got.Code != 409 {
		t.Fatal("duplicate name accepted")
	}
	if got := voiceProfileRequest(handler, "DELETE", "/v1/speech/voices/"+id, nil, true); got.Code != 200 || strings.Contains(got.Body.String(), id) {
		t.Fatal("delete failed")
	}
	if _, _, err := resolveSynthesisVoice(id); err == nil {
		t.Fatal("deleted voice silently fell back to another identity")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("deleted reference retained")
	}
}
func TestSpeechVoiceRejectsMalformedSamplesIDsAndSymlinks(t *testing.T) {
	home := speechSettingsHome(t)
	handler := (&bridgeServer{token: testToken}).handler()
	sample := base64.StdEncoding.EncodeToString(speechTestWav())
	for _, input := range []speechVoiceImport{
		{Name: "", Format: "wav", Audio: sample}, {Name: "line\nbreak", Format: "wav", Audio: sample}, {Name: strings.Repeat("字", 41), Format: "wav", Audio: sample},
		{Name: "test", Format: "m4a", Audio: sample}, {Name: "test", Format: "mp3", Audio: sample}, {Name: "test", Format: "wav", Audio: "!!!!"},
		{Name: "test", Format: "wav", Audio: base64.StdEncoding.EncodeToString(make([]byte, voiceSampleLimit+1))},
	} {
		if got := voiceProfileRequest(handler, "POST", "/v1/speech/voices", input, true); got.Code != 400 {
			t.Fatalf("invalid voice admitted: %d", got.Code)
		}
	}
	for _, id := range []string{"../config.toml", "https://example.com/voice.wav", "clone:" + strings.Repeat("z", 32), sample} {
		if _, _, err := resolveSynthesisVoice(id); err == nil {
			t.Fatal("arbitrary voice/path admitted")
		}
	}
	directory := filepath.Join(home, "voices")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	id := "clone:" + strings.Repeat("b", 32)
	if err := os.Symlink(filepath.Join(home, "config.toml"), filepath.Join(directory, strings.TrimPrefix(id, "clone:")+".json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveSynthesisVoice(id); err == nil {
		t.Fatal("symlink used as voice sample")
	}
}
func TestSpeechCloningRoutesSelectedReferenceOnly(t *testing.T) {
	home := speechSettingsHome(t)
	handler := (&bridgeServer{token: testToken}).handler()
	ids := []string{}
	samples := []string{}
	for i := 0; i < 2; i++ {
		wav := speechTestWav()
		wav[len(wav)-1] = byte(i + 1)
		samples = append(samples, base64.StdEncoding.EncodeToString(wav))
		res := voiceProfileRequest(handler, "POST", "/v1/speech/voices", speechVoiceImport{Name: fmt.Sprintf("Fixture %d", i), Format: "wav", Audio: samples[i]}, true)
		var catalog speechVoiceCatalog
		if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &catalog) != nil {
			t.Fatal("import failed")
		}
		for _, voice := range catalog.Voices {
			if voice.Name == fmt.Sprintf("Fixture %d", i) {
				ids = append(ids, voice.ID)
			}
		}
	}
	want := 0
	calls := 0
	server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input struct {
			Model    string
			Messages []map[string]string
			Audio    map[string]string
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Model != "mimo-v2.5-tts-voiceclone" || len(input.Messages) != 2 || input.Messages[0]["role"] != "user" || input.Messages[1]["content"] != "新的一句话。" || input.Audio["voice"] != "data:audio/wav;base64,"+samples[want] || r.Header.Get("Authorization") != "Bearer fixture-clone-key" {
			t.Error("clone contract or identity mismatch")
		}
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"audio":{"data":%q}}}]}`, base64.StdEncoding.EncodeToString(speechTestWav()))
	}))
	defer server.Close()
	config := fmt.Sprintf("[[providers]]\nname='mimo'\nkind='openai'\nbase_url=%q\nmodels=['mimo-v2.6-flash']\napi_key_env='CLONE_FIXTURE_KEY'\n[network]\nproxy_mode='off'\n", server.URL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := configpkg.SetCredential("CLONE_FIXTURE_KEY", "fixture-clone-key"); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		want = i
		res := voiceProfileRequest(handler, "POST", "/v1/speech/synthesize", speechSynthesisRequest{Provider: "mimo", Text: "新的一句话。", Voice: id}, true)
		if res.Code != 200 || strings.Contains(res.Body.String(), "fixture-clone-key") || strings.Contains(res.Body.String(), samples[i]) {
			t.Fatalf("clone failed or leaked: %d", res.Code)
		}
	}
	if calls != 2 {
		t.Fatal("unexpected dispatch count", calls)
	}
}
