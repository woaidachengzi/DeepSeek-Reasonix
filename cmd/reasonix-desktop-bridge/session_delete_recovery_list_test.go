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
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func TestPendingSessionDeletesExposePathFreeExplicitRecoveryEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript, err := sessionpath.TranscriptPath(sessionDir, "interrupted-delete")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte("pending user-requested deletion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{
		ID: "interrupted-delete", Path: transcript, Title: "Interrupted conversation",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginDelete(context.Background(), "interrupted-delete", transcript); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", desktopbridge.NewRuntimeManager(newControllerFactory(nil))).handler()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/deletion-recovery", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("pending delete status=%d body=%s", response.Code, response.Body.String())
	}
	var body pendingSessionDeletesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ProtocolVersion != 1 || len(body.Sessions) != 1 || body.Sessions[0].ID != "interrupted-delete" || body.Sessions[0].Title != "Interrupted conversation" {
		t.Fatalf("pending deletes = %#v", body)
	}
	if strings.Contains(response.Body.String(), transcript) || strings.Contains(response.Body.String(), `"path"`) {
		t.Fatalf("pending delete response leaked a path: %s", response.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/v1/sessions/interrupted-delete", nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+testToken)
	deleteRequest.Header.Set(requestIDHeader, "explicit-recovery-delete")
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, deleteRequest)
	if deleted.Code != http.StatusOK {
		t.Fatalf("explicit delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
		t.Fatalf("transcript survived explicit deletion: %v", err)
	}

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, request)
	if after.Code != http.StatusOK {
		t.Fatalf("pending deletes after recovery status=%d body=%s", after.Code, after.Body.String())
	}
	if err := json.Unmarshal(after.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 0 {
		t.Fatalf("completed deletion remained pending: %#v", body.Sessions)
	}
	identities, err = sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(context.Background(), "interrupted-delete")
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("recovered identity = %#v, exists=%v, err=%v", record, exists, err)
	}
}
