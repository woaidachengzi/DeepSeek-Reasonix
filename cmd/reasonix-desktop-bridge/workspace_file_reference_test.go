package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/boot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

func writeReferenceFixture(t *testing.T, root, path string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# Gold review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

type workspaceReferenceTestRuntime struct{ *controllerRuntime }

func (r *workspaceReferenceTestRuntime) SessionPath() string {
	return filepath.Join(r.controller.WorkspaceRoot(), "test-session.jsonl")
}

func TestWorkspaceFilenameReferenceHTTP(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	name := "gold-technical-review-2026-09-01.md"
	writeReferenceFixture(t, root, "reports/"+name)
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: root, Sink: event.Discard})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &workspaceReferenceTestRuntime{&controllerRuntime{controller: controller}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	t.Cleanup(func() { _ = manager.Shutdown() })
	bridge := newBridgeServer(testToken, "instance", manager)
	handler := bridge.handler()
	post := func(route, path string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"path": path})
		if route == "/v1/sessions:open" {
			body = []byte(`{"sessionId":"tab-reference"}`)
		}
		request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	if response := post("/v1/sessions:open", ""); response.Code != 200 {
		t.Fatalf("open: %d %s", response.Code, response.Body.String())
	}
	preview := func(path string, status int, code string) workspaceFileResponse {
		t.Helper()
		response := post("/v1/sessions/tab-reference:workspace-file", path)
		if response.Code != status {
			t.Fatalf("preview %q: %d %s", path, response.Code, response.Body.String())
		}
		var result workspaceFileResponse
		if status == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
		} else if !strings.Contains(response.Body.String(), `"code":"`+code+`"`) || strings.Contains(response.Body.String(), root) {
			t.Fatalf("unsafe or wrong error: %s", response.Body.String())
		}
		return result
	}
	if result := preview(name, 200, ""); result.Preview.Path != "reports/"+name || result.Preview.Body != "# Gold review\n" {
		t.Fatalf("nested preview: %#v", result)
	}
	preview("missing.md", 404, "workspace_file_not_found")
	preview("wrong/"+name, 404, "workspace_file_not_found")
	preview("../"+name, 400, "invalid_request")
	writeReferenceFixture(t, root, "other/"+name)
	preview(name, 409, "workspace_file_ambiguous")
	preview("reports/"+name, 200, "")
	writeReferenceFixture(t, root, name)
	if result := preview(name, 200, ""); result.Preview.Path != name {
		t.Fatalf("exact root path must win: %#v", result)
	}
}

func TestWorkspaceFilenameLookupBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeReferenceFixture(t, outside, "secret.md")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "secret.md")); err != nil {
		t.Fatal(err)
	}
	writeReferenceFixture(t, root, "node_modules/secret.md")
	writeReferenceFixture(t, root, ".hidden/secret.md")
	writeReferenceFixture(t, root, "czsc_env/pyvenv.cfg")
	writeReferenceFixture(t, root, "czsc_env/lib/secret.md")
	if _, err := findWorkspaceFilename(root, "secret.md"); !errors.Is(err, desktopbridge.ErrWorkspaceFileNotFound) {
		t.Fatalf("hidden/symlink lookup: %v", err)
	}
	writeReferenceFixture(t, root, "docs/中文 文档.md")
	if path, err := findWorkspaceFilename(root, "中文 文档.md"); err != nil || path != "docs/中文 文档.md" {
		t.Fatalf("CJK lookup: %q, %v", path, err)
	}
	for _, path := range []string{"../secret.md", "docs/secret.md", `docs\secret.md`, "/secret.md", "C:secret.md", ".", "..", ""} {
		if isBareWorkspaceFilename(path) {
			t.Fatalf("accepted non-bare path %q", path)
		}
	}
}

func TestWorkspaceFilenameLookupRejectsIncompleteScan(t *testing.T) {
	root := t.TempDir()
	writeReferenceFixture(t, root, "000/match.md")
	for i := 0; i < workspaceReferenceWalkLimit; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("dir-%05d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := findWorkspaceFilename(root, "match.md"); !errors.Is(err, desktopbridge.ErrWorkspaceFileUnavailable) {
		t.Fatalf("incomplete scan silently chose a file: %v", err)
	}
}
