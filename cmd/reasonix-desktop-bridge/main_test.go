package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
)

const testToken = "0123456789abcdef0123456789abcdef"

type bridgeTestRuntime struct {
	path          string
	state         string
	submits       []string
	cancelCalls   int
	shutdownCalls int
}

func (r *bridgeTestRuntime) SessionPath() string { return r.path }
func (r *bridgeTestRuntime) State() string       { return r.state }
func (r *bridgeTestRuntime) Submit(input string) { r.submits = append(r.submits, input) }
func (r *bridgeTestRuntime) Cancel()             { r.cancelCalls++ }
func (r *bridgeTestRuntime) Shutdown() error {
	r.shutdownCalls++
	return nil
}

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

func TestBridgeServerOpensSessionAndReturnsSnapshot(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		if request.SessionID != "tab-1" {
			t.Fatalf("unexpected session ID: %q", request.SessionID)
		}
		if request.WorkspaceRoot != "/workspace" {
			t.Fatalf("unexpected workspace root: %q", request.WorkspaceRoot)
		}
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1","workspaceRoot":"/workspace"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRequest.Header.Set("Content-Type", "application/json")
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	var openResponse struct {
		ProtocolVersion int                       `json:"protocolVersion"`
		Session         desktopbridge.SessionView `json:"session"`
	}
	if err := json.NewDecoder(openRecorder.Body).Decode(&openResponse); err != nil {
		t.Fatalf("decode open response: %v", err)
	}
	if openResponse.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("protocol version = %d", openResponse.ProtocolVersion)
	}
	if openResponse.Session.ID != "tab-1" || openResponse.Session.Path != runtime.path || openResponse.Session.State != "idle" {
		t.Fatalf("unexpected session: %#v", openResponse.Session)
	}

	snapshotRequest := httptest.NewRequest(http.MethodGet, "/v1/sessions/tab-1/snapshot", nil)
	snapshotRequest.Header.Set("Authorization", "Bearer "+testToken)
	snapshotRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(snapshotRecorder, snapshotRequest)
	if snapshotRecorder.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", snapshotRecorder.Code, snapshotRecorder.Body.String())
	}

	var snapshotResponse struct {
		ProtocolVersion int                       `json:"protocolVersion"`
		Sequence        uint64                    `json:"sequence"`
		Session         desktopbridge.SessionView `json:"session"`
	}
	if err := json.NewDecoder(snapshotRecorder.Body).Decode(&snapshotResponse); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	if snapshotResponse.ProtocolVersion != desktopbridge.ProtocolVersion || snapshotResponse.Sequence != 0 || snapshotResponse.Session.ID != "tab-1" {
		t.Fatalf("unexpected snapshot: %#v", snapshotResponse)
	}
}

func TestBridgeServerShutdownClosesOpenedRuntime(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	shutdownRequest := httptest.NewRequest(http.MethodPost, "/v1:shutdown", nil)
	shutdownRequest.Header.Set("Authorization", "Bearer "+testToken)
	shutdownRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(shutdownRecorder, shutdownRequest)
	if shutdownRecorder.Code != http.StatusAccepted {
		t.Fatalf("shutdown status = %d, body = %s", shutdownRecorder.Code, shutdownRecorder.Body.String())
	}
	if runtime.shutdownCalls != 1 {
		t.Fatalf("shutdown calls = %d, want 1", runtime.shutdownCalls)
	}
}

func TestBridgeServerSubmitsAndCancelsOpenedRuntime(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	bridge := newBridgeServer(testToken, "instance", manager)

	openRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1"}`))
	openRequest.Header.Set("Authorization", "Bearer "+testToken)
	openRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openRecorder.Code, openRecorder.Body.String())
	}

	submitRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:submit", strings.NewReader(`{"input":"hello"}`))
	submitRequest.Header.Set("Authorization", "Bearer "+testToken)
	submitRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body = %s", submitRecorder.Code, submitRecorder.Body.String())
	}
	if len(runtime.submits) != 1 || runtime.submits[0] != "hello" {
		t.Fatalf("submits = %#v", runtime.submits)
	}

	cancelRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:cancel", nil)
	cancelRequest.Header.Set("Authorization", "Bearer "+testToken)
	cancelRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d, body = %s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	if runtime.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d", runtime.cancelCalls)
	}
}

func TestBridgeServerRejectsMalformedAndUnknownCommandInput(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance")

	malformedRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"tab-1","unexpected":true}`))
	malformedRequest.Header.Set("Authorization", "Bearer "+testToken)
	malformedRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(malformedRecorder, malformedRequest)
	if malformedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, body = %s", malformedRecorder.Code, malformedRecorder.Body.String())
	}

	unknownRouteRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-1:unknown", nil)
	unknownRouteRequest.Header.Set("Authorization", "Bearer "+testToken)
	unknownRouteRecorder := httptest.NewRecorder()
	bridge.handler().ServeHTTP(unknownRouteRecorder, unknownRouteRequest)
	if unknownRouteRecorder.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, body = %s", unknownRouteRecorder.Code, unknownRouteRecorder.Body.String())
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
