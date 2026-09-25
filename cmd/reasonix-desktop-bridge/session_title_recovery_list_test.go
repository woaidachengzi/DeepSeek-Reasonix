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
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func TestPendingSessionTitleRecoveriesExposeOldCatalogIndependentEntry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "older-than-recent-catalog"
	path, err := sessionpath.TranscriptPath(sessionDir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "project")
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{
		ID: id, Path: path, Title: "Before rename", WorkspaceRoot: workspaceRoot,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginManualTitleRename(ctx, id, path, "", "After rename", 0); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", desktopbridge.NewRuntimeManager(newControllerFactory(nil))).handler()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/title-recovery", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	read := func() pendingSessionTitleRecoveriesResponse {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("title recovery status=%d body=%s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), path) || strings.Contains(response.Body.String(), `"path"`) ||
			strings.Contains(response.Body.String(), "After rename") {
			t.Fatalf("title recovery response leaked transcript or uncommitted title: %s", response.Body.String())
		}
		var body pendingSessionTitleRecoveriesResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	body := read()
	if body.ProtocolVersion != desktopbridge.ProtocolVersion || len(body.Sessions) != 1 ||
		body.Sessions[0].ID != id || body.Sessions[0].Title != "Before rename" ||
		body.Sessions[0].WorkspaceRoot != workspaceRoot || body.Sessions[0].State != sessionidentity.StateReady {
		t.Fatalf("pending title recovery = %#v", body)
	}
	if err := agent.RenameSession(path, "After rename"); err != nil {
		t.Fatal(err)
	}
	identities, err = sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := recoverBridgeManualTitleRename(ctx, identities, id, path); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if after := read(); len(after.Sessions) != 0 {
		t.Fatalf("completed title recovery remains pending: %#v", after.Sessions)
	}
}
