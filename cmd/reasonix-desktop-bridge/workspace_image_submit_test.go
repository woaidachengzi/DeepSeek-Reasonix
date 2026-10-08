package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

func TestControllerRuntimeFreezesExplicitVisionCapabilityForGlobalImages(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 64, 40))); err != nil {
		t.Fatal(err)
	}
	type observed struct {
		images                  int
		ownedPrompt, exactImage bool
		lastUserKind            string
	}
	requests := make(chan observed, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("image provider fixture request malformed")
			return
		}
		var record observed
		for _, message := range body.Messages {
			if message.Role != "user" {
				continue
			}
			var text string
			if json.Unmarshal(message.Content, &text) == nil {
				record.lastUserKind = "text"
				record.ownedPrompt = record.ownedPrompt || strings.Contains(text, "image-owned-turn")
				continue
			}
			record.lastUserKind = "parts"
			var parts []struct {
				Type, Text string
				ImageURL   struct{ URL string } `json:"image_url"`
			}
			if json.Unmarshal(message.Content, &parts) != nil {
				t.Error("image provider parts malformed")
				return
			}
			for _, part := range parts {
				if part.Type == "text" {
					record.ownedPrompt = record.ownedPrompt || strings.Contains(part.Text, "image-owned-turn")
				}
				if part.Type == "image_url" {
					record.images++
					record.exactImage = part.ImageURL.URL == "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pixels.Bytes())
				}
			}
		}
		select {
		case requests <- record:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"image-owned-reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	config := "default_model = \"local/alpha\"\n[desktop]\nprovider_access = [\"local\"]\n" +
		"[network]\nproxy_mode = \"off\"\n[[providers]]\nname = \"local\"\nkind = \"openai\"\n" +
		"base_url = \"" + server.URL + "/v1\"\nmodels = [\"alpha\"]\ndefault = \"alpha\"\nvision_models = [\"alpha\"]\n"
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := appconfig.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.ResolveModel("local/alpha")
	if !ok || !appconfig.EffectiveVision(entry) {
		t.Fatal("explicit image fixture capability did not resolve")
	}
	runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: "global-image-capability"})
	if err != nil {
		t.Fatal(err)
	}
	actual := runtime.(*controllerRuntime)
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if !actual.controller.ImageInputEnabled() {
		t.Fatal("real Global controller lost explicit vision capability")
	}
	upload := filepath.Join(t.TempDir(), "clipboard.png")
	if err := os.WriteFile(upload, pixels.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	attachment, err := runtime.AttachFile(upload)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Submit("image-owned-turn\n\n@" + attachment.Path)
	var record observed
	select {
	case record = <-requests:
	case <-time.After(10 * time.Second):
		t.Fatal("real Global controller did not deliver image turn")
	}
	// Let the actual turn finish before shutdown/test-directory cleanup, even
	// when its request was wrong, so the test cannot race the transcript writer.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && runtime.State() != "idle" {
		time.Sleep(10 * time.Millisecond)
	}
	if runtime.State() != "idle" {
		t.Fatal("image turn did not settle before cleanup")
	}
	if record.images != 1 || !record.ownedPrompt || !record.exactImage {
		t.Fatalf("controller image delivery failed: imageCount=%d promptOwned=%t bytesEqual=%t lastUserKind=%s", record.images, record.ownedPrompt, record.exactImage, record.lastUserKind)
	}
}
