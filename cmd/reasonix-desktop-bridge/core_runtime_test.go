package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
	"reasonix/internal/sessionidentity"
	sessionstore "reasonix/internal/store"
)

// The bridge and the identity importer must not derive transcript paths
// separately: that drift is what the S0 fix removes. The importer side of this
// pair is asserted in internal/sessionidentity (its catalog test imports the
// same shared rule), so this test pins the writer side plus the validator.
func TestBridgeSessionPathMatchesTheSharedRule(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"tauri-abc123", "bare_id", "t-"} {
		got, err := bridgeSessionPath(dir, id)
		if err != nil {
			t.Fatalf("bridgeSessionPath(%q): %v", id, err)
		}
		want, err := sessionpath.TranscriptPath(dir, id)
		if err != nil {
			t.Fatalf("sessionpath.TranscriptPath(%q): %v", id, err)
		}
		if got != want {
			t.Fatalf("bridge path %q != shared path %q", got, want)
		}
		// Flat: a workspace never selects the directory.
		if expected := filepath.Join(dir, "tauri-"+id+".jsonl"); want != expected {
			t.Fatalf("shared path = %q, want %q", want, expected)
		}
	}
}

func TestBridgeSessionPathIsDeterministicAndContained(t *testing.T) {
	dir := t.TempDir()
	path, err := bridgeSessionPath(dir, "preview_42-a")
	if err != nil {
		t.Fatalf("bridge session path: %v", err)
	}
	want := filepath.Join(dir, "tauri-preview_42-a.jsonl")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := bridgeSessionPath(dir, "../outside"); err == nil {
		t.Fatal("unsafe bridge session ID produced a path")
	}
}

func TestRegisteredMissingTranscriptCannotBecomeFresh(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	path, err := bridgeSessionPath(sessionDir, "known")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: "known", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	factory := newControllerFactory(nil)
	workspace := t.TempDir()
	if runtime, err := factory.Open(ctx, desktopbridge.OpenRequest{SessionID: "known", WorkspaceRoot: workspace}); runtime != nil || !errors.Is(err, ErrKnownSessionMissing) {
		t.Fatalf("open registered missing session = %v, %v", runtime, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registered missing transcript was recreated: %v", err)
	}
	identityStore, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	record, registered, err := identityStore.Get(ctx, "known")
	_ = identityStore.Close()
	if err != nil || !registered || record.State != sessionidentity.StateMissing {
		t.Fatalf("deleted transcript identity state = %#v, %v, %v", record, registered, err)
	}
	runtime, err := factory.Open(ctx, desktopbridge.OpenRequest{SessionID: "new", WorkspaceRoot: workspace})
	if err != nil {
		t.Fatalf("unregistered new session was rejected: %v", err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	identityStore, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	newRecord, registered, err := identityStore.Get(ctx, "new")
	_ = identityStore.Close()
	if err != nil || !registered || newRecord.State != sessionidentity.StateReserved {
		t.Fatalf("fresh session was not reserved: %#v, %v, %v", newRecord, registered, err)
	}
}

func TestMissingIdentityWinsOverAResidualTranscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	path, err := bridgeSessionPath(sessionDir, "missing-but-restored")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	candidate := sessionidentity.Candidate{ID: "missing-but-restored", Path: path}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{candidate}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{candidate}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: candidate.ID, WorkspaceRoot: t.TempDir()})
	if runtime != nil || !errors.Is(err, ErrKnownSessionMissing) {
		t.Fatalf("open missing identity with restored transcript = %v, %v", runtime, err)
	}
}

