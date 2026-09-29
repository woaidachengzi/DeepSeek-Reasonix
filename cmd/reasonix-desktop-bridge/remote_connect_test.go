package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
	"reasonix/internal/remote/sshtest"
)

func TestPreviewRemoteConnectRequiresAndPersistsExplicitHostKeyTrust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	t.Setenv("HOME", home)
	remoteRoot := filepath.Join(home, "sftp-root")
	if err := os.MkdirAll(filepath.Join(remoteRoot, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteRoot, "README.md"), []byte("remote workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := sshtest.Start(t, sshtest.Options{SFTPRoot: remoteRoot})
	address := strings.TrimPrefix(server.Addr, "127.0.0.1:")
	port, err := strconv.Atoi(address)
	if err != nil {
		t.Fatal(err)
	}
	configPath := configpkg.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "default_model = \"deepseek-flash\"\n\n[remote]\n\n[[remote.hosts]]\nname = \"loopback\"\nhost = \"127.0.0.1\"\nport = " + address + "\nuser = \"preview-test\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := newBridgeServer(testToken, "instance-a")
	t.Cleanup(bridge.remoteSessions.closeAll)
	request := func(trust string) remoteConnectResponse {
		t.Helper()
		body, err := json.Marshal(remoteConnectRequest{Name: "loopback", TrustFingerprint: trust})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/connect", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("connect status = %d: %s", response.Code, response.Body.String())
		}
		var result remoteConnectResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode connect response: %v", err)
		}
		return result
	}

	first := request("")
	if first.Status != "host_key_confirmation" || first.Fingerprint == "" || first.Address == "" || !strings.HasPrefix(first.Host, "preview-test@127.0.0.1") {
		t.Fatalf("first connect must request host-key confirmation, got %#v", first)
	}
	knownHosts := configpkg.RemoteKnownHostsPath()
	beforeTrust, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatalf("managed known_hosts was not initialized: %v", err)
	}
	if strings.Contains(string(beforeTrust), first.Fingerprint) {
		t.Fatal("unconfirmed host key was persisted early")
	}

	trusted := request(first.Fingerprint)
	if trusted.Status != "connected" || trusted.Fingerprint != first.Fingerprint {
		t.Fatalf("confirmed connection = %#v", trusted)
	}
	if _, err := os.Stat(knownHosts); err != nil {
		t.Fatalf("accepted host key was not persisted: %v", err)
	}
	connectedClient, _ := bridge.remoteSessions.get("loopback")
	workspaceBody, _ := json.Marshal(remoteSettingsChange{Action: "upsert", Host: remoteSettingsHostInput{
		Name: "loopback", Host: "127.0.0.1", Port: port, User: "preview-test",
		Workspace: "/selected/project", ServeInstall: "auto", CredentialMode: "remote",
	}})
	workspaceReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/hosts", strings.NewReader(string(workspaceBody)))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceReq.Header.Set("Authorization", "Bearer "+testToken)
	workspaceReq.Header.Set(requestIDHeader, "remote-workspace-update")
	workspaceResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(workspaceResponse, workspaceReq)
	if workspaceResponse.Code != http.StatusOK {
		t.Fatalf("save workspace path: %d %s", workspaceResponse.Code, workspaceResponse.Body.String())
	}
	if currentClient, _ := bridge.remoteSessions.get("loopback"); currentClient != connectedClient {
		t.Fatal("changing only the workspace path disconnected the live SSH client")
	}

	browseBody, _ := json.Marshal(remoteBrowseRequest{Name: "loopback", Path: "~"})
	browseReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/browse", strings.NewReader(string(browseBody)))
	browseReq.Header.Set("Content-Type", "application/json")
	browseReq.Header.Set("Authorization", "Bearer "+testToken)
	browseResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(browseResponse, browseReq)
	if browseResponse.Code != http.StatusOK {
		t.Fatalf("browse remote home: %d %s", browseResponse.Code, browseResponse.Body.String())
	}
	var listing remoteBrowseResponse
	if err := json.Unmarshal(browseResponse.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path == "" || listing.ParentPath == "" || len(listing.Entries) != 2 || !listing.Entries[0].IsDir || listing.Entries[1].Name != "README.md" {
		t.Fatalf("remote listing = %#v", listing)
	}

	disconnectBody, _ := json.Marshal(remoteDisconnectRequest{Name: "loopback"})
	disconnectReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/disconnect", strings.NewReader(string(disconnectBody)))
	disconnectReq.Header.Set("Content-Type", "application/json")
	disconnectReq.Header.Set("Authorization", "Bearer "+testToken)
	disconnectResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(disconnectResponse, disconnectReq)
	if disconnectResponse.Code != http.StatusOK {
		t.Fatalf("disconnect remote host: %d %s", disconnectResponse.Code, disconnectResponse.Body.String())
	}
	if client, _ := bridge.remoteSessions.get("loopback"); client != nil {
		t.Fatal("disconnect left an SSH client in the Preview session manager")
	}

	again := request("")
	if again.Status != "connected" || again.Fingerprint != first.Fingerprint {
		t.Fatalf("known host should reconnect without another prompt, got %#v", again)
	}
}
