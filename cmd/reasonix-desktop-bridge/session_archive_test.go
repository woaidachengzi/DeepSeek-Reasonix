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

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

func TestSessionArchiveHTTPAuthPersistenceArtifactsAndRecovery(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	dir := appconfig.SessionDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, _ := bridgeSessionPath(dir, "first")
	before := []byte("private transcript contents")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := strings.TrimSuffix(path, ".jsonl") + ".ckpt"
	if err := os.Mkdir(artifact, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "checkpoint"), []byte("keep checkpoint"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := ids.ImportLegacyCatalog(context.Background(), dir, []sessionidentity.Candidate{{ID: "first", Path: path, Title: "Saved conversation", WorkspaceRoot: "/work"}}); err != nil {
		t.Fatal(err)
	}
	ids.Close()
	call := func(h http.Handler, method, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/sessions/archives", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	h := newBridgeServer(testToken, "one").handler()
	request := `{"sessionId":"first","archived":true}`
	if w := call(h, "POST", request, "wrong"); w.Code != 401 {
		t.Fatal("archive accepted unauthorized request", w.Code)
	}
	lease, err := agent.TryAcquireSessionLease(path)
	if err != nil {
		t.Fatal(err)
	}
	if w := call(h, "POST", request, testToken); w.Code != 409 {
		t.Fatal("archive accepted another writer", w.Code, w.Body.String())
	}
	lease.Release()
	if w := call(h, "POST", `{"sessionId":"../outside","archived":true}`, testToken); w.Code != 400 {
		t.Fatal("path identifier accepted")
	}
	if w := call(h, "POST", request, testToken); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if data, _ := os.ReadFile(path); string(data) != string(before) {
		t.Fatal("archive modified transcript")
	}
	if data, _ := os.ReadFile(filepath.Join(artifact, "checkpoint")); string(data) != "keep checkpoint" {
		t.Fatal("checkpoint lost")
	}
	if err := (&controllerFactory{}).ValidateOpen(desktopbridge.OpenRequest{SessionID: "first"}); err == nil {
		t.Fatal("archived session admission accepted")
	}
	restarted := newBridgeServer(testToken, "two").handler()
	w := call(restarted, "GET", "", testToken)
	var view sessionArchiveResponse
	if json.Unmarshal(w.Body.Bytes(), &view) != nil || len(view.Sessions) != 1 || strings.Contains(w.Body.String(), string(before)) {
		t.Fatal("restart lost archive or leaked contents", w.Body.String())
	}
	if w := call(restarted, "POST", `{"sessionId":"first","archived":false}`, testToken); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := (&controllerFactory{}).ValidateOpen(desktopbridge.OpenRequest{SessionID: "first"}); err != nil {
		t.Fatal("restored session blocked", err)
	}
	if err := os.WriteFile(sessionArchivePath(), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := call(restarted, "POST", request, testToken); w.Code != 500 {
		t.Fatal("corrupt archive silently repaired", w.Code)
	}
	if data, _ := os.ReadFile(sessionArchivePath()); string(data) != "bad" {
		t.Fatal("corruption overwritten")
	}
}
