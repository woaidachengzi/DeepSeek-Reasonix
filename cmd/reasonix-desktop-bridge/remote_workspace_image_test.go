package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/desktopbridge"
)

func TestWorkspaceImageEndpointUsesActualConfiguredProxyAndBoundedPixels(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodConnect || r.Host != "93.184.216.34:80" {
			t.Error("proxy did not receive a vetted-IP CONNECT")
			http.Error(w, "invalid", 400)
			return
		}
		conn, buffer, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		request, err := http.ReadRequest(bufio.NewReader(buffer))
		if err != nil {
			t.Error(err)
			return
		}
		defer request.Body.Close()
		if request.Header.Get("Authorization") != "" || request.URL.Path != "/safe.png" {
			t.Error("bridge credentials leaked or target differs")
		}
		_, _ = fmt.Fprintf(buffer, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", pixels.Len())
		_, _ = buffer.Write(pixels.Bytes())
		_ = buffer.Flush()
	}))
	defer proxy.Close()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[network]\nproxy_mode = \"custom\"\nproxy_url = \""+proxy.URL+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &targetTestRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/tmp/image-proxy.jsonl", state: "idle"}, root: t.TempDir()}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Shutdown() })
	bridge := newBridgeServer(testToken, "image-proxy", manager)
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/owned:workspace-image", strings.NewReader(`{"source":"http://93.184.216.34/safe.png"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	reply := httptest.NewRecorder()
	bridge.handler().ServeHTTP(reply, request)
	var envelope struct {
		Image desktopbridge.WorkspaceImageView `json:"image"`
	}
	if reply.Code != 200 {
		t.Fatal(reply.Code)
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || envelope.Image.ErrorCode != "" || !strings.HasPrefix(envelope.Image.URL, "data:image/png;base64,") {
		t.Fatal("configured proxy/image path failed", calls.Load(), envelope.Image.ErrorCode)
	}
}
