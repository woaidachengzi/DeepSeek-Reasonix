package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	configpkg "reasonix/internal/config"
	"reasonix/internal/remote"
	"reasonix/internal/remote/sshtest"
)

func TestPreviewRemoteAuthPromptOverridesStoredCredentialOnlyForAttempt(t *testing.T) {
	passwordKey := "REASONIX_TEST_SSH_PASSWORD"
	passphraseKey := "REASONIX_TEST_SSH_PASSPHRASE"
	t.Setenv(passwordKey, "saved-password")
	t.Setenv(passphraseKey, "saved-passphrase")
	host := remote.ResolvedHost{PasswordEnv: passwordKey, PassphraseEnv: passphraseKey}

	auth := previewRemoteAuth(host, "temporary-password", "temporary-passphrase")
	password, err := auth.Password()
	if err != nil || password != "temporary-password" {
		t.Fatalf("temporary password override = %q, %v", password, err)
	}
	passphrase, err := auth.Passphrase()
	if err != nil || passphrase != "temporary-passphrase" {
		t.Fatalf("temporary passphrase override = %q, %v", passphrase, err)
	}

	storedAuth := previewRemoteAuth(host, "", "")
	password, err = storedAuth.Password()
	if err != nil || password != "saved-password" {
		t.Fatalf("stored password fallback = %q, %v", password, err)
	}
	passphrase, err = storedAuth.Passphrase()
	if err != nil || passphrase != "saved-passphrase" {
		t.Fatalf("stored passphrase fallback = %q, %v", passphrase, err)
	}
}

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
	previewBody, _ := json.Marshal(remotePreviewRequest{Name: "loopback", Path: listing.Entries[1].Path})
	previewReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/preview", strings.NewReader(string(previewBody)))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set("Authorization", "Bearer "+testToken)
	previewResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(previewResponse, previewReq)
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview remote text file: %d %s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview remotePreviewResponse
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "text" || preview.Content != "remote workspace" || preview.Truncated || preview.Path != listing.Entries[1].Path {
		t.Fatalf("remote file preview = %#v", preview)
	}
	if preview.Revision != remoteContentRevision([]byte("remote workspace")) {
		t.Fatalf("remote preview revision = %q", preview.Revision)
	}
	saveBody, _ := json.Marshal(remoteSaveRequest{Name: "loopback", Path: preview.Path, Revision: preview.Revision, Content: "updated remote text"})
	saveReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/save", strings.NewReader(string(saveBody)))
	saveReq.Header.Set("Content-Type", "application/json")
	saveReq.Header.Set("Authorization", "Bearer "+testToken)
	saveResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(saveResponse, saveReq)
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save remote text file: %d %s", saveResponse.Code, saveResponse.Body.String())
	}
	var saved remoteSaveResponse
	if err := json.Unmarshal(saveResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Path != preview.Path || saved.Revision != remoteContentRevision([]byte("updated remote text")) {
		t.Fatalf("remote save response = %#v", saved)
	}
	updatedFile, err := os.ReadFile(filepath.Join(remoteRoot, "README.md"))
	if err != nil || string(updatedFile) != "updated remote text" {
		t.Fatalf("remote file contents after save = %q, err=%v", updatedFile, err)
	}
	updatedInfo, err := os.Stat(filepath.Join(remoteRoot, "README.md"))
	if err != nil || updatedInfo.Mode().Perm() != 0o644 {
		t.Fatalf("remote file mode after save = %v, err=%v", updatedInfo, err)
	}
	staleBody, _ := json.Marshal(remoteSaveRequest{Name: "loopback", Path: preview.Path, Revision: preview.Revision, Content: "stale overwrite"})
	staleReq := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/save", strings.NewReader(string(staleBody)))
	staleReq.Header.Set("Content-Type", "application/json")
	staleReq.Header.Set("Authorization", "Bearer "+testToken)
	staleResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(staleResponse, staleReq)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale remote save status = %d, want 409: %s", staleResponse.Code, staleResponse.Body.String())
	}
	startSaves := make(chan struct{})
	saveResults := make(chan int, 2)
	for _, content := range []string{"parallel update A", "parallel update B"} {
		go func(content string) {
			<-startSaves
			body, _ := json.Marshal(remoteSaveRequest{Name: "loopback", Path: saved.Path, Revision: saved.Revision, Content: content})
			req := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/save", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testToken)
			response := httptest.NewRecorder()
			bridge.handler().ServeHTTP(response, req)
			saveResults <- response.Code
		}(content)
	}
	close(startSaves)
	firstSave, secondSave := <-saveResults, <-saveResults
	if !((firstSave == http.StatusOK && secondSave == http.StatusConflict) || (secondSave == http.StatusOK && firstSave == http.StatusConflict)) {
		t.Fatalf("concurrent saves accepted the same revision: statuses %d and %d", firstSave, secondSave)
	}
	previewBody, _ = json.Marshal(remotePreviewRequest{Name: "loopback", Path: listing.Path})
	previewReq = httptest.NewRequest(http.MethodPost, "/v1/settings/remote/preview", strings.NewReader(string(previewBody)))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set("Authorization", "Bearer "+testToken)
	previewResponse = httptest.NewRecorder()
	bridge.handler().ServeHTTP(previewResponse, previewReq)
	if previewResponse.Code != http.StatusBadRequest {
		t.Fatalf("directory preview status = %d, want 400: %s", previewResponse.Code, previewResponse.Body.String())
	}

	pathCall := func(input remotePathChangeRequest, id string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/remote/paths", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set(requestIDHeader, id)
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	created := path.Join(listing.Path, "created")
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "mkdir", Path: created}, "mkdir-created"); response.Code != http.StatusOK {
		t.Fatalf("create remote directory: %d %s", response.Code, response.Body.String())
	}
	if info, err := os.Stat(filepath.Join(remoteRoot, "created")); err != nil || !info.IsDir() {
		t.Fatalf("created directory missing: %v, %v", info, err)
	}
	renamed := path.Join(listing.Path, "renamed")
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "rename", Path: created, NewPath: renamed}, "rename-created"); response.Code != http.StatusOK {
		t.Fatalf("rename remote directory: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "renamed")); err != nil {
		t.Fatalf("renamed directory missing: %v", err)
	}
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "rename", Path: renamed, NewPath: preview.Path}, "rename-conflict"); response.Code != http.StatusConflict {
		t.Fatalf("rename over an existing remote path = %d, want 409: %s", response.Code, response.Body.String())
	}
	if err := os.WriteFile(filepath.Join(remoteRoot, "renamed", "child.txt"), []byte("remove me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "delete", Path: renamed}, "delete-nonempty"); response.Code != http.StatusConflict {
		t.Fatalf("nonrecursive remote directory delete = %d, want 409: %s", response.Code, response.Body.String())
	}
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "delete", Path: renamed, Recursive: true}, "delete-recursive"); response.Code != http.StatusOK {
		t.Fatalf("recursive remote directory delete: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "renamed")); !os.IsNotExist(err) {
		t.Fatalf("recursive remote delete left directory: %v", err)
	}
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "delete", Path: listing.Path, Recursive: true}, "delete-home"); response.Code != http.StatusBadRequest {
		t.Fatalf("remote home delete = %d, want 400: %s", response.Code, response.Body.String())
	}
	link := filepath.Join(remoteRoot, "project-link")
	if err := os.Symlink(filepath.Join(remoteRoot, "project"), link); err != nil {
		t.Fatal(err)
	}
	if response := pathCall(remotePathChangeRequest{Name: "loopback", Action: "delete", Path: path.Join(listing.Path, "project-link"), Recursive: true}, "delete-symlink"); response.Code != http.StatusOK {
		t.Fatalf("delete symlink: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink was not removed: %v", err)
	}
	if info, err := os.Stat(filepath.Join(remoteRoot, "project")); err != nil || !info.IsDir() {
		t.Fatalf("symlink target was deleted: %v", err)
	}

	// Real TCP round trip through SSH; only this machine may bind the forward.
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localPort := portProbe.Addr().(*net.TCPAddr).Port
	_ = portProbe.Close()
	forwardInput := remoteForwardRequest{Name: "loopback", Action: "add", ID: "test", LocalPort: localPort, RemoteHost: "127.0.0.1", RemotePort: target.Addr().(*net.TCPAddr).Port}
	forwardCall := func(input remoteForwardRequest, id string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/v1/settings/remote/forwards", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestIDHeader, id)
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	forwarded := forwardCall(forwardInput, "forward-add")
	if forwarded.Code != 200 {
		t.Fatalf("add forward: %d %s", forwarded.Code, forwarded.Body.String())
	}
	var forwardView remoteForwardsView
	if err := json.Unmarshal(forwarded.Body.Bytes(), &forwardView); err != nil || len(forwardView.Forwards) != 1 || !forwardView.Forwards[0].Active || !strings.HasPrefix(forwardView.Forwards[0].LocalAddress, "127.0.0.1:") {
		t.Fatalf("forward view: %#v %v", forwardView, err)
	}
	replay := forwardCall(forwardInput, "forward-add")
	if replay.Code != 200 || replay.Body.String() != forwarded.Body.String() {
		t.Fatal("forward idempotency replay changed")
	}
	if duplicate := forwardCall(forwardInput, "forward-duplicate"); duplicate.Code != 409 {
		t.Fatal("duplicate forward accepted")
	}
	invalid := forwardInput
	invalid.ID = "other"
	invalid.LocalPort = 0
	if rejected := forwardCall(invalid, "invalid-forward"); rejected.Code != 400 {
		t.Fatal("invalid local port accepted")
	}
	tunnel, err := net.DialTimeout("tcp", forwardView.Forwards[0].LocalAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = tunnel.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = tunnel.Write([]byte("ping"))
	reply := make([]byte, 4)
	_, err = io.ReadFull(tunnel, reply)
	_ = tunnel.Close()
	if err != nil || string(reply) != "ping" {
		t.Fatalf("forward round trip: %q %v", reply, err)
	}
	// Configuration reads carry the real connection state after reopening UI.
	settingsReq := httptest.NewRequest("GET", "/v1/settings/remote", nil)
	settingsReq.Header.Set("Authorization", "Bearer "+testToken)
	settingsResp := httptest.NewRecorder()
	bridge.handler().ServeHTTP(settingsResp, settingsReq)
	var liveSettings remoteSettingsView
	_ = json.Unmarshal(settingsResp.Body.Bytes(), &liveSettings)
	if len(liveSettings.Hosts) != 1 || liveSettings.Hosts[0].Connection == nil || liveSettings.Hosts[0].Connection.Status != "connected" {
		t.Fatal("live state missing on settings reopen")
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

	if listener, err := net.Listen("tcp", forwardView.Forwards[0].LocalAddress); err != nil {
		t.Fatal("disconnect leaked forwarded port")
	} else {
		_ = listener.Close()
	}

	again := request("")
	if again.Status != "connected" || again.Fingerprint != first.Fingerprint {
		t.Fatalf("known host should reconnect without another prompt, got %#v", again)
	}
}
