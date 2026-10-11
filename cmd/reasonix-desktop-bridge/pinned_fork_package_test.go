package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

// Uses the release sidecar inside the built App, with a disposable profile.
// No account credentials, live conversations or external providers are used.
func TestPinnedForkActualPackageRestore(t *testing.T) {
	binary := os.Getenv("REASONIX_E_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires the sidecar from the actual built .app")
	}
	profile, workspace := t.TempDir(), t.TempDir()
	for name, value := range map[string]string{"REASONIX_HOME": profile, "REASONIX_STATE_HOME": profile, "REASONIX_CACHE_HOME": t.TempDir(), "HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file"} {
		t.Setenv(name, value)
	}
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	dir := appconfig.SessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	parent := "tauri-package-parent"
	parentPath, err := bridgeSessionPath(dir, parent)
	if err != nil {
		t.Fatal(err)
	}
	transcript := "{\"role\":\"system\",\"content\":\"base\"}\n{\"role\":\"user\",\"content\":\"first question\"}\n{\"role\":\"assistant\",\"content\":\"first answer\"}\n{\"role\":\"user\",\"content\":\"later question\"}\n{\"role\":\"assistant\",\"content\":\"later answer\"}\n"
	if err := os.WriteFile(parentPath, []byte(transcript), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("private package context"), 0600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	err = identities.ImportLegacyCatalog(context.Background(), dir, []sessionidentity.Candidate{{ID: parent, Path: parentPath, WorkspaceRoot: workspace, Title: "package parent"}})
	identities.Close()
	if err != nil {
		t.Fatal(err)
	}
	var cmd *exec.Cmd
	var base string
	sequence := 0
	start := func() {
		t.Helper()
		sequence++
		ready := filepath.Join(profile, fmt.Sprintf("ready-%d.json", sequence))
		log, err := os.Create(filepath.Join(profile, fmt.Sprintf("sidecar-%d.log", sequence)))
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(binary, "--ready-file", ready, "--launch-id", "pinned-fork-package", "--host-pid", strconv.Itoa(os.Getpid()))
		child.Dir, child.Env, child.Stdin, child.Stdout, child.Stderr = profile, os.Environ(), strings.NewReader(testToken+"\n"), log, log
		if err := child.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		cmd = child
		t.Cleanup(func() {
			if child.ProcessState == nil {
				_ = child.Process.Kill()
				_ = child.Wait()
			}
			_ = log.Close()
		})
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if raw, err := os.ReadFile(ready); err == nil {
				var response struct {
					Address string `json:"address"`
				}
				if json.Unmarshal(raw, &response) == nil && response.Address != "" {
					base = "http://" + response.Address
					return
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		raw, _ := os.ReadFile(log.Name())
		t.Fatalf("packaged sidecar did not become ready: %s", raw)
	}
	requestNumber := 0
	call := func(method, route string, body any, requestID string) []byte {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequest(method, base+route, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		requestNumber++
		if requestID == "" {
			requestID = fmt.Sprintf("package-pinned-%d", requestNumber)
		}
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(requestIDHeader, requestID)
		client := &http.Client{Timeout: 120 * time.Second}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
			t.Fatalf("%s %s: %d %s", method, route, response.StatusCode, raw)
		}
		return raw
	}
	stop := func() {
		t.Helper()
		call("POST", "/v1:shutdown", map[string]any{}, "")
		done := make(chan error, 1)
		go func(child *exec.Cmd) { done <- child.Wait() }(cmd)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("package failed to exit normally")
		}
	}
	open := func(id string) {
		call("POST", "/v1/sessions:switch", map[string]any{"sessionId": id, "workspaceRoot": workspace}, "")
	}
	checkPins := func(id string, count int) {
		t.Helper()
		var response pinnedFilesResponse
		if err := json.Unmarshal(call("GET", "/v1/sessions/"+id+"/pinned-files", nil, ""), &response); err != nil || response.SessionID != id || len(response.Files) != count {
			t.Fatalf("pins: %+v %v", response, err)
		}
	}
	checkHistory := func(id string, count int) desktopbridge.HistoryView {
		t.Helper()
		var response desktopbridge.HistoryView
		if err := json.Unmarshal(call("GET", "/v1/sessions/"+id+"/history", nil, ""), &response); err != nil || response.Session.ID != id || len(response.Messages) != count {
			t.Fatalf("history: %+v %v", response, err)
		}
		return response
	}
	start()
	open(parent)
	call("POST", "/v1/sessions/"+parent+":pin-file", map[string]any{"path": "notes.txt"}, "pin-parent")
	checkPins(parent, 1)
	history := checkHistory(parent, 4)
	if history.Messages[1].ForkToken == "" {
		t.Fatal("packaged history omitted fork token")
	}
	before, err := os.ReadFile(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"forkToken": history.Messages[1].ForkToken}
	reply := call("POST", "/v1/sessions/"+parent+":fork", body, "fork-once")
	if !bytes.Equal(reply, call("POST", "/v1/sessions/"+parent+":fork", body, "fork-once")) {
		t.Fatal("fork retry created a second child")
	}
	var fork legacyConversationForkResultResponse
	if err := json.Unmarshal(reply, &fork); err != nil || !fork.Result.OK || fork.Result.SessionID == "" {
		t.Fatalf("fork: %+v %v", fork, err)
	}
	after, err := os.ReadFile(parentPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("fork changed the source transcript")
	}
	child := fork.Result.SessionID
	open(child)
	checkPins(child, 1)
	childHistory := checkHistory(child, 2)
	if childHistory.Messages[1].Content != "first answer" {
		t.Fatal("fork included a later answer")
	}
	call("POST", "/v1/sessions/"+child+":unpin-file", map[string]any{"path": "notes.txt"}, "")
	checkPins(child, 0)
	open(parent)
	checkPins(parent, 1)
	checkHistory(parent, 4)
	stop()
	start()
	open(child)
	checkHistory(child, 2)
	checkPins(child, 0)
	open(parent)
	checkHistory(parent, 4)
	checkPins(parent, 1)
	if err := os.Remove(filepath.Join(workspace, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	var unavailable pinnedFilesResponse
	if err := json.Unmarshal(call("GET", "/v1/sessions/"+parent+"/pinned-files", nil, ""), &unavailable); err != nil || len(unavailable.Files) != 1 || unavailable.Files[0].Error == "" {
		t.Fatalf("unavailable pins: %+v %v", unavailable, err)
	}
	call("POST", "/v1/sessions/"+parent+":unpin-file", map[string]any{"path": "notes.txt"}, "")
	checkPins(parent, 0)
	stop()
}
