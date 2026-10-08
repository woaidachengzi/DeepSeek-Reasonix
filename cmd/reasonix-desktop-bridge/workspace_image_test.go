package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/desktopbridge"
)

func TestWorkspaceImageEndpointRequiresAuthorizationAndOwnedSession(t *testing.T) {
	runtime := &targetTestRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/tmp/image.jsonl", state: "idle"}, root: t.TempDir()}
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.root, "image.png"), pixels.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Shutdown() })
	bridge := newBridgeServer(testToken, "workspace-image-test", manager)
	request := func(id, body string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+id+":workspace-image", strings.NewReader(body))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if response := request("owned", `{"source":"image.png"}`, false); response.Code != http.StatusUnauthorized {
		t.Fatal(response.Code)
	}
	if response := request("other", `{"source":"image.png"}`, true); response.Code != http.StatusNotFound {
		t.Fatal(response.Code)
	}
	if response := request("owned", `{"source":"image.png","root":"/etc"}`, true); response.Code != http.StatusBadRequest {
		t.Fatal(response.Code)
	}
	response := request("owned", `{"source":"image.png"}`, true)
	var payload struct {
		ProtocolVersion int                              `json:"protocolVersion"`
		Image           desktopbridge.WorkspaceImageView `json:"image"`
	}
	if response.Code != http.StatusOK {
		t.Fatal(response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ProtocolVersion != 1 || !strings.HasPrefix(payload.Image.URL, "data:image/png;base64,") || payload.Image.Filename != "image.png" {
		t.Fatal("invalid image envelope")
	}
}
