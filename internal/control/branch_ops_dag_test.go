package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/store"
)

func newSchemaTwoBranchController(t *testing.T) (*Controller, *agent.Session, string) {
	t.Helper()
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	sess := exec.Session()
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "root prompt"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "root answer"})
	c := New(Options{Executor: exec, SessionDir: dir, Label: "test", Sink: event.Discard})
	path := filepath.Join(dir, "root.jsonl")
	c.SetSessionPath(path)
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.Head(); !ok {
		t.Fatal("session must be schema 2 after its first save")
	}
	return c, sess, path
}

func TestBranchAndSwitchUseHeadsForSchemaTwo(t *testing.T) {
	c, sess, path := newSchemaTwoBranchController(t)
	rootID := agent.BranchID(path)
	head, err := c.Branch("experiment")
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}
	if head == "" || head == path || strings.HasSuffix(head, ".jsonl") {
		t.Fatalf("Branch on schema 2 must return a head id, got %q", head)
	}
	if c.SessionPath() != path {
		t.Fatalf("branch must not change the session path: %q", c.SessionPath())
	}
	entries, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.jsonl"))
	transcripts := 0
	for _, entry := range entries {
		if store.IsSessionTranscriptName(filepath.Base(entry)) {
			transcripts++
		}
	}
	if transcripts != 1 {
		t.Fatalf("branch created a new transcript file: %v", entries)
	}
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "on the branch"})
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	branches, err := c.Branches()
	if err != nil {
		t.Fatal(err)
	}
	var main, branch *agent.BranchInfo
	for i := range branches {
		switch branches[i].ID {
		case rootID:
			main = &branches[i]
		case head:
			branch = &branches[i]
		}
	}
	if main == nil || branch == nil {
		t.Fatalf("branches = %+v, want the log's main head and the new head", branches)
	}
	if main.HeadID != agent.SessionMainHead || branch.HeadKind != agent.HeadKindFork || branch.ParentID != rootID || branch.Name != "experiment" || branch.Path != path {
		t.Fatalf("head branch infos = main %+v branch %+v", main, branch)
	}
	tree := c.BranchTreeText()
	if !strings.Contains(tree, "experiment") {
		t.Fatalf("tree must list the head:\n%s", tree)
	}
	if _, err := c.SwitchBranch(rootID); err != nil {
		t.Fatalf("SwitchBranch main: %v", err)
	}
	if got := len(sess.Snapshot()); got != 3 || c.SessionPath() != path {
		t.Fatalf("after switching back: %d messages path %q", got, c.SessionPath())
	}
	if _, err := c.SwitchBranch(head); err != nil {
		t.Fatalf("SwitchBranch head: %v", err)
	}
	if got := sess.Snapshot(); len(got) != 4 || got[3].Content != "on the branch" {
		t.Fatalf("after switching to the head: %+v", got)
	}
	reloaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if ref, _ := reloaded.Head(); ref.HeadID != head {
		t.Fatalf("reload must land on the switched head, got %+v", ref)
	}
}

func TestForkAtTurnCreatesRewindHeadInSameLog(t *testing.T) {
	c, sess, path := newSchemaTwoBranchController(t)
	// A guarded turn opens a checkpoint boundary the fork can target.
	c.beginCheckpoint(t.Context(), "second prompt")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "second prompt"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "second answer"})
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	turn := -1
	for candidate := range 8 {
		if c.CheckpointHasBoundary(candidate) {
			turn = candidate
		}
	}
	if turn < 0 {
		t.Fatal("no checkpoint boundary recorded")
	}
	head, err := c.ForkNamed(turn, "")
	if err != nil {
		t.Fatalf("ForkNamed: %v", err)
	}
	if c.SessionPath() != path || strings.HasSuffix(head, ".jsonl") {
		t.Fatalf("fork must stay on the log: path %q head %q", c.SessionPath(), head)
	}
	if got := len(sess.Snapshot()); got != 3 {
		t.Fatalf("forked transcript has %d messages, want the prefix before the turn", got)
	}
	heads, err := agent.ListSessionHeads(path)
	if err != nil || len(heads) != 2 || heads[1].ID != head || heads[1].Kind != agent.HeadKindFork {
		t.Fatalf("heads = %+v err=%v", heads, err)
	}
	if heads[0].MessageCount != 5 {
		t.Fatalf("main head must keep its full chain, got %d messages", heads[0].MessageCount)
	}
}

