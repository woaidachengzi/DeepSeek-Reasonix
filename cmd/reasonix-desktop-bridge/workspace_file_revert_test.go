package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/checkpoint"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestWorkspaceFileRevertRequiresExplicitConflictResolution(t *testing.T) {
	root := t.TempDir()
	sessions := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessions, "session.jsonl")
	checkpointDir := strings.TrimSuffix(sessionPath, ".jsonl") + ".ckpt"
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	before, existed := "before", true
	ckpt := checkpoint.Checkpoint{
		SchemaVersion: checkpoint.SchemaV2, Turn: 1, Time: time.Now(), Prompt: "edit", MsgIndex: 3,
		Coverage: checkpoint.CoverageComplete,
		Files: []checkpoint.FileSnap{{
			Path: "notes.txt", Content: &before, SHA256: checkpoint.Digest([]byte(before)), Mode: 0o644,
			AfterExisted: &existed, AfterSHA256: checkpoint.Digest([]byte("after")), AfterMode: 0o644,
			CaptureSource: checkpoint.CaptureBeforeMutation,
		}},
	}
	encoded, err := json.Marshal(ckpt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkpointDir, "turn-1.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	session := agent.NewSession("")
	session.Replace([]provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "start"},
		{Role: provider.RoleAssistant, Content: "done"},
		{Role: provider.RoleUser, Content: "edit"},
		{Role: provider.RoleAssistant, Content: "done"},
	})
	if err := session.Save(sessionPath); err != nil {
		t.Fatal(err)
	}
	ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: sessions, SessionPath: sessionPath, WorkspaceRoot: root})
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}

	plan, err := runtime.PrepareWorkspaceFileRevert("notes.txt")
	if err != nil || !plan.CanFiles || plan.PlanID == "" || len(plan.Conflicts) != 0 {
		t.Fatalf("clean preview: plan=%+v err=%v", plan, err)
	}
	if err := os.WriteFile(file, []byte("manual"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.CommitWorkspaceFileRevert(plan.PlanID, "")
	if err != nil || result.OK {
		t.Fatalf("stale commit was accepted: result=%+v err=%v", result, err)
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != "manual" {
		t.Fatalf("manual edit was changed: %q err=%v", content, err)
	}
	plan, err = runtime.PrepareWorkspaceFileRevert("notes.txt")
	if err != nil || !plan.CanFiles || len(plan.Conflicts) == 0 {
		t.Fatalf("conflict preview: plan=%+v err=%v", plan, err)
	}
	result, err = runtime.CommitWorkspaceFileRevert(plan.PlanID, "")
	if err != nil || result.OK {
		t.Fatalf("conflict committed without resolution: result=%+v err=%v", result, err)
	}
	plan, err = runtime.PrepareWorkspaceFileRevert("notes.txt")
	if err != nil || !plan.CanFiles || len(plan.Conflicts) == 0 {
		t.Fatalf("fresh conflict preview: plan=%+v err=%v", plan, err)
	}
	result, err = runtime.CommitWorkspaceFileRevert(plan.PlanID, "overwrite_checkpoint")
	if err != nil || !result.OK || result.WrittenCount != 1 {
		t.Fatalf("explicit overwrite: result=%+v err=%v", result, err)
	}
	content, err = os.ReadFile(file)
	if err != nil || string(content) != "before" {
		t.Fatalf("restored content = %q err=%v", content, err)
	}
	if !result.UndoAvailable || result.TransactionID == "" {
		t.Fatalf("restore did not produce an undo transaction: %+v", result)
	}
	if err := os.WriteFile(file, []byte("changed-after-restore"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, err := runtime.UndoWorkspaceFileRevert(result.TransactionID)
	if err != nil || undo.OK {
		t.Fatalf("undo overwrote a later manual edit: result=%+v err=%v", undo, err)
	}
	content, err = os.ReadFile(file)
	if err != nil || string(content) != "changed-after-restore" {
		t.Fatalf("later manual edit was changed: %q err=%v", content, err)
	}
	if err := os.WriteFile(file, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, err = runtime.UndoWorkspaceFileRevert(result.TransactionID)
	if err != nil || !undo.OK {
		t.Fatalf("undo restore: result=%+v err=%v", undo, err)
	}
	content, err = os.ReadFile(file)
	if err != nil || string(content) != "manual" {
		t.Fatalf("undo content = %q err=%v", content, err)
	}
	undo, err = runtime.UndoWorkspaceFileRevert(result.TransactionID)
	if err != nil || undo.OK {
		t.Fatalf("transaction was undone twice: result=%+v err=%v", undo, err)
	}
	if _, err := runtime.PrepareWorkspaceFileRevert("../outside.txt"); err == nil {
		t.Fatal("path outside workspace was accepted")
	}
}
