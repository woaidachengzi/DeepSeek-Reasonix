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

func TestBackfillSessionTitlesRejectsMalformedJSONWithProtocolError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/titles/first-message", strings.NewReader(`{"titles":[`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
	var body struct {
		ProtocolVersion int `json:"protocolVersion"`
		Error           struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
		body.ProtocolVersion != 1 || body.Error.Code != "invalid_request" {
		t.Fatalf("malformed title backfill response: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Lstat(appconfig.DesktopSessionIdentityPath()); !os.IsNotExist(err) {
		t.Fatalf("malformed title backfill opened identity store: %v", err)
	}
}

func TestBackfillSessionTitlesOnlyFillsFallbackTitles(t *testing.T) {
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
	for _, item := range []struct{ id, title string }{
		{id: "needs-title"},
		{id: "manual-title", title: "Chosen title"},
	} {
		path, err := sessionpath.TranscriptPath(sessionDir, item.id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{{
			ID: item.id, Path: path, Title: item.title,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", nil).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/titles/first-message", strings.NewReader(
		`{"titles":[{"sessionId":"needs-title","title":"First question"},{"sessionId":"manual-title","title":"Must not replace"}]}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"title":"First question"`) ||
		!strings.Contains(response.Body.String(), `"title":"Chosen title"`) {
		t.Fatalf("backfill status=%d body=%s", response.Code, response.Body.String())
	}

	identities, err = sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identities.Close() }()
	filled, ok, err := identities.Get(context.Background(), "needs-title")
	if err != nil || !ok || filled.Title != "First question" || filled.TitleSource != sessionidentity.TitleFallback {
		t.Fatalf("fallback title = %#v ok=%v err=%v", filled, ok, err)
	}
	manual, ok, err := identities.Get(context.Background(), "manual-title")
	if err != nil || !ok || manual.Title != "Chosen title" {
		t.Fatalf("manual title was replaced: %#v ok=%v err=%v", manual, ok, err)
	}
}

func TestBackfillSessionTitlesRejectsDuplicateIDsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	handler := newBridgeServer(testToken, "instance", nil).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/titles/first-message", strings.NewReader(
		`{"titles":[{"sessionId":"same","title":"one"},{"sessionId":"same","title":"two"}]}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate title status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(appconfig.DesktopSessionIdentityPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected batch created identity store: %v", err)
	}
}
