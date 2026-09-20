package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestHealthRequiresToken(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHealthReturnsProtocolAndCapabilities(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got healthResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != desktopbridge.ProtocolVersion || got.Status != "ok" || got.SidecarInstanceID != "instance-a" {
		t.Fatalf("health = %#v", got)
	}
}

func TestShutdownIsAuthenticatedAndIdempotent(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-a")
	handler := bridge.handler()
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/v1:shutdown", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
		}
	}
	select {
	case <-bridge.shutdownRequested:
	default:
		t.Fatal("shutdown request was not signalled")
	}
}

func TestReadyFileDoesNotContainTokenAndIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ready.json")
	ready := readyFile{ProtocolVersion: desktopbridge.ProtocolVersion, Address: "127.0.0.1:12345", SidecarInstanceID: "instance-a", LaunchID: "launch-a"}
	if err := writeReadyFile(path, ready); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == testToken {
		t.Fatal("ready file contains token")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRequireLoopbackAddress(t *testing.T) {
	if err := requireLoopbackAddress("127.0.0.1:0"); err != nil {
		t.Fatalf("loopback address rejected: %v", err)
	}
	if err := requireLoopbackAddress("0.0.0.0:0"); err == nil {
		t.Fatal("wildcard address accepted")
	}
}

func TestRunPublishesReadyHealthAndShutdown(t *testing.T) {
	readyPath := filepath.Join(t.TempDir(), "ready.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, config{
			listen:    "127.0.0.1:0",
			readyFile: readyPath,
			launchID:  "test-launch",
		}, testToken)
	}()

	var ready readyFile
	deadline := time.Now().Add(5 * time.Second)
	for {
		content, err := os.ReadFile(readyPath)
		if err == nil && json.Unmarshal(content, &ready) == nil && ready.Address != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge did not publish ready file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ready.LaunchID != "test-launch" || ready.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("ready = %#v", ready)
	}

	healthRequest, err := http.NewRequest(http.MethodGet, "http://"+ready.Address+"/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	healthRequest.Header.Set("Authorization", "Bearer "+testToken)
	healthResponse, err := http.DefaultClient.Do(healthRequest)
	if err != nil {
		t.Fatal(err)
	}
	healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthResponse.StatusCode, http.StatusOK)
	}

	shutdownRequest, err := http.NewRequest(http.MethodPost, "http://"+ready.Address+"/v1:shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	shutdownRequest.Header.Set("Authorization", "Bearer "+testToken)
	shutdownResponse, err := http.DefaultClient.Do(shutdownRequest)
	if err != nil {
		t.Fatal(err)
	}
	shutdownResponse.Body.Close()
	if shutdownResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown status = %d, want %d", shutdownResponse.StatusCode, http.StatusAccepted)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("run returned %v", err)
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("ready file remained after shutdown: %v", err)
	}
}
