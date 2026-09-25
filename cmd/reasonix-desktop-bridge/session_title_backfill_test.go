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

func TestBackfillSessionTitlesSkipsUnsafeStoredTitleWithoutBlockingOtherRows(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	for position, entry := range []struct{ id, title string }{
		{id: "old-unsafe", title: "old\nlegacy title"},
		{id: "new-fallback"},
	} {
		path, err := sessionpath.TranscriptPath(sessionDir, entry.id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{
			ID: entry.id, Path: path, Title: entry.title, Position: position,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/titles/first-message", strings.NewReader(
		`{"titles":[{"sessionId":"old-unsafe","title":"Safe candidate"},{"sessionId":"new-fallback","title":"First question"}]}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("title backfill status=%d body=%s", response.Code, response.Body.String())
	}
	var body backfillSessionTitlesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Titles) != 1 ||
		body.Titles[0].SessionID != "new-fallback" || body.Titles[0].Title != "First question" {
		t.Fatalf("unsafe title blocked or leaked in backfill: %#v err=%v", body, err)
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	unsafe, exists, err := identities.Get(ctx, "old-unsafe")
	if err != nil || !exists || unsafe.Title != "old\nlegacy title" || unsafe.TitleSource != sessionidentity.TitleLegacyUnknown {
		t.Fatalf("unsafe historical title was mutated: %#v exists=%v err=%v", unsafe, exists, err)
	}
	fallback, exists, err := identities.Get(ctx, "new-fallback")
	if err != nil || !exists || fallback.Title != "First question" || fallback.TitleSource != sessionidentity.TitleFallback {
		t.Fatalf("valid fallback title was not written: %#v exists=%v err=%v", fallback, exists, err)
	}
}
