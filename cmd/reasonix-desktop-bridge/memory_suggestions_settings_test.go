package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/provider"
	"reasonix/internal/sessionidentity"
)

func TestPreviewMemorySuggestionsAreWorkspaceScopedAndExplicitlyAccepted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	root := t.TempDir()
	otherRoot := t.TempDir()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(id, workspace string) string {
		t.Helper()
		path, err := sessionpath.TranscriptPath(sessionDir, id)
		if err != nil {
			t.Fatal(err)
		}
		session := agent.NewSession("")
		session.Add(provider.Message{Role: provider.RoleUser, Content: "以后请始终用中文回复，除非我明确要求英文。"})
		if err := session.Save(path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	firstPath := write("preview-suggest-project", root)
	secondPath := write("preview-suggest-other", otherRoot)
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(context.Background(), sessionDir, []sessionidentity.Candidate{
		{ID: "preview-suggest-project", Path: firstPath, WorkspaceRoot: root},
		{ID: "preview-suggest-other", Path: secondPath, WorkspaceRoot: otherRoot},
	}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	server := newBridgeServer(testToken, "instance-memory-suggestions")
	handler := server.handler()
	query := "/v1/settings/memory/suggestions?workspaceRoot=" + url.QueryEscape(root)
	request := httptest.NewRequest(http.MethodGet, query, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized suggestions status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, query, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("suggestions status = %d: %s", response.Code, response.Body.String())
	}
	var view struct {
		Memories []struct {
			ID       string   `json:"id"`
			Evidence []string `json:"evidence"`
		} `json:"memories"`
		Skills []any `json:"skills"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Memories) != 1 || !strings.Contains(strings.Join(view.Memories[0].Evidence, " "), "preview-suggest-project") || strings.Contains(strings.Join(view.Memories[0].Evidence, " "), "preview-suggest-other") {
		t.Fatalf("workspace suggestions leaked or missed session: %#v", view.Memories)
	}
	body := `{"workspaceRoot":` + strconvQuote(root) + `,"kind":"memory","id":` + strconvQuote(view.Memories[0].ID) + `}`
	request = httptest.NewRequest(http.MethodPost, "/v1/settings/memory/suggestions/accept", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(requestIDHeader, "accept-suggested-memory")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("accept suggestion status = %d: %s", response.Code, response.Body.String())
	}
	var accepted previewMemorySuggestionAcceptanceView
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Path == "" || !strings.HasPrefix(filepath.Clean(accepted.Path), filepath.Clean(appconfig.MemoryUserDir())) || len(accepted.Suggestions.Memories) != 0 {
		t.Fatalf("accepted suggestion = %#v", accepted)
	}
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
