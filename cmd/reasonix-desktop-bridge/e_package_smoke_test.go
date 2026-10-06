package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/remote/sshtest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in smoke of the actual binary extracted from a built .app. All profile,
// credentials, SSH and bot identities are disposable; no external IM is used.
func TestEActualPackageLifecycleSmoke(t *testing.T) {
	binary := os.Getenv("REASONIX_E_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires the actual built .app sidecar")
	}
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	sshRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sshRoot, "sample.txt"), []byte("package fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	sshServer := sshtest.Start(t, sshtest.Options{SFTPRoot: sshRoot})
	_, portText, _ := net.SplitHostPort(sshServer.Addr)
	sshPort, _ := strconv.Atoi(portText)
	type running struct {
		cmd   *exec.Cmd
		base  string
		ready string
		log   *os.File
	}
	var process *running
	readyCounter := 0
	start := func(bin string) {
		t.Helper()
		readyCounter++
		ready := filepath.Join(profile, fmt.Sprintf("ready-%d.json", readyCounter))
		log, err := os.Create(filepath.Join(profile, fmt.Sprintf("sidecar-%d.log", readyCounter)))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "--ready-file", ready, "--launch-id", "package-e-smoke", "--host-pid", strconv.Itoa(os.Getpid()))
		cmd.Dir = profile
		cmd.Env = os.Environ()
		cmd.Stdin = strings.NewReader(testToken + "\n")
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		process = &running{cmd: cmd, ready: ready, log: log}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			_ = log.Close()
		})
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(ready); err == nil {
				var doc struct {
					Address string `json:"address"`
				}
				if json.Unmarshal(data, &doc) == nil && doc.Address != "" {
					process.base = "http://" + doc.Address
					return
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal("packaged sidecar did not become ready")
	}
	requestCounter := 0
	call := func(method, path string, body any, status int, authorized bool) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req, err := http.NewRequest(method, process.base+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		requestCounter++
		req.Header.Set(requestIDHeader, fmt.Sprintf("package-%d", requestCounter))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		client := http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		reply, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("%s %s: status %d expected %d; %s", method, path, resp.StatusCode, status, reply)
		}
		if bytes.Contains(reply, []byte("unused-package-secret")) {
			t.Fatal("credential leaked through management response")
		}
		return reply
	}
	stop := func() {
		t.Helper()
		call("POST", "/v1:shutdown", map[string]any{}, 202, true)
		done := make(chan error, 1)
		go func() { done <- process.cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("sidecar failed to exit")
		}
		if _, err := os.Stat(process.ready); !os.IsNotExist(err) {
			t.Fatal("shutdown leaked readiness file")
		}
		_ = process.log.Close()
	}
	start(binary)
	call("GET", "/v1/settings/bots/pairing", nil, 401, false)
	call("POST", "/v1/settings/remote/forwards", map[string]any{"name": "fixture", "action": "list"}, 401, false)
	call("POST", "/v1/settings/remote/hosts", remoteSettingsChange{Action: "upsert", Host: remoteSettingsHostInput{Name: "fixture", Host: "127.0.0.1", Port: sshPort, User: "fixture", ServeInstall: "auto", CredentialMode: "remote"}}, 200, true)
	var trust remoteConnectResponse
	_ = json.Unmarshal(call("POST", "/v1/settings/remote/connect", remoteConnectRequest{Name: "fixture"}, 200, true), &trust)
	if trust.Status != "host_key_confirmation" || trust.Fingerprint == "" {
		t.Fatal("first connection bypassed explicit trust")
	}
	call("POST", "/v1/settings/remote/connect", remoteConnectRequest{Name: "fixture", TrustFingerprint: trust.Fingerprint}, 200, true)
	var listing remoteBrowseResponse
	_ = json.Unmarshal(call("POST", "/v1/settings/remote/browse", remoteBrowseRequest{Name: "fixture", Path: "~"}, 200, true), &listing)
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "sample.txt" {
		t.Fatal("packaged SFTP browse failed")
	}
	var preview remotePreviewResponse
	_ = json.Unmarshal(call("POST", "/v1/settings/remote/preview", remotePreviewRequest{Name: "fixture", Path: listing.Entries[0].Path}, 200, true), &preview)
	call("POST", "/v1/settings/remote/save", remoteSaveRequest{Name: "fixture", Path: preview.Path, Revision: preview.Revision, Content: "package changed"}, 200, true)
	call("POST", "/v1/settings/remote/save", remoteSaveRequest{Name: "fixture", Path: preview.Path, Revision: preview.Revision, Content: "stale"}, 409, true)
	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localPort := portProbe.Addr().(*net.TCPAddr).Port
	_ = portProbe.Close()
	call("POST", "/v1/settings/remote/forwards", remoteForwardRequest{Name: "fixture", Action: "add", ID: "dev", LocalPort: localPort, RemoteHost: "127.0.0.1", RemotePort: sshPort}, 200, true)
	localAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort))
	tunnel, err := net.DialTimeout("tcp", localAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = tunnel.SetReadDeadline(time.Now().Add(3 * time.Second))
	banner := make([]byte, 64)
	n, err := tunnel.Read(banner)
	_ = tunnel.Close()
	if err != nil || !strings.HasPrefix(string(banner[:n]), "SSH-") {
		t.Fatal("forward failed actual SSH transport")
	}
	call("POST", "/v1/settings/bots", botSettingsChange{Action: "create_connection", Connection: &botConnectionInput{ID: "package-work", Platform: "feishu", Domain: "feishu", Label: "Package fixture", Identity: "unused-app-id", Secret: "unused-package-secret"}}, 200, true)
	pairing, _, err := bot.CreateOrRefreshPairingRequest(bot.InboundMessage{Platform: bot.PlatformFeishu, Domain: "feishu", ConnectionID: "package-work", ChatType: bot.ChatDM, ChatID: "chat", UserID: "fixture-user"}, bot.PairingConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	call("GET", "/v1/settings/bots/pairing", nil, 200, true)
	call("POST", "/v1/settings/bots/pairing", botPairingChange{Action: "approve", Code: pairing.Code}, 200, true)
	stored, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Bot.Connections) != 1 || stored.Bot.Connections[0].Enabled || len(stored.Bot.Connections[0].Access.Users) != 1 || len(stored.Bot.Allowlist.FeishuUsers) != 0 {
		t.Fatal("pairing changed the wrong scope or enabled the bot")
	}
	for _, file := range []string{appconfig.UserConfigPath(), filepath.Join(profile, ".env"), bot.PairingStorePath()} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("%s permissions missing or too broad: %v", filepath.Base(file), err)
		}
	}
	call("POST", "/v1/settings/bots/runtime", botRuntimeActionRequest{Action: "restart"}, 202, true)
	stop()
	if listener, err := net.Listen("tcp", localAddress); err != nil {
		t.Fatal("package shutdown leaked forwarded port")
	} else {
		_ = listener.Close()
	}
	start(binary)
	var settings botSettingsView
	_ = json.Unmarshal(call("GET", "/v1/settings/bots", nil, 200, true), &settings)
	found := false
	for _, channel := range settings.Channels {
		if channel.ID == "package-work" && channel.CredentialsSet && !channel.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("restart lost connection or its local credential")
	}
	var reconnect remoteConnectResponse
	_ = json.Unmarshal(call("POST", "/v1/settings/remote/connect", remoteConnectRequest{Name: "fixture"}, 200, true), &reconnect)
	if reconnect.Status != "connected" {
		t.Fatal("restart lost persisted host trust")
	}
	var forwards remoteForwardsView
	_ = json.Unmarshal(call("POST", "/v1/settings/remote/forwards", remoteForwardRequest{Name: "fixture", Action: "list"}, 200, true), &forwards)
	if len(forwards.Forwards) != 0 {
		t.Fatal("session forward unexpectedly survived process exit")
	}
	stop()
	rollback := os.Getenv("REASONIX_E_ROLLBACK_BIN")
	if rollback != "" {
		start(rollback)
		call("GET", "/v1/settings/bots", nil, 200, true)
		call("GET", "/v1/settings/remote", nil, 200, true)
		stop()
		start(binary)
		call("POST", "/v1/settings/bots", botSettingsChange{Action: "remove_connection", ChannelID: "package-work"}, 200, true)
		stop()
	}
	bytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes)
	t.Logf("actual packaged sidecar sha256=%s; restart/SSH trust/SFTP revision/loopback tunnel/pairing scope/auth/0600/exit passed; rollback=%t", hex.EncodeToString(sum[:]), rollback != "")
}
