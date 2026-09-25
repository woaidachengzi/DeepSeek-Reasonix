package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func TestSyncSessionCatalogRejectsControlCharactersWithoutMutation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript, err := sessionpath.TranscriptPath(sessionDir, "existing")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{
		ID: "existing", Path: transcript, WorkspaceRoot: "/original",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(syncSessionCatalogRequest{Sessions: []sessionidentity.WorkbenchOrderEntry{{
		ID: "existing", WorkspaceRoot: "/project\nother",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/sync-catalog", strings.NewReader(string(payload)))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("invalid workspace: status=%d body=%s", response.Code, response.Body.String())
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "existing")
	if err != nil || !exists || record.WorkspaceRoot != "/original" {
		t.Fatalf("identity mutated by rejected sync: %#v, exists=%v, err=%v", record, exists, err)
	}
}

func TestSyncSessionCatalogMirrorsOrderAndWorkspaceWithoutTouchingTitles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	for position, id := range []string{"first", "second", "hidden"} {
		path, err := sessionpath.TranscriptPath(sessionDir, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{
			ID: id, Path: path, WorkspaceRoot: "/old/project", Title: "Title " + id, Position: position,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", nil).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/sync-catalog", strings.NewReader(
		`{"sessions":[{"sessionId":"second","workspaceRoot":"/new/project"},{"sessionId":"first","workspaceRoot":""}]}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"synced":2`) {
		t.Fatalf("sync status=%d body=%s", response.Code, response.Body.String())
	}
	identities, err = sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identities.Close() }()
	records, err := identities.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].ID != "second" || records[0].WorkspaceRoot != "/new/project" ||
		records[0].Title != "Title second" || records[0].Position != 0 ||
		records[1].ID != "first" || records[1].WorkspaceRoot != "" || records[1].Position != 1 ||
		records[2].ID != "hidden" || records[2].WorkspaceRoot != "/old/project" || records[2].Position != 2 {
		t.Fatalf("synced session directory = %#v", records)
	}
}

func TestSyncSessionCatalogRejectsDuplicateEntriesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := sessionpath.TranscriptPath(sessionDir, "only")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{
		ID: "only", Path: path, WorkspaceRoot: "/before", Position: 0,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance", nil).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/sync-catalog", strings.NewReader(
		`{"sessions":[{"sessionId":"only","workspaceRoot":"/after"},{"sessionId":"only","workspaceRoot":"/again"}]}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate sync status=%d body=%s", response.Code, response.Body.String())
	}
	identities, err = sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identities.Close() }()
	record, exists, err := identities.Get(context.Background(), "only")
	if err != nil || !exists || record.WorkspaceRoot != "/before" || record.Position != 0 {
		t.Fatalf("rejected sync mutated identity: %#v exists=%v err=%v", record, exists, err)
	}
}
