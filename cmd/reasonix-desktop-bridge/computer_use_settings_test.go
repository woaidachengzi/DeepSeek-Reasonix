package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"reasonix/internal/computeruse"
	"reasonix/internal/desktopbridge"
)

func TestComputerUseStatusIsAuthenticatedAndReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	b := newBridgeServer(testToken, "computer-use-settings")
	const path = "/v1/settings/computer-use"
	unauthorized := httptest.NewRecorder()
	b.handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	b.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var view computerUseStatusView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ProtocolVersion != desktopbridge.ProtocolVersion || view.Platform != runtime.GOOS || view.Available != computeruse.Available() {
		t.Fatalf("unexpected native status: %+v", view)
	}
	if !view.Available && (view.Accessibility || view.ScreenRecording) {
		t.Fatalf("unsupported build claimed permission readiness: %+v", view)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("status changed the user profile: %v %v", entries, err)
	}
}
