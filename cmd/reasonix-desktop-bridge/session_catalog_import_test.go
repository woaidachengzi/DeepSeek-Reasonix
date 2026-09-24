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

func TestImportLegacyCatalogPreservesExistingAndMissingEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	existingPath, err := sessionpath.TranscriptPath(sessionDir, "legacy-existing")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{
		ID: "legacy-existing", Path: existingPath, Title: "New title", Position: 5,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", nil).handler()
	body := `{"sessions":[{"sessionId":"legacy-existing","title":"Stale title","workspaceRoot":"/work/project"},{"sessionId":"legacy-missing","title":"Missing title"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/import-catalog", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("catalog import status=%d body=%s", response.Code, response.Body.String())
	}
	var result importLegacyCatalogResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Accepted != 2 || result.ProtocolVersion != 1 {
		t.Fatalf("catalog import response=%#v err=%v", result, err)
	}
	identities, err = sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identities.Close() }()
	existing, ok, err := identities.Get(context.Background(), "legacy-existing")
	if err != nil || !ok || existing.Title != "New title" || existing.Position != 5 || existing.WorkspaceRoot != "" {
		t.Fatalf("existing identity was overwritten: %#v ok=%v err=%v", existing, ok, err)
	}
	missing, ok, err := identities.Get(context.Background(), "legacy-missing")
	if err != nil || !ok || missing.State != sessionidentity.StateMissing || missing.Title != "Missing title" {
		t.Fatalf("missing legacy identity was dropped: %#v ok=%v err=%v", missing, ok, err)
	}
	if strings.Contains(response.Body.String(), existingPath) {
		t.Fatalf("catalog import response leaked a transcript path: %s", response.Body.String())
	}
}

func TestImportLegacyCatalogRequiresAuthorizationAndBoundedInput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	handler := newBridgeServer(testToken, "instance", nil).handler()
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/sessions/import-catalog", strings.NewReader(`{"sessions":[]}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized import status=%d", unauthorized.Code)
	}
	entries := make([]string, 51)
	for i := range entries {
		entries[i] = `{"sessionId":"session-` + string(rune('a'+i%26)) + `"}`
	}
	body := `{"sessions":[` + strings.Join(entries, ",") + `]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/import-catalog", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized import status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(appconfig.DesktopSessionIdentityPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected import created identity store: %v", err)
	}
}
