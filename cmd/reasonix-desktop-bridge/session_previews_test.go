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