func TestCheckpointBoundaryStaysWithItsConversationHead(t *testing.T) {
	c, sess, path := newSchemaTwoBranchController(t)
	c.beginCheckpoint(t.Context(), "second prompt")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "second prompt"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "second answer"})
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	metas := c.Checkpoints() // Binds a checkpoint created before message IDs were stored.
	if len(metas) != 1 || metas[0].MessageID == "" {
		t.Fatalf("checkpoint has no message identity: %+v", metas)
	}
	turn := metas[0].Turn
	mainID := agent.BranchID(path)
	plan, err := c.PrepareRewind(turn, RewindConversation)
	if err != nil || !plan.CanConversation || plan.PlanID == "" {
		t.Fatalf("main rewind plan = %+v err=%v", plan, err)
	}
	forkID, err := c.ForkNamed(turn, "alternative")
	if err != nil {
		t.Fatal(err)
	}
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "different prompt"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "different answer"})
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if got := c.Checkpoints(); len(got) != 0 || c.CheckpointHasBoundary(turn) {
		t.Fatalf("foreign checkpoint visible on fork: %+v", got)
	}
	if preview, err := c.PrepareRewind(turn, RewindConversation); err != nil || preview.PlanID != "" || preview.CanConversation {
		t.Fatalf("foreign checkpoint got rewind plan: %+v err=%v", preview, err)
	}
	if result, err := c.CommitRewind(plan.PlanID); err == nil || result.OK {
		t.Fatalf("plan from main committed on fork: %+v err=%v", result, err)
	}
	if _, err := c.ForkNamed(turn, "invalid"); err == nil {
		t.Fatal("fork accepted checkpoint from a different head")
	}
	if got := sess.Snapshot(); len(got) != 5 || got[3].Content != "different prompt" {
		t.Fatalf("foreign checkpoint changed fork: %+v", got)
	}
	if _, err := c.SwitchBranch(mainID); err != nil {
		t.Fatal(err)
	}
	if got := c.Checkpoints(); len(got) != 1 || got[0].MessageID != metas[0].MessageID {
		t.Fatalf("main checkpoint missing after switch: %+v", got)
	}
	c.Close()
	reloaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	exec := agent.New(nil, nil, reloaded, agent.Options{}, event.Discard)
	reopened := New(Options{Executor: exec, SessionDir: filepath.Dir(path), SessionPath: path, Sink: event.Discard})
	t.Cleanup(reopened.Close)
	if got := reopened.Checkpoints(); len(got) != 1 || got[0].MessageID != metas[0].MessageID {
		t.Fatalf("persisted main checkpoint missing: %+v", got)
	}
	if _, err := reopened.SwitchBranch(forkID); err != nil {
		t.Fatal(err)
	}
	if got := reopened.Checkpoints(); len(got) != 0 || reopened.CheckpointHasBoundary(turn) {
		t.Fatalf("foreign checkpoint reappeared after reopen: %+v", got)
	}
}

func TestFileBranchesOnlyKeepsFileBranchesForSchemaTwo(t *testing.T) {
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	exec.Session().Add(provider.Message{Role: provider.RoleUser, Content: "root prompt"})
	c := New(Options{Executor: exec, SessionDir: dir, Label: "test", Sink: event.Discard, FileBranchesOnly: true})
	path := filepath.Join(dir, "root.jsonl")
	c.SetSessionPath(path)
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	branchPath, err := c.Branch("child")
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}
	if branchPath == path || !strings.HasSuffix(branchPath, ".jsonl") || c.SessionPath() != branchPath {
		t.Fatalf("FileBranchesOnly must keep file branches: returned %q, session path %q", branchPath, c.SessionPath())
	}
	if heads, _ := agent.ListSessionHeads(path); len(heads) != 1 {
		t.Fatalf("file branch must not add heads to the source log: %+v", heads)
	}
}

func TestCommitRewindInPlaceForksRewindHeadAndKeepsController(t *testing.T) {
	c, sess, path := newSchemaTwoBranchController(t)
	c.beginCheckpoint(t.Context(), "second prompt")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "second prompt"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "second answer"})
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	turn := -1
	for candidate := range 8 {
		if c.CheckpointHasBoundary(candidate) {
			turn = candidate
		}
	}
	plan, err := c.PrepareRewind(turn, RewindConversation)
	if err != nil || !plan.CanConversation {
		t.Fatalf("PrepareRewind = %+v err=%v", plan, err)
	}
	result, err := c.CommitRewindInPlace(plan.PlanID)
	if err != nil || !result.OK || !result.ConversationForked || result.Branch == "" || strings.HasSuffix(result.Branch, ".jsonl") {
		t.Fatalf("CommitRewindInPlace = %+v err=%v", result, err)
	}
	if c.SessionPath() != path || len(sess.Snapshot()) != 3 {
		t.Fatalf("controller after in-place rewind: path %q messages %d", c.SessionPath(), len(sess.Snapshot()))
	}
	heads, err := agent.ListSessionHeads(path)
	if err != nil || len(heads) != 2 || heads[1].ID != result.Branch || heads[1].Kind != agent.HeadKindRewind || !heads[1].Selected || heads[0].MessageCount != 5 {
		t.Fatalf("heads = %+v err=%v", heads, err)
	}
}

func TestBranchTreeMarksTheCurrentHead(t *testing.T) {
	c, _, path := newSchemaTwoBranchController(t)
	if got := c.CurrentBranchID(); got != agent.BranchID(path) {
		t.Fatalf("CurrentBranchID on main = %q, want the file id %q", got, agent.BranchID(path))
	}
	head, err := c.Branch("experiment")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.CurrentBranchID(); got != head {
		t.Fatalf("CurrentBranchID after Branch = %q, want the new head %q", got, head)
	}
	tree := c.BranchTreeText()
	for line := range strings.SplitSeq(tree, "\n") {
		if strings.Contains(line, "experiment") != strings.HasSuffix(line, "current") {
			t.Fatalf("tree marks the wrong branch current:\n%s", tree)
		}
	}
	if _, err := c.SwitchBranch(agent.BranchID(path)); err != nil {
		t.Fatal(err)
	}
	if got := c.CurrentBranchID(); got != agent.BranchID(path) {
		t.Fatalf("CurrentBranchID back on main = %q", got)
	}
}
