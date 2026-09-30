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
	conversationPreview, err := runtime.PrepareConversationRewind(1)
	if err != nil || !conversationPreview.CanConversation || conversationPreview.PlanID == "" {
		t.Fatalf("conversation preview = %+v err=%v", conversationPreview, err)
	}
	if result, err := runtime.CommitConversationRewind(plan.PlanID); err == nil || result.OK {
		t.Fatalf("code plan accepted by conversation endpoint: result=%+v err=%v", result, err)
	}
	if result, err := runtime.CommitCombinedRewind(conversationPreview.PlanID, true); err != nil || result.OK {
		t.Fatalf("conversation plan accepted by combined endpoint: result=%+v err=%v", result, err)
	}
	conversationResult, err := runtime.CommitConversationRewind(conversationPreview.PlanID)
	if err != nil || !conversationResult.OK || !conversationResult.ConversationForked || conversationResult.HeadID == "" {
		t.Fatalf("conversation rewind = %+v err=%v", conversationResult, err)
	}
	if controller.SessionPath() != sessionPath || len(session.Snapshot()) != 3 {
		t.Fatalf("conversation rewind changed path or kept later messages: path=%q messages=%d", controller.SessionPath(), len(session.Snapshot()))
	}
	if history := runtime.History(); len(history) != 2 || history[1].Content != "done" {
		t.Fatalf("conversation history after rewind = %+v", history)
	}
	if heads, err := agent.ListSessionHeads(sessionPath); err != nil || len(heads) != 2 || !heads[1].Selected {
		t.Fatalf("conversation rewind heads = %+v err=%v", heads, err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(content) != "after" {
			t.Fatalf("conversation rewind changed %s = %q err=%v", name, content, err)
		}
	}
	if result, err := runtime.UndoConversationRewind("wrong-head"); err == nil || result.OK {
		t.Fatalf("wrong head undid conversation: result=%+v err=%v", result, err)
	}
	if result, err := runtime.UndoConversationRewind(conversationResult.HeadID); err != nil || !result.OK {
		t.Fatalf("undo conversation = %+v err=%v", result, err)
	}
	if !reflect.DeepEqual(session.Snapshot(), conversationBefore) || controller.SessionPath() != sessionPath {
		t.Fatal("undo did not restore the original conversation on the same path")
	}
	if history := runtime.History(); len(history) != 4 || history[3].Content != "done" {
		t.Fatalf("conversation history after undo = %+v", history)
	}
	reloaded, err := agent.LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Snapshot()) != len(conversationBefore) {
		t.Fatalf("persisted conversation after undo = %d messages", len(reloaded.Snapshot()))
	}
	if result, err := runtime.UndoConversationRewind(conversationResult.HeadID); err == nil || result.OK {
		t.Fatalf("repeated undo succeeded: result=%+v err=%v", result, err)
	}
	continuedPlan, err := runtime.PrepareConversationRewind(1)
	if err != nil || !continuedPlan.CanConversation {
		t.Fatalf("continued conversation preview = %+v err=%v", continuedPlan, err)
	}
	continued, err := runtime.CommitConversationRewind(continuedPlan.PlanID)
	if err != nil || !continued.OK {
		t.Fatalf("continued conversation rewind = %+v err=%v", continued, err)
	}
	session.Add(provider.Message{Role: provider.RoleUser, Content: "continue here"})
	if err := controller.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if result, err := runtime.UndoConversationRewind(continued.HeadID); err == nil || result.OK {
		t.Fatalf("continued head was undone: result=%+v err=%v", result, err)
	}
	if controller.SessionPath() != sessionPath || session.Snapshot()[3].Content != "continue here" {
		t.Fatal("failed undo disturbed the continued conversation")
	}
	versions, err := runtime.SessionHeads()
	if err != nil || len(versions) != 2 || !versions[1].Selected {
		t.Fatalf("continued versions = %+v err=%v", versions, err)
	}
	if err := runtime.SwitchSessionHead(agent.SessionMainHead); err != nil {
		t.Fatalf("switch to original: %v", err)
	}
	if controller.SessionPath() != sessionPath || !reflect.DeepEqual(session.Snapshot(), conversationBefore) {
		t.Fatal("switching to original changed path or returned wrong messages")
	}
	if err := runtime.SwitchSessionHead(continued.HeadID); err != nil {
		t.Fatalf("switch to continued version: %v", err)
	}
	if controller.SessionPath() != sessionPath || session.Snapshot()[3].Content != "continue here" {
		t.Fatal("switching back lost the continued version")
	}
	reopened, err := agent.LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := reopened.Head(); !ok || head.HeadID != continued.HeadID || reopened.Snapshot()[3].Content != "continue here" {
		t.Fatal("reopening did not retain the selected conversation version")
	}
	if err := runtime.SwitchSessionHead("missing-head"); err == nil {
		t.Fatal("unknown head was accepted")
	}
	if checkpoints := runtime.WorkspaceCheckpoints(); len(checkpoints) != 0 {
		t.Fatalf("foreign checkpoint appeared on continued version: %+v", checkpoints)
	}
	foreignPlan, err := runtime.PrepareCombinedRewind(1)
	if err != nil || foreignPlan.CanFiles || foreignPlan.CanConversation || foreignPlan.PlanID != "" {
		t.Fatalf("foreign checkpoint was accepted: %+v err=%v", foreignPlan, err)
	}
	if err := runtime.SwitchSessionHead(agent.SessionMainHead); err != nil {
		t.Fatalf("switch to checkpoint's version: %v", err)
	}
	planAcrossSwitch, err := runtime.PrepareCodeRewind(1)
	if err != nil || !planAcrossSwitch.CanFiles {
		t.Fatalf("code plan before head switch = %+v err=%v", planAcrossSwitch, err)
	}
	if err := runtime.SwitchSessionHead(continued.HeadID); err != nil {
		t.Fatal(err)
	}
	if result, err := runtime.CommitCodeRewind(planAcrossSwitch.PlanID, true); err != nil || result.OK {
		t.Fatalf("plan from another head committed: %+v err=%v", result, err)
	}
	if err := runtime.SwitchSessionHead(agent.SessionMainHead); err != nil {
		t.Fatal(err)
	}
	combinedPlan, err := runtime.PrepareCombinedRewind(1)
	if err != nil || !combinedPlan.CanFiles || !combinedPlan.CanConversation || !combinedPlan.RequiresCoverageConfirmation {
		t.Fatalf("combined preview = %+v err=%v", combinedPlan, err)
	}
	if result, err := runtime.CommitCombinedRewind(combinedPlan.PlanID, false); err != nil || result.OK {
		t.Fatalf("unconfirmed combined rewind = %+v err=%v", result, err)
	}
	if len(session.Snapshot()) != 5 {
		t.Fatal("unconfirmed combined rewind changed conversation")
	}
	combinedResult, err := runtime.CommitCombinedRewind(combinedPlan.PlanID, true)
	if err != nil || !combinedResult.OK || combinedResult.Partial || !combinedResult.ConversationForked || !combinedResult.FilesRestored || !combinedResult.UndoAvailable || combinedResult.HeadID == "" {
		t.Fatalf("combined commit = %+v err=%v", combinedResult, err)
	}
	if controller.SessionPath() != sessionPath || len(session.Snapshot()) != 3 {
		t.Fatal("combined commit changed session path or kept later messages")
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if data, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(data) != "before" {
			t.Fatalf("combined restore %s = %q err=%v", name, data, err)
		}
	}
	if undo, err := runtime.UndoWorkspaceFileRevert(combinedResult.TransactionID); err != nil || !undo.OK {
		t.Fatalf("undo combined file transaction = %+v err=%v", undo, err)
	}
	if head, ok := controller.SessionHead(); !ok || head.HeadID != agent.SessionMainHead {
		t.Fatalf("combined undo did not return to previous head: %+v", head)
	}
	continuedCombinedPlan, err := runtime.PrepareCombinedRewind(1)
	if err != nil || !continuedCombinedPlan.CanFiles || !continuedCombinedPlan.CanConversation {
		t.Fatalf("continued combined preview = %+v err=%v", continuedCombinedPlan, err)
	}
	continuedCombined, err := runtime.CommitCombinedRewind(continuedCombinedPlan.PlanID, true)
	if err != nil || !continuedCombined.OK || continuedCombined.Partial {
		t.Fatalf("continued combined commit = %+v err=%v", continuedCombined, err)
	}
	session.Add(provider.Message{Role: provider.RoleUser, Content: "keep this version"})
	if err := controller.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if undo, err := runtime.UndoWorkspaceFileRevert(continuedCombined.TransactionID); err != nil || !undo.OK {
		t.Fatalf("undo files on continued head = %+v err=%v", undo, err)
	}
	if head, ok := controller.SessionHead(); !ok || head.HeadID != continuedCombined.HeadID {
		t.Fatalf("file undo switched continued head: %+v", head)
	}
	if err := runtime.SwitchSessionHead(agent.SessionMainHead); err != nil {
		t.Fatalf("return to earlier version: %v", err)
	}
	stalePlan, err := runtime.PrepareCombinedRewind(1)
	if err != nil || !stalePlan.CanFiles || !stalePlan.CanConversation {
		t.Fatalf("stale combined preview = %+v err=%v", stalePlan, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("manual edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	partial, err := runtime.CommitCombinedRewind(stalePlan.PlanID, true)
	if err != nil || !partial.OK || !partial.Partial || !partial.ConversationForked || partial.FilesRestored || partial.HeadID == "" {
		t.Fatalf("partial combined rewind = %+v err=%v", partial, err)
	}
	if partial.Error == "" || strings.Contains(partial.Error, root) || controller.SessionPath() != sessionPath {
		t.Fatalf("partial result leaked a path or changed transcript identity: %+v", partial)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(data) != "manual edit" {
		t.Fatalf("partial rewind overwrote manual edit: %q err=%v", data, err)
	}
	if _, err := runtime.UndoConversationRewind(partial.HeadID); err != nil {
		t.Fatalf("return after partial combined rewind: %v", err)
	}
}

func TestConversationRewindRejectsFileBranchPolicyBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	session := agent.NewSession("sys")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{
		Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: dir,
		SessionPath: path, FileBranchesOnly: true,
	})
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	preview, err := runtime.PrepareConversationRewind(0)
	if err != nil || preview.CanConversation || preview.DisabledReason == "" || preview.PlanID != "" {
		t.Fatalf("file-branch preview = %+v err=%v", preview, err)
	}
	if result, err := runtime.CommitConversationRewind("valid-plan-id"); err == nil || result.OK {
		t.Fatalf("file-branch commit = %+v err=%v", result, err)
	}
	if controller.SessionPath() != path {
		t.Fatalf("file-branch policy changed session path to %q", controller.SessionPath())
	}
}

func TestConversationRewindPreviewDisablesSchemaOneSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	legacy := "{\"role\":\"system\",\"content\":\"sys\"}\n{\"role\":\"user\",\"content\":\"hello\"}\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := session.Head(); ok {
		t.Fatal("legacy fixture unexpectedly has a schema-2 head")
	}
	ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: dir, SessionPath: path})
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	preview, err := runtime.PrepareConversationRewind(0)
	if err != nil || preview.CanConversation || preview.PlanID != "" || preview.DisabledReason == "" {
		t.Fatalf("legacy conversation preview = %+v err=%v", preview, err)
	}
	if result, err := runtime.CommitConversationRewind("valid-plan-id"); err == nil || result.OK {
		t.Fatalf("legacy conversation commit = %+v err=%v", result, err)
	}
	if plan, err := runtime.PrepareCombinedRewind(0); err != nil || plan.CanFiles || plan.CanConversation || plan.PlanID != "" {
		t.Fatalf("legacy combined preview = %+v err=%v", plan, err)
	}
	if heads, err := runtime.SessionHeads(); err != nil || len(heads) != 0 {
		t.Fatalf("legacy heads = %+v err=%v", heads, err)
	}
	if err := runtime.SwitchSessionHead("main"); err == nil {
		t.Fatal("legacy session accepted an in-log head switch")
	}
	if controller.SessionPath() != path || len(session.Snapshot()) != 2 {
		t.Fatal("legacy conversation changed despite disabled preview")
	}
}
