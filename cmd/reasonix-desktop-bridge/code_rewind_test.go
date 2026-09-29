package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/checkpoint"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestCodeRewindRequiresCoverageConfirmationAndPreservesConversation(t *testing.T) {
	root, sessions := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("after"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sessionPath := filepath.Join(sessions, "session.jsonl")
	checkpointDir := strings.TrimSuffix(sessionPath, ".jsonl") + ".ckpt"
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	before, existed := "before", true
	files := make([]checkpoint.FileSnap, 0, 2)
	for _, name := range []string{"a.txt", "b.txt"} {
		files = append(files, checkpoint.FileSnap{
			Path: name, Content: &before, SHA256: checkpoint.Digest([]byte(before)), Mode: 0o644,
			AfterExisted: &existed, AfterSHA256: checkpoint.Digest([]byte("after")), AfterMode: 0o644,
			CaptureSource: checkpoint.CaptureBeforeMutation,
		})
	}
	ckpt := checkpoint.Checkpoint{
		SchemaVersion: checkpoint.SchemaV2, Turn: 1, Time: time.Now(), Prompt: "edit two files", MsgIndex: 3,
		Coverage:     checkpoint.CoveragePartial,
		CoverageGaps: []checkpoint.CoverageGap{{Reason: checkpoint.GapBashSideEffect}},
		Files:        files,
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
		{Role: provider.RoleUser, Content: "edit two files"},
		{Role: provider.RoleAssistant, Content: "done"},
	})
	if err := session.Save(sessionPath); err != nil {
		t.Fatal(err)
	}
	conversationBefore := session.Snapshot()
	ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: sessions, SessionPath: sessionPath, WorkspaceRoot: root})
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	checkpoints := runtime.WorkspaceCheckpoints()
	if len(checkpoints) != 1 || checkpoints[0].Turn != 1 || checkpoints[0].TurnFileCount != 2 {
		t.Fatalf("checkpoint list = %+v", checkpoints)
	}
	plan, err := runtime.PrepareCodeRewind(1)
	if err != nil || !plan.CanFiles || !plan.RequiresCoverageConfirmation || plan.FileCount != 2 || len(plan.Files) != 2 {
		t.Fatalf("code rewind preview = %+v err=%v", plan, err)
	}
	conversationPlan, err := controller.PrepareRewind(1, control.RewindConversation)
	if err != nil {
		t.Fatalf("conversation preview: %v", err)
	}
	if result, err := runtime.CommitCodeRewind(conversationPlan.PlanID, true); err != nil || result.OK {
		t.Fatalf("code endpoint accepted conversation plan: result=%+v err=%v", result, err)
	}
	result, err := runtime.CommitCodeRewind(plan.PlanID, false)
	if err != nil || result.OK {
		t.Fatalf("unconfirmed partial coverage committed: result=%+v err=%v", result, err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(content) != "after" {
			t.Fatalf("unconfirmed %s = %q err=%v", name, content, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("manual"), 0o644); err != nil {
		t.Fatal(err)
	}
	bothPlan, err := controller.PrepareRewind(1, control.RewindBoth)
	if err != nil || bothPlan.CanFiles || !bothPlan.CanConversation {
		t.Fatalf("conflicted combined preview = %+v err=%v", bothPlan, err)
	}
	if result, err := controller.CommitRewindInPlace(bothPlan.PlanID); err == nil || result.OK || result.ConversationForked {
		t.Fatalf("conflicted combined rewind forked conversation: result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(session.Snapshot(), conversationBefore) || controller.SessionPath() != sessionPath {
		t.Fatal("conflicted combined rewind changed the active conversation")
	}
	if heads, err := agent.ListSessionHeads(sessionPath); err != nil || len(heads) != 1 {
		t.Fatalf("conflicted combined rewind created a head: heads=%+v err=%v", heads, err)
	}
	result, err = runtime.CommitCodeRewind(plan.PlanID, true)
	if err != nil || result.OK {
		t.Fatalf("stale file rewind committed: result=%+v err=%v", result, err)
	}
	content, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(content) != "after" {
		t.Fatalf("stale rewind changed a.txt = %q err=%v", content, err)
	}
	content, err = os.ReadFile(filepath.Join(root, "b.txt"))
	if err != nil || string(content) != "manual" {
		t.Fatalf("stale rewind changed b.txt = %q err=%v", content, err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = runtime.PrepareCodeRewind(1)
	if err != nil || !plan.CanFiles {
		t.Fatalf("fresh preview after manual edit: plan=%+v err=%v", plan, err)
	}
	result, err = runtime.CommitCodeRewind(plan.PlanID, true)
	if err != nil || !result.OK || result.WrittenCount != 2 || !result.UndoAvailable {
		t.Fatalf("confirmed code rewind = %+v err=%v", result, err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(content) != "before" {
			t.Fatalf("restored %s = %q err=%v", name, content, err)
		}
	}
	if !reflect.DeepEqual(session.Snapshot(), conversationBefore) {
		t.Fatal("code-only rewind changed conversation")
	}
	undo, err := runtime.UndoWorkspaceFileRevert(result.TransactionID)
	if err != nil || !undo.OK {
		t.Fatalf("undo code rewind = %+v err=%v", undo, err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(content) != "after" {
			t.Fatalf("undone %s = %q err=%v", name, content, err)
		}
	}
}
