package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Run the actual built .app sidecar, not an in-process controller stub. No GUI,
// system clipboard, external target, real credentials or user profile is used.
func TestWorkspaceImageActualPackageProxySmoke(t *testing.T) {
	binary := os.Getenv("REASONIX_IMAGE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires an explicitly selected packaged sidecar")
	}
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodConnect || r.Host != "93.184.216.34:80" {
			t.Error("package did not use pinned-IP proxy CONNECT")
			http.Error(w, "invalid", 400)
			return
		}
		conn, buffer, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error("proxy hijack failed")
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		request, err := http.ReadRequest(bufio.NewReader(buffer))
		if err != nil {
			t.Error("package image tunnel failed")
			return
		}
		defer request.Body.Close()
		for _, name := range []string{"Authorization", "Cookie", "Referer", "Proxy-Authorization"} {
			if request.Header.Get(name) != "" {
				t.Error("private header escaped packaged image transport", name)
			}
		}
		if request.URL.Path != "/owned.png" || request.Host != "93.184.216.34" {
			t.Error("package image target changed")
		}
		_, _ = fmt.Fprintf(buffer, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", pixels.Len())
		_, _ = buffer.Write(pixels.Bytes())
		_ = buffer.Flush()
	}))
	defer proxy.Close()
	var modelCalls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		var body struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body) != nil ||
			r.URL.Path != "/v1/chat/completions" || body.Model != "alpha" || !body.Stream {
			t.Error("packaged vision model fixture request invalid")
			http.Error(w, "invalid", 400)
			return
		}
		imageCount, owned, exact := 0, false, false
		for _, message := range body.Messages {
			if message.Role != "user" {
				continue
			}
			var text string
			if json.Unmarshal(message.Content, &text) == nil {
				owned = owned || strings.Contains(text, "image-package-owned-turn")
				continue
			}
			var parts []struct {
				Type, Text string
				ImageURL   struct{ URL string } `json:"image_url"`
			}
			if json.Unmarshal(message.Content, &parts) != nil {
				t.Error("package vision parts invalid")
				return
			}
			for _, part := range parts {
				owned = owned || (part.Type == "text" && strings.Contains(part.Text, "image-package-owned-turn"))
				if part.Type == "image_url" {
					imageCount++
					exact = part.ImageURL.URL == "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pixels.Bytes())
				}
			}
		}
		if imageCount != 1 || !owned || !exact {
			t.Error("package did not deliver exact owned image input")
			http.Error(w, "invalid image", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"package-image-accepted\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer model.Close()
	profile := t.TempDir()
	configuration := "default_model = \"local/alpha\"\n[desktop]\nprovider_access = [\"local\"]\n" +
		"[network]\nproxy_mode = \"custom\"\nproxy_url = \"" + proxy.URL + "\"\nno_proxy = \"127.0.0.1\"\n" +
		"[[providers]]\nname = \"local\"\nkind = \"openai\"\nbase_url = \"" + model.URL + "/v1\"\n" +
		"models = [\"alpha\"]\ndefault = \"alpha\"\nvision_models = [\"alpha\"]\n"
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(profile, "ready.json")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--ready-file", ready, "--launch-id", "image-package-smoke", "--host-pid", strconv.Itoa(os.Getpid()))
	cmd.Dir = profile
	// A minimal environment prevents inherited profile and provider overrides.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(),
		"REASONIX_HOME=" + profile, "REASONIX_STATE_HOME=" + profile, "REASONIX_CACHE_HOME=" + t.TempDir(), "REASONIX_CREDENTIALS_STORE=file"}
	cmd.Stdin = strings.NewReader(testToken + "\n")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal("packaged image sidecar failed to start")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			_ = cmd.Process.Kill() // only the child created above, never an existing app
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("packaged image child did not terminate")
			}
		}
	}()
	var address string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(ready)
		if err == nil {
			var doc struct {
				ProtocolVersion int    `json:"protocolVersion"`
				LaunchID        string `json:"launchId"`
				Address         string `json:"address"`
			}
			if json.Unmarshal(data, &doc) == nil && doc.ProtocolVersion == 1 && doc.LaunchID == "image-package-smoke" {
				host, _, err := net.SplitHostPort(doc.Address)
				if err == nil && net.ParseIP(host).IsLoopback() {
					address = doc.Address
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if address == "" {
		t.Fatal("packaged image sidecar did not publish owned loopback readiness")
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	sequence := 0
	callMethod := func(method, path, body string, authorized bool, status int) []byte {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, method, "http://"+address+path, strings.NewReader(body))
		if err != nil {
			t.Fatal("invalid package image fixture request")
		}
		sequence++
		request.Header.Set(requestIDHeader, fmt.Sprintf("image-package-%d", sequence))
		request.Header.Set("Content-Type", "application/json")
		if authorized {
			request.Header.Set("Authorization", "Bearer "+testToken)
		}
		reply, err := client.Do(request)
		if err != nil {
			t.Fatal("package image HTTP request failed")
		}
		defer reply.Body.Close()
		data, err := io.ReadAll(io.LimitReader(reply.Body, 1<<20))
		if err != nil || reply.StatusCode != status {
			t.Fatal("package image HTTP result differs", reply.StatusCode, status)
		}
		return data
	}
	call := func(path, body string, authorized bool, status int) []byte {
		return callMethod(http.MethodPost, path, body, authorized, status)
	}
	call("/v1/sessions:open", `{"sessionId":"image-package-owned"}`, true, 200)
	route := "/v1/sessions/image-package-owned:workspace-image"
	source := `{"source":"http://93.184.216.34/owned.png"}`
	call(route, source, false, 401)
	call("/v1/sessions/unowned:workspace-image", source, true, 404)
	data := call(route, source, true, 200)
	var result struct {
		Image struct {
			URL       string `json:"url"`
			OpenHref  string `json:"openHref"`
			ErrorCode string `json:"errorCode"`
		} `json:"image"`
	}
	if json.Unmarshal(data, &result) != nil || result.Image.ErrorCode != "" || !strings.HasPrefix(result.Image.URL, "data:image/png;base64,") || result.Image.OpenHref != "http://93.184.216.34/owned.png" || calls.Load() != 1 {
		t.Fatal("actual package did not return bounded proxy pixels")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.Image.URL, "data:image/png;base64,"))
	if err != nil {
		t.Fatal("packaged proxy returned invalid image encoding")
	}
	config, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || config.Width != 2 || config.Height != 2 {
		t.Fatal("packaged proxy returned unexpected image dimensions")
	}
	data = call(route, `{"source":"http://169.254.169.254/metadata"}`, true, 200)
	result.Image.URL, result.Image.OpenHref, result.Image.ErrorCode = "", "", ""
	if json.Unmarshal(data, &result) != nil || result.Image.URL != "" || result.Image.ErrorCode != "blocked-remote" || calls.Load() != 1 {
		t.Fatal("actual package allowed a private image target")
	}
	upload := filepath.Join(profile, "owned.png")
	if err := os.WriteFile(upload, pixels.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(map[string]string{"sessionId": "image-package-owned", "path": upload})
	var attachment struct {
		Attachment struct {
			Path string `json:"path"`
		} `json:"attachment"`
	}
	if json.Unmarshal(call("/v1/sessions/image-package-owned:attach", string(encoded), true, 201), &attachment) != nil || !strings.HasPrefix(attachment.Attachment.Path, ".reasonix/attachments/") {
		t.Fatal("actual package attachment copy failed")
	}
	input, _ := json.Marshal(map[string]string{"input": "image-package-owned-turn\n\n@" + attachment.Attachment.Path})
	call("/v1/sessions/image-package-owned:submit", string(input), true, 202)
	completed := false
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var history struct {
			Session struct {
				State string `json:"state"`
			} `json:"session"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if json.Unmarshal(callMethod(http.MethodGet, "/v1/sessions/image-package-owned/history", "", true, 200), &history) != nil {
			t.Fatal("invalid package image history")
		}
		for _, message := range history.Messages {
			if message.Role == "assistant" && strings.Contains(message.Content, "package-image-accepted") && history.Session.State == "idle" {
				completed = true
			}
		}
		if completed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !completed || modelCalls.Load() != 1 || calls.Load() != 1 {
		t.Fatal("package image reply was not persisted once, or proxy/no-proxy routing changed")
	}
	call("/v1:shutdown", `{}`, true, 202)
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatal("packaged image sidecar failed normal exit")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("packaged image sidecar did not exit")
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatal("packaged image sidecar leaked readiness")
	}
}
