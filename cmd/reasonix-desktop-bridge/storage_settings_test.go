package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/desktopbridge"
)

func TestStorageSettingsReportEffectivePreviewDirectories(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", state)
	t.Setenv("REASONIX_CACHE_HOME", cache)
	bridge := newBridgeServer(testToken, "instance")
	request := httptest.NewRequest(http.MethodGet, "/v1/settings/storage", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("storage status = %d, body = %s", response.Code, response.Body.String())
	}
	var got previewStorageSettingsView
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != desktopbridge.ProtocolVersion || got.ProfilePath != home || got.StatePath != state || got.CachePath != cache || got.ExtensionsPath != filepath.Join(home, "plugins") {
		t.Fatalf("storage directories = %+v", got)
	}
	unauthorized := httptest.NewRecorder()
	bridge.handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/settings/storage", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized storage status = %d", unauthorized.Code)
	}
}
