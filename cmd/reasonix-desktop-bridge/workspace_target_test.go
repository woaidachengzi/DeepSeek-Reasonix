package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/desktopbridge"
)

type targetTestRuntime struct {
	*bridgeTestRuntime
	root string
	err  error
}

func (r *targetTestRuntime) LocalWorkspace() (string, error) { return r.root, r.err }

func TestWorkspaceTargetRequiresAuthorizationAndOwnedSession(t *testing.T) {
	runtime := &targetTestRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/tmp/target.jsonl", state: "idle"}, root: t.TempDir()}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "global"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Shutdown() })
	bridge := newBridgeServer(testToken, "workspace-target-test", manager)
	request := func(id string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+id+"/workspace-target", nil)
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		bridge.handler().ServeHTTP(response, req)
		return response
	}
	if response := request("global", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status %d", response.Code)
	}
	response := request("global", true)
	if response.Code != http.StatusOK {
		t.Fatalf("owned status %d", response.Code)
	}
	var target workspaceTargetResponse
	if err := json.Unmarshal(response.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if target.ProtocolVersion != 1 || target.SessionID != "global" || target.WorkspaceRoot != runtime.root {
		t.Fatalf("incorrect target %+v", target)
	}
	if response := request("other-session", true); response.Code != http.StatusNotFound {
		t.Fatalf("unowned status %d", response.Code)
	}
	runtime.err = desktopbridge.ErrInvalidWorkspacePath
	if response := request("global", true); response.Code == http.StatusOK {
		t.Fatal("unavailable workspace reported success")
	}
}

func TestGlobalWorkspaceIsProfileScopedStableAndSeparateFromProjectAssignment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() { _ = manager.Shutdown() })
	view, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "global-target"})
	if err != nil {
		t.Fatal(err)
	}
	if view.WorkspaceRoot != "" {
		t.Fatal("Global acquired a project assignment")
	}
	target, err := manager.WorkspaceTarget(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(home, "global-workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if target.WorkspaceRoot != want {
		t.Fatalf("global target %q, want %q", target.WorkspaceRoot, want)
	}
	if err := os.WriteFile(filepath.Join(want, "preserved.md"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if _, err := manager.Switch(context.Background(), desktopbridge.OpenRequest{SessionID: "project-target", WorkspaceRoot: project}); err != nil {
		t.Fatal(err)
	}
	target, err = manager.WorkspaceTarget("project-target")
	if err != nil {
		t.Fatal(err)
	}
	project, _ = filepath.EvalSymlinks(project)
	if target.WorkspaceRoot != project {
		t.Fatal("project opened the Global workspace")
	}
	if _, err := manager.Switch(context.Background(), desktopbridge.OpenRequest{SessionID: "global-target"}); err != nil {
		t.Fatal(err)
	}
	target, err = manager.WorkspaceTarget("global-target")
	if err != nil || target.WorkspaceRoot != want {
		t.Fatalf("unstable Global target: %+v %v", target, err)
	}
	if data, err := os.ReadFile(filepath.Join(want, "preserved.md")); err != nil || string(data) != "preserve" {
		t.Fatal("Global content was not preserved")
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	restarted := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() { _ = restarted.Shutdown() })
	if _, err := restarted.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "global-target"}); err != nil {
		t.Fatal(err)
	}
	target, err = restarted.WorkspaceTarget("global-target")
	if err != nil || target.WorkspaceRoot != want {
		t.Fatalf("restart target: %+v %v", target, err)
	}
	otherHome := t.TempDir()
	t.Setenv("REASONIX_HOME", otherHome)
	other, err := previewGlobalWorkspace()
	if err != nil || other == want {
		t.Fatalf("profiles share Global target: %q %v", other, err)
	}
}

func TestGlobalWorkspaceRejectsFileAndSymlinkWithoutTouchingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := filepath.Join(home, "global-workspace")
	if err := os.WriteFile(root, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := previewGlobalWorkspace(); err == nil {
		t.Fatal("file accepted as workspace")
	}
	if data, err := os.ReadFile(root); err != nil || string(data) != "original" {
		t.Fatal("existing file was altered")
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := previewGlobalWorkspace(); err == nil {
		t.Fatal("implicit Global symlink accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("symlink target was altered")
	}
}
