package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
	sessionstore "reasonix/internal/store"
)

func TestSessionPreviewReadsOnlyFirstVisibleUserTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tauri-test.jsonl")
	session := agent.NewSession("private system prompt")
	session.Add(provider.Message{Role: provider.RoleUser, Content: sessioncontext.Build(sessioncontext.Sections{Workspace: "private context"}).Content})
	session.Add(provider.Message{Role: provider.RoleUser, Content: "请修复这个项目\n再添加测试"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "private assistant answer"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	preview := readSessionPreview(path, "test")
	if preview.FirstUser != "请修复这个项目\n再添加测试" || preview.Title != "" {
		t.Fatalf("preview = %+v", preview)
	}
	if strings.Contains(preview.FirstUser, "private") {
		t.Fatal("preview leaked host or assistant content")
	}
	if err := agent.RenameSession(path, "手动标题"); err != nil {
		t.Fatal(err)
	}
	if got := readSessionPreview(path, "test").Title; got != "手动标题" {
		t.Fatalf("stored title = %q", got)
	}
	if got := readSessionPreview(filepath.Join(t.TempDir(), "missing.jsonl"), "missing"); got.FirstUser != "" || got.Title != "" {
		t.Fatalf("missing preview = %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSessionPreviewUsesFreshListingProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tauri-cached.jsonl")
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "ordinary first question"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	want, turns, ok := agent.SessionPreviewCached(path)
	if !ok || turns != 1 || want != "ordinary first question" {
		t.Fatalf("fixture listing projection = %q, %d, %v", want, turns, ok)
	}
	if got := readSessionPreview(path, "cached"); got.FirstUser != want {
		t.Fatalf("cached preview = %+v, want first user %q", got, want)
	}
}

func TestSessionPreviewDoesNotFollowTranscriptMetadataOrEventLogSymlinks(t *testing.T) {
	root := t.TempDir()
	profileSessions := filepath.Join(root, "profile", "sessions")
	if err := os.MkdirAll(profileSessions, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideTranscript := filepath.Join(root, "outside.jsonl")
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "outside secret"})
	if err := session.Save(outsideTranscript); err != nil {
		t.Fatal(err)
	}
	transcriptLink := filepath.Join(profileSessions, "tauri-transcript-link.jsonl")
	if err := os.Symlink(outsideTranscript, transcriptLink); err != nil {
		t.Fatal(err)
	}
	if got := readSessionPreview(transcriptLink, "transcript-link"); got.FirstUser != "" || got.Title != "" {
		t.Fatalf("transcript symlink preview leaked outside data: %+v", got)
	}

	profileTranscript := filepath.Join(profileSessions, "tauri-meta-link.jsonl")
	profileSession := agent.NewSession("")
	profileSession.Add(provider.Message{Role: provider.RoleUser, Content: "inside safe"})
	if err := profileSession.Save(profileTranscript); err != nil {
		t.Fatal(err)
	}
	metaPath := profileTranscript + ".meta"
	outsideMeta := filepath.Join(root, "outside.jsonl.meta")
	if err := os.WriteFile(outsideMeta, []byte(`{"customTitle":"outside title"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideMeta, metaPath); err != nil {
		t.Fatal(err)
	}
	if got := readSessionPreview(profileTranscript, "meta-link"); got.Title != "" || got.FirstUser != "inside safe" {
		t.Fatalf("metadata symlink preview = %+v", got)
	}

	eventTranscript := filepath.Join(profileSessions, "tauri-event-link.jsonl")
	eventSession := agent.NewSession("")
	eventSession.Add(provider.Message{Role: provider.RoleUser, Content: "local checkpoint"})
	if err := eventSession.Save(eventTranscript); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(eventTranscript + ".meta"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	profileEventLog := sessionstore.SessionEventLog(eventTranscript)
	if err := os.Remove(profileEventLog); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(sessionstore.SessionEventLog(outsideTranscript), profileEventLog); err != nil {
		t.Fatal(err)
	}
	if got := readSessionPreview(eventTranscript, "event-link"); got.FirstUser != "" || got.Title != "" {
		t.Fatalf("event-log symlink preview leaked outside data: %+v", got)
	}
}

func TestSessionPreviewEndpointDoesNotOpenOrSwitchController(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	path, err := bridgeSessionPath(appconfig.SessionDir(), "historical")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "历史问题"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, _ desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		t.Fatal("preview must not open a controller")
		return nil, nil
	}))
	handler := newBridgeServer(testToken, "instance", manager).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:previews", strings.NewReader(`{"sessionIds":["historical"]}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"firstUser":"历史问题"`) {
		t.Fatalf("preview status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, open := manager.Snapshot(); open {
		t.Fatal("preview opened a controller")
	}
	invalid := httptest.NewRequest(http.MethodPost, "/v1/sessions:previews", strings.NewReader(`{"sessionIds":["../outside"]}`))
	invalid.Header.Set("Authorization", "Bearer "+testToken)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, invalid)
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d", denied.Code)
	}
}

func TestSessionPreviewEndpointDoesNotFollowSessionDirectorySymlink(t *testing.T) {
	stateRoot := t.TempDir()
	outside := t.TempDir()
	t.Setenv("REASONIX_HOME", stateRoot)
	t.Setenv("REASONIX_STATE_HOME", stateRoot)
	if err := os.Symlink(outside, filepath.Join(stateRoot, "sessions")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outside, "tauri-outside.jsonl")
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "outside directory secret"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(_ context.Context, _ desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		t.Fatal("preview must not open a controller")
		return nil, nil
	}))
	handler := newBridgeServer(testToken, "instance", manager).handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:previews", strings.NewReader(`{"sessionIds":["outside"]}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "outside directory secret") || strings.Contains(response.Body.String(), "firstUser") {
		t.Fatalf("symlinked session directory preview = %d, %s", response.Code, response.Body.String())
	}
}