func TestUnregisteredSessionResidueCannotBecomeFresh(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	path, err := bridgeSessionPath(sessionDir, "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionstore.SessionMeta(path), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "orphan", WorkspaceRoot: t.TempDir()})
	if runtime != nil || !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("open unregistered session with sidecar = %v, %v", runtime, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan transcript was created: %v", err)
	}
}

func TestUnregisteredExistingTranscriptIsClaimedReady(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	path, err := bridgeSessionPath(sessionDir, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: "legacy", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("resume legacy transcript: %v", err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	identityStore, err := sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer identityStore.Close()
	record, registered, err := identityStore.Get(context.Background(), "legacy")
	if err != nil || !registered || record.State != sessionidentity.StateReady {
		t.Fatalf("legacy identity = %#v, %v, %v", record, registered, err)
	}
}

func TestBridgeLifecycleSinkMarksReservedIdentityReadyAfterFirstSave(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := bridgeSessionPath(sessionDir, "first-save")
	if err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Reserve(ctx, sessionDir, sessionidentity.Candidate{ID: "first-save", Path: path}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink := newBridgeLifecycleSink(event.Discard, "first-save")
	sink.Emit(event.Event{Kind: event.TurnDone})
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "first-save")
	if err != nil || !exists || record.State != sessionidentity.StateReady {
		t.Fatalf("first transcript save did not advance reservation: %#v, %v, %v", record, exists, err)
	}
}

func TestMissingSessionFailsClosedWhenIdentityStoreIsUnreadable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	identityPath := appconfig.DesktopSessionIdentityPath()
	if err := os.MkdirAll(filepath.Dir(identityPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identityPath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory := newControllerFactory(nil)
	runtime, err := factory.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "unknown", WorkspaceRoot: t.TempDir()})
	if runtime != nil || err == nil {
		t.Fatalf("unreadable identity store allowed a fresh session: %v, %v", runtime, err)
	}
}

func TestTruncateBridgeHistoryContentPreservesUnicodeAndMarksTruncation(t *testing.T) {
	content := strings.Repeat("界", bridgeHistoryMaxContentRunes+1)
	got, truncated := truncateBridgeHistoryContent(content)
	if !truncated || !strings.HasPrefix(got, strings.Repeat("界", bridgeHistoryMaxContentRunes)) || !strings.HasSuffix(got, "[Preview truncated this message]") {
		t.Fatalf("history truncation = %q, %v", got, truncated)
	}
}

func TestBridgeHistoryProjectionHidesHostSessionContext(t *testing.T) {
	snapshot := sessioncontext.Build(sessioncontext.Sections{
		Environment: "darwin/arm64",
		Workspace:   "Current workspace: /tmp/project",
	})
	if !sessioncontext.IsContent(snapshot.Content) {
		t.Fatal("test fixture is not a valid session-context snapshot")
	}
	if sessioncontext.IsContent("the user's actual question") {
		t.Fatal("ordinary user text was classified as session context")
	}
	// Keep this contract close to the bridge's projection rule: the host
	// snapshot is omitted while the adjacent user question remains visible.
	visible := []provider.Message{
		{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: snapshot.Content},
		{Role: provider.RoleUser, Origin: provider.MessageOriginUser, Content: "<reasoning-language>\n必须使用简体中文\n</reasoning-language>\n\nthe user's actual question"},
	}
	projected := make([]string, 0, len(visible))
	for _, message := range visible {
		if !bridgeHistoryMessageVisible(message) {
			continue
		}
		projected = append(projected, bridgeHistoryMessageContent(message))
	}
	if len(projected) != 1 || projected[0] != "the user's actual question" {
		t.Fatalf("projected bridge history = %#v", projected)
	}
}

func TestControllerRuntimeRenamesSessionMetadataWithoutTouchingTranscript(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("REASONIX_HOME", t.TempDir())
	open := func() desktopbridge.Runtime {
		factory := newControllerFactory(nil)
		factory.base.WorkspaceRoot = workspace
		runtime, err := factory.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "tab-title"})
		if err != nil {
			t.Fatalf("open session: %v", err)
		}
		t.Cleanup(func() { _ = runtime.Shutdown() })
		return runtime
	}

	runtime := open()
	if got := runtime.Title(); got != "" {
		t.Fatalf("untitled session title = %q", got)
	}
	sessionPath := runtime.SessionPath()
	transcriptBefore, err := os.ReadFile(sessionPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := runtime.Rename("Release notes"); err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if got := runtime.Title(); got != "Release notes" {
		t.Fatalf("renamed session title = %q", got)
	}
	if _, ok, err := agent.LoadBranchMeta(sessionPath); err != nil || !ok {
		t.Fatalf("load branch meta = ok %v, err %v", ok, err)
	}
	transcriptAfter, err := os.ReadFile(sessionPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if string(transcriptAfter) != string(transcriptBefore) {
		t.Fatal("rename rewrote the session transcript")
	}

	restored := open()
	if restored.SessionPath() != sessionPath || restored.Title() != "Release notes" {
		t.Fatalf("restored session = %q, %q", restored.SessionPath(), restored.Title())
	}
}

func TestControllerRuntimeDeleteRemovesSessionArtifacts(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("REASONIX_HOME", t.TempDir())
	open := func(sessionID string) desktopbridge.Runtime {
		factory := newControllerFactory(nil)
		factory.base.WorkspaceRoot = workspace
		runtime, err := factory.Open(context.Background(), desktopbridge.OpenRequest{SessionID: sessionID})
		if err != nil {
			t.Fatalf("open session: %v", err)
		}
		return runtime
	}

	doomed := open("tab-doomed")
	if err := doomed.Rename("Scratch"); err != nil {
		t.Fatalf("rename doomed session: %v", err)
	}
	doomedPath := doomed.SessionPath()
	if _, ok, err := agent.LoadBranchMeta(doomedPath); err != nil || !ok {
		t.Fatalf("session metadata before delete = ok %v, err %v", ok, err)
	}
	// Collect the artifacts that actually exist: a session with no turn yet has
	// no transcript, so the test must assert on the observed set rather than an
	// assumed file list.
	artifacts, err := filepath.Glob(doomedPath + "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) == 0 {
		t.Fatal("the session owns no artifacts to delete")
	}

	if err := doomed.Delete(); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	// A second sweep of an already-removed session must stay successful, so a
	// retried bridge request cannot report a completed delete as a failure.
	if err := doomed.Delete(); err != nil {
		t.Fatalf("repeat delete: %v", err)
	}
	for _, artifact := range artifacts {
		if _, err := os.Stat(artifact); !os.IsNotExist(err) {
			t.Fatalf("artifact survived delete: %s (err = %v)", artifact, err)
		}
	}

	survivor := open("tab-survivor")
	t.Cleanup(func() { _ = survivor.Shutdown() })
	if survivor.SessionPath() == doomedPath {
		t.Fatal("the surviving session shares the deleted path")
	}
	if err := survivor.Rename("Kept"); err != nil {
		t.Fatalf("rename survivor: %v", err)
	}
	if _, ok, err := agent.LoadBranchMeta(survivor.SessionPath()); err != nil || !ok {
		t.Fatalf("deleting one session removed another: ok %v, err %v", ok, err)
	}
}

func TestControllerRuntimeAttachFileCopiesIntoSessionWorkspace(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "research notes.txt")
	if err := os.WriteFile(source, []byte("selected context"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: workspace, Sink: event.Discard})
	if err != nil {
		t.Fatalf("build controller: %v", err)
	}
	t.Cleanup(controller.Close)

	runtime := &controllerRuntime{controller: controller}
	attachment, err := runtime.AttachFile(source)
	if err != nil {
		t.Fatalf("attach file: %v", err)
	}
	if attachment.Name != "research notes.txt" || attachment.Path == source || !strings.HasPrefix(attachment.Path, ".reasonix/attachments/") || attachment.Size != int64(len("selected context")) || attachment.IsImage {
		t.Fatalf("attachment metadata = %#v", attachment)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(attachment.Path)))
	if err != nil || string(data) != "selected context" {
		t.Fatalf("copied attachment content = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".reasonix", "attachments")); err != nil {
		t.Fatalf("workspace attachment directory: %v", err)
	}
}

func TestControllerRuntimeListsBoundedWorkspaceEntries(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("REASONIX_HOME", t.TempDir())
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"README.md":   "read me",
		"src/main.go": "package main",
		".git/config": "private",
	} {
		path := filepath.Join(workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: workspace, Sink: event.Discard})
	if err != nil {
		t.Fatalf("build controller: %v", err)
	}
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	listing, err := runtime.ListWorkspace("")
	if err != nil {
		t.Fatalf("list workspace: %v", err)
	}
	if listing.Path != "" || len(listing.Entries) != 2 || listing.Entries[0].Name != "src" || !listing.Entries[0].IsDir || listing.Entries[1].Path != "README.md" {
		t.Fatalf("root listing = %#v", listing)
	}
	child, err := runtime.ListWorkspace("src")
	if err != nil || len(child.Entries) != 1 || child.Entries[0].Path != "src/main.go" {
		t.Fatalf("child listing = %#v, err = %v", child, err)
	}
	if _, err := runtime.ListWorkspace("../"); err == nil {
		t.Fatal("workspace listing accepted a path outside the root")
	}
}

