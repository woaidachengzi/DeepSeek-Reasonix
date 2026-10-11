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
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/pinnedcontext"
)

func TestPinnedFilesHTTPRoutesRequireOwnershipAndReplayMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("context"), 0600); err != nil {
		t.Fatal(err)
	}
	exec := agent.New(nil, nil, agent.NewSession("base"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Executor: exec, Runner: exec, Sink: event.Discard, SessionPath: path, WorkspaceRoot: root})
	defer ctrl.Close()
	runtime := &controllerRuntime{controller: ctrl}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "owned", WorkspaceRoot: root}); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "pinned-test", manager).handler()
	call := func(method, route, body, requestID string, authorized bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, route, strings.NewReader(body))
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		if requestID != "" {
			r.Header.Set(requestIDHeader, requestID)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", route, w.Code, w.Body.String())
		}
		return w
	}
	call("GET", "/v1/sessions/owned/pinned-files", "", "", false, http.StatusUnauthorized)
	call("GET", "/v1/sessions/other/pinned-files", "", "", true, http.StatusNotFound)
	call("POST", "/v1/sessions/owned:pin-file", `{"path":"notes.txt"}`, "", true, http.StatusBadRequest)
	first := call("POST", "/v1/sessions/owned:pin-file", `{"path":"notes.txt"}`, "pin-1", true, http.StatusOK)
	call("POST", "/v1/sessions/owned:unpin-file", `{"path":"notes.txt"}`, "unpin-1", true, http.StatusOK)
	replay := call("POST", "/v1/sessions/owned:pin-file", `{"path":"notes.txt"}`, "pin-1", true, http.StatusOK)
	if first.Body.String() != replay.Body.String() {
		t.Fatal("mutation response was not replayed")
	}
	listed := call("GET", "/v1/sessions/owned/pinned-files", "", "", true, http.StatusOK)
	var response pinnedFilesResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &response); err != nil || response.SessionID != "owned" || response.Files == nil || len(response.Files) != 0 {
		t.Fatalf("replayed mutation repinned file: %+v %v", response, err)
	}
	call("POST", "/v1/sessions/owned:pin-file", `{"path":"notes.txt"}`, "pin-2", true, http.StatusOK)
	listed = call("GET", "/v1/sessions/owned/pinned-files", "", "", true, http.StatusOK)
	if err := json.Unmarshal(listed.Body.Bytes(), &response); err != nil || len(response.Files) != 1 {
		t.Fatalf("fresh mutation: %+v %v", response, err)
	}
}

func TestRuntimePinnedFilesRestoreAndFeedTurnLoader(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("original content"), 0600); err != nil {
		t.Fatal(err)
	}
	newRuntime := func() *controllerRuntime {
		exec := agent.New(nil, nil, agent.NewSession("base"), agent.Options{}, event.Discard)
		ctrl := control.New(control.Options{Executor: exec, Runner: exec, Sink: event.Discard, SessionPath: path, WorkspaceRoot: root})
		t.Cleanup(ctrl.Close)
		return &controllerRuntime{controller: ctrl}
	}
	runtime := newRuntime()
	if files, err := runtime.PinnedFiles(); err != nil || files == nil || len(files) != 0 {
		t.Fatalf("initial: %+v %v", files, err)
	}
	if files, err := runtime.ChangePinnedFile("notes.txt", true); err != nil || len(files) != 1 {
		t.Fatalf("pin: %+v %v", files, err)
	}
	restored := newRuntime()
	if files, err := restored.PinnedFiles(); err != nil || len(files) != 1 || files[0].Path != "notes.txt" {
		t.Fatalf("restored: %+v %v", files, err)
	}
	load := pinnedcontext.Loader(root, nil)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("changed content"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := load(context.Background(), path)
	if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Content != "changed content" {
		t.Fatalf("fresh context: %+v %v", snapshot, err)
	}
	if err := os.Remove(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if files, err := restored.PinnedFiles(); err != nil || len(files) != 1 || files[0].Error == "" {
		t.Fatalf("missing: %+v %v", files, err)
	}
	if _, err := restored.ChangePinnedFile("notes.txt", false); err != nil {
		t.Fatal(err)
	}
	snapshot, err = load(context.Background(), path)
	if err != nil || len(snapshot.Files) != 0 || len(snapshot.Issues) != 0 {
		t.Fatalf("revoked: %+v %v", snapshot, err)
	}
}
