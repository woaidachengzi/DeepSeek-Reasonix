package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/checkpoint"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/sessionidentity"
)

func TestLegacyConversationForkRegistersReopenableChildAndPreservesSource(t *testing.T) {
	state := t.TempDir()
	workspace := t.TempDir()
	workspaceFile := filepath.Join(workspace, "keep.txt")
	if err := os.WriteFile(workspaceFile, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REASONIX_HOME", state)
	t.Setenv("REASONIX_STATE_HOME", state)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	parentID := "tauri-parent"
	parentPath, err := bridgeSessionPath(sessionDir, parentID)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("{\"role\":\"system\",\"content\":\"sys\"}\n{\"role\":\"user\",\"content\":\"first\"}\n{\"role\":\"assistant\",\"content\":\"first answer\"}\n{\"role\":\"user\",\"content\":\"second\"}\n{\"role\":\"assistant\",\"content\":\"second answer\"}\n")
	if err := os.WriteFile(parentPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	ckptDir := strings.TrimSuffix(parentPath, ".jsonl") + ".ckpt"
	if err := os.MkdirAll(ckptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ckpt, err := json.Marshal(checkpoint.Checkpoint{SchemaVersion: checkpoint.SchemaV2, Turn: 1, Time: time.Now(), Prompt: "second", MsgIndex: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ckptDir, "turn-1.json"), ckpt, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.ImportLegacyCatalog(ctx, sessionDir, []sessionidentity.Candidate{{ID: parentID, Path: parentPath, WorkspaceRoot: workspace}}); err != nil {
		t.Fatal(err)
	}
	_ = identities.Close()
	session, err := agent.LoadSession(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, dag := session.Head(); dag {
		t.Fatal("fixture must use the old session format")
	}
	ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: sessionDir, SessionPath: parentPath, WorkspaceRoot: workspace})
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller, sessionID: parentID}
	preview, err := runtime.PrepareLegacyConversationFork(1)
	if err != nil || !preview.CanConversation || preview.PlanID == "" {
		t.Fatalf("preview = %+v, err = %v", preview, err)
	}
	if _, err := runtime.CommitLegacyConversationFork("invalid-plan"); err == nil {
		t.Fatal("invalid plan accepted")
	}
	result, err := runtime.CommitLegacyConversationFork(preview.PlanID)
	if err != nil || !result.OK || result.SessionID == "" || result.SessionID == parentID {
		t.Fatalf("commit = %+v, err = %v", result, err)
	}
	if controller.SessionPath() != parentPath {
		t.Fatal("source controller moved")
	}
	if current, err := os.ReadFile(parentPath); err != nil || !reflect.DeepEqual(current, original) {
		t.Fatalf("source changed: %v", err)
	}
	if current, err := os.ReadFile(workspaceFile); err != nil || string(current) != "untouched" {
		t.Fatalf("workspace file changed: %q, %v", current, err)
	}
	childPath, err := bridgeSessionPath(sessionDir, result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := agent.LoadSession(childPath)
	if err != nil {
		t.Fatal(err)
	}
	got := child.Snapshot()
	if len(got) != 3 || got[2].Content != "first answer" {
		t.Fatalf("child messages = %+v", got)
	}
	reopened, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: result.SessionID})
	if err != nil {
		t.Fatalf("child identity cannot reopen: %v", err)
	}
	defer reopened.Shutdown()
}