func TestControllerRuntimePreviewsSafeWorkspaceFiles(t *testing.T) {
	workspace := t.TempDir()
	textPath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(textPath, []byte("hello\nworld"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "image.bin"), []byte{'a', 0, 'b'}, 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: workspace, Sink: event.Discard})
	if err != nil {
		t.Fatalf("build controller: %v", err)
	}
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	preview, err := runtime.ReadWorkspaceFile("notes.txt")
	if err != nil || preview.Path != "notes.txt" || preview.Body != "hello\nworld" || preview.Size != 11 || preview.Binary || preview.Truncated {
		t.Fatalf("text preview = %#v, err = %v", preview, err)
	}
	binary, err := runtime.ReadWorkspaceFile("image.bin")
	if err != nil || !binary.Binary || binary.Body != "" || binary.Size != 3 {
		t.Fatalf("binary preview = %#v, err = %v", binary, err)
	}
	if _, err := runtime.ReadWorkspaceFile("../outside.txt"); err == nil {
		t.Fatal("workspace file preview accepted a path outside the root")
	}
}

func TestControllerRuntimeListsAndPreviewsGitChanges(t *testing.T) {
	repo := t.TempDir()
	runGitTest := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGitTest("init", "-q")
	runGitTest("config", "user.email", "test@example.com")
	runGitTest("config", "user.name", "Reasonix Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest("add", "tracked.txt")
	runGitTest("commit", "-qm", "initial")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("created\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := boot.Build(context.Background(), boot.Options{WorkspaceRoot: repo, Sink: event.Discard})
	if err != nil {
		t.Fatalf("build controller: %v", err)
	}
	t.Cleanup(controller.Close)
	runtime := &controllerRuntime{controller: controller}
	changes := runtime.WorkspaceChanges()
	if !changes.GitAvailable || changes.GitBranch == "" || len(changes.Files) != 2 {
		t.Fatalf("workspace changes = %#v", changes)
	}
	detail, err := runtime.WorkspaceChangeDetail("tracked.txt")
	if err != nil || detail.Source != "git" || !strings.Contains(detail.Diff, "+new") || detail.Added != 1 || detail.Removed != 1 {
		t.Fatalf("tracked detail = %#v, err = %v", detail, err)
	}
	untracked, err := runtime.WorkspaceChangeDetail("new.txt")
	if err != nil || untracked.Source != "git" || !strings.Contains(untracked.Diff, "+created") {
		t.Fatalf("untracked detail = %#v, err = %v", untracked, err)
	}
}

func TestBridgeSessionChangeDetailHandlesCreatedAndDeletedFiles(t *testing.T) {
	workspace := t.TempDir()
	created := filepath.Join(workspace, "created.txt")
	if err := os.WriteFile(created, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	createdDetail, err := bridgeSessionChangeDetail(workspace, "created.txt", nil)
	if err != nil || createdDetail.Source != "session" || !strings.Contains(createdDetail.Diff, "+new") || createdDetail.Added != 1 {
		t.Fatalf("created session detail = %#v, err = %v", createdDetail, err)
	}

	deletedDetail, err := bridgeSessionChangeDetail(workspace, "deleted.txt", func() *string {
		old := "old\n"
		return &old
	}())
	if err != nil || deletedDetail.Source != "session" || !strings.Contains(deletedDetail.Diff, "-old") || deletedDetail.Removed != 1 {
		t.Fatalf("deleted session detail = %#v, err = %v", deletedDetail, err)
	}
}
