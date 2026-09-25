package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"reasonix/internal/guardian"
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

func TestBridgeSessionResidueDetectsAllDurableArtifacts(t *testing.T) {
	root := t.TempDir()
	transcript := filepath.Join(root, "tauri-residue.jsonl")
	guardianPath := guardian.PathFor(transcript)
	for _, artifact := range []string{
		sessionstore.SessionEventLog(transcript),
		sessionstore.SessionGoalState(transcript),
		sessionstore.SessionEventIndex(transcript),
		sessionstore.SessionCheckpointDir(transcript),
		sessionstore.SessionJobsDir(transcript),
		sessionstore.SessionInboxDir(transcript),
		sessionstore.SessionCleanupPending(transcript),
		guardianPath,
		guardian.CursorPathFor(transcript),
		sessionstore.SessionEventLog(guardianPath),
	} {
		t.Run(filepath.Base(artifact), func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
				t.Fatal(err)
			}
			if artifact == sessionstore.SessionCheckpointDir(transcript) || artifact == sessionstore.SessionJobsDir(transcript) || artifact == sessionstore.SessionInboxDir(transcript) {
				if err := os.MkdirAll(artifact, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(artifact, []byte("residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := hasBridgeSessionResidue(transcript)
			if err != nil || !got {
				t.Fatalf("hasBridgeSessionResidue = %v, %v; want true, nil", got, err)
			}
		})
		if err := os.RemoveAll(artifact); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := hasBridgeSessionResidue(transcript); err != nil || got {
		t.Fatalf("empty session residue = %v, %v; want false, nil", got, err)
	}
}

func TestBridgeSessionResidueDetectsOwnedSubagents(t *testing.T) {
	sessionDir := t.TempDir()
	transcript := filepath.Join(sessionDir, "tauri-parent.jsonl")
	subagentDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subagentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(agent.SubagentMeta{
		Ref: "sa_residue_test", ParentSession: agent.BranchID(transcript), Status: agent.SubagentCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagentDir, "sa_residue_test.meta.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := hasBridgeSessionResidue(transcript)
	if err != nil || !got {
		t.Fatalf("hasBridgeSessionResidue with owned subagent = %v, %v; want true, nil", got, err)
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
	identityStore, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
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
	identityStore, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
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

func TestPhysicalIdentityConflictIsReportedAsSessionConflict(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	path, err := bridgeSessionPath(sessionDir, "new-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := []byte("{}\n")
	if err := os.WriteFile(path, transcript, 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: "legacy-owner", Path: path}}); err != nil {
		_ = identities.Close()
		t.Fatalf("seed existing path owner: %v", err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{
		SessionID: "new-id", WorkspaceRoot: t.TempDir(),
	})
	if runtime != nil || !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("open colliding identity = %v, %v; want session conflict", runtime, err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(transcript) {
		t.Fatalf("conflicting session path changed: %q, %v", after, err)
	}
}

func TestDeletePhysicalPathConflictDoesNotSweepTranscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	path, err := bridgeSessionPath(sessionDir, "delete-target")
	if err != nil {
		t.Fatal(err)
	}
	transcript := []byte("{}\n")
	if err := os.WriteFile(path, transcript, 0o600); err != nil {
		t.Fatal(err)
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	identities, err := sessionidentity.Open(ctx, identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", identityPath)
	if err != nil {
		t.Fatal(err)
	}
	for position, identity := range []struct{ id, path string }{
		{id: "delete-target", path: path},
	} {
		relative, err := filepath.Rel(appconfig.SessionProfileRoot(), identity.path)
		if err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, identity.id, filepath.ToSlash(relative), position); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{
		SessionID: "delete-target", WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("open legacy duplicate identity: %v", err)
	}
	defer runtime.Shutdown()
	db, err = sql.Open("sqlite", identityPath)
	if err != nil {
		t.Fatal(err)
	}
	ownerRelative, err := filepath.Rel(appconfig.SessionProfileRoot(), filepath.Join(aliasDir, filepath.Base(path)))
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions
		(id, relative_path, position, state, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, 'ready', 0, 0)`, "legacy-owner", filepath.ToSlash(ownerRelative), 1); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	conflictingRuntime, openErr := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{
		SessionID: "delete-target", WorkspaceRoot: t.TempDir(),
	})
	if conflictingRuntime != nil || !errors.Is(openErr, desktopbridge.ErrSessionConflict) {
		if conflictingRuntime != nil {
			_ = conflictingRuntime.Shutdown()
		}
		t.Fatalf("open legacy duplicate identity = %v, %v; want session conflict", conflictingRuntime, openErr)
	}
	if err := runtime.Delete(); !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("delete aliased identity = %v, want session conflict", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(transcript) {
		t.Fatalf("physical conflict delete swept transcript: %q, %v", after, err)
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
	identityStore, err := sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
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
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
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
	identities, err := sessionidentity.OpenReadOnly(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatalf("open session identity: %v", err)
	}
	identity, exists, err := identities.Get(context.Background(), "tab-title")
	_ = identities.Close()
	if err != nil || !exists || identity.Title != "Release notes" || identity.TitleSource != sessionidentity.TitleUser {
		t.Fatalf("renamed session identity = %#v, exists %v, err %v", identity, exists, err)
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

func TestControllerRuntimeRenameLeavesMetadataWhenIdentityStoreUnavailable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: "rename-store-unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Rename("Original title"); err != nil {
		t.Fatal(err)
	}
	metaPath := sessionstore.SessionMeta(runtime.SessionPath())
	metaBefore, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	parkedPath := identityPath + ".parked"
	if err := os.Rename(identityPath, parkedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(identityPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(identityPath)
		_ = os.Rename(parkedPath, identityPath)
	})
	if err := runtime.Rename("Should not persist"); err == nil {
		t.Fatal("rename succeeded with an unavailable identity store")
	}
	if got := runtime.Title(); got != "Original title" {
		t.Fatalf("failed rename changed sidecar title to %q", got)
	}
	metaAfter, err := os.ReadFile(metaPath)
	if err != nil || string(metaAfter) != string(metaBefore) {
		t.Fatalf("failed rename changed sidecar bytes: read error=%v", err)
	}
}

func TestControllerRuntimeRenameRestoresSidecarAfterRejectedIdentityWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "rename-write-rejected"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Rename("Original title"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_title_update BEFORE UPDATE OF title ON sessions
		BEGIN SELECT RAISE(ABORT, 'injected title write failure'); END`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Rename("Should not persist"); err == nil {
		t.Fatal("rename succeeded despite rejected identity write")
	}
	if got := runtime.Title(); got != "Original title" {
		t.Fatalf("failed rename left sidecar title %q", got)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "rename-write-rejected")
	if err != nil || !exists || record.Title != "Original title" {
		t.Fatalf("failed rename changed identity: %#v exists=%v err=%v", record, exists, err)
	}
}

func TestControllerRuntimeRenameRejectsChangedIdentityPathBeforeSidecarWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	runtime, err := newControllerFactory(nil).Open(context.Background(), desktopbridge.OpenRequest{SessionID: "rename-path-changed"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Rename("Original title"); err != nil {
		t.Fatal(err)
	}
	metaPath := sessionstore.SessionMeta(runtime.SessionPath())
	metaBefore, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	originalRelative, err := filepath.Rel(root, runtime.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(`UPDATE sessions SET relative_path=? WHERE id='rename-path-changed'`, filepath.ToSlash(originalRelative))
	}()
	if _, err := db.Exec(`UPDATE sessions SET relative_path='sessions/other.jsonl' WHERE id='rename-path-changed'`); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Rename("Should not persist"); !errors.Is(err, sessionidentity.ErrPathChanged) {
		t.Fatalf("rename with changed identity path = %v, want ErrPathChanged", err)
	}
	metaAfter, err := os.ReadFile(metaPath)
	if err != nil || string(metaAfter) != string(metaBefore) {
		t.Fatalf("changed identity path rewrote sidecar: read error=%v", err)
	}
}

func TestControllerRuntimeRenameRejectsSymlinkedTitleSidecar(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "rename-meta-link"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Rename("Original title"); err != nil {
		t.Fatal(err)
	}
	metaPath := sessionstore.SessionMeta(runtime.SessionPath())
	originalMeta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.meta")
	outsideBytes := []byte(`{"custom_title":"outside private title"}`)
	if err := os.WriteFile(outside, outsideBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(metaPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, metaPath); err != nil {
		t.Skipf("sidecar symlinks unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(metaPath)
		_ = os.WriteFile(metaPath, originalMeta, 0o600)
	})
	if err := runtime.Rename("Should not persist"); !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("rename through sidecar symlink = %v, want session conflict", err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != string(outsideBytes) {
		t.Fatalf("external sidecar changed: %q, %v", got, err)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	if _, pending, err := identities.PendingManualTitleRename(ctx, "rename-meta-link"); err != nil || pending {
		t.Fatalf("rejected sidecar link created intent: pending=%v err=%v", pending, err)
	}
}

func TestControllerRuntimeRenameRejectsTitleChangedBySidecarRead(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "rename-sanitized-title"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Rename("Original title"); err != nil {
		t.Fatal(err)
	}
	metaPath := sessionstore.SessionMeta(runtime.SessionPath())
	before, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	// The regular title validator accepts this text, but LoadBranchMeta
	// strips the trailing memory-recall block on every read.
	unstable := "Chosen <memory-recall>hidden</memory-recall>"
	if got := agent.UserPreviewText(unstable); got != "Chosen" {
		t.Fatalf("test title no longer exercises sanitization: %q", got)
	}
	if err := runtime.Rename(unstable); !errors.Is(err, desktopbridge.ErrInvalidTitle) {
		t.Fatalf("unstable title rename = %v, want invalid title", err)
	}
	after, err := os.ReadFile(metaPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("rejected title changed sidecar bytes: %v", err)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	if _, pending, err := identities.PendingManualTitleRename(ctx, "rename-sanitized-title"); err != nil || pending {
		t.Fatalf("rejected title created intent: pending=%v err=%v", pending, err)
	}
	record, exists, err := identities.Get(ctx, "rename-sanitized-title")
	if err != nil || !exists || record.Title != "Original title" || record.TitleRevision != 1 {
		t.Fatalf("rejected title changed identity: %#v exists=%v err=%v", record, exists, err)
	}
}

func TestBridgeOpenRecoversDurableManualTitleIntent(t *testing.T) {
	for _, test := range []struct {
		name, sidecarTitle, wantTitle string
		wantPending                   bool
		wantOpenError                 bool
	}{
		{name: "sidecar committed", sidecarTitle: "New title", wantTitle: "New title"},
		{name: "sidecar unchanged", wantTitle: "", wantPending: false},
		{name: "sidecar changed again", sidecarTitle: "Third title", wantPending: true, wantOpenError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("REASONIX_HOME", root)
			t.Setenv("REASONIX_STATE_HOME", root)
			ctx := context.Background()
			sessionDir := appconfig.SessionDir()
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				t.Fatal(err)
			}
			path, err := bridgeSessionPath(sessionDir, "pending-rename")
			if err != nil {
				t.Fatal(err)
			}
			identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
			if err != nil {
				t.Fatal(err)
			}
			if err := identities.Reserve(ctx, sessionDir, sessionidentity.Candidate{ID: "pending-rename", Path: path}); err != nil {
				t.Fatal(err)
			}
			if err := identities.BeginManualTitleRename(ctx, "pending-rename", path, "", "New title", 0); err != nil {
				t.Fatal(err)
			}
			if err := identities.Close(); err != nil {
				t.Fatal(err)
			}
			if test.sidecarTitle != "" {
				if err := agent.RenameSession(path, test.sidecarTitle); err != nil {
					t.Fatal(err)
				}
			}
			runtime, openErr := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "pending-rename"})
			if test.wantOpenError {
				if openErr == nil {
					_ = runtime.Shutdown()
					t.Fatal("ambiguous title intent reopened")
				}
			} else {
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer runtime.Shutdown()
				if got := runtime.Title(); got != test.wantTitle {
					t.Fatalf("recovered runtime title = %q, want %q", got, test.wantTitle)
				}
			}
			identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
			if err != nil {
				t.Fatal(err)
			}
			defer identities.Close()
			intent, pending, err := identities.PendingManualTitleRename(ctx, "pending-rename")
			if err != nil || pending != test.wantPending {
				t.Fatalf("recovered intent = %#v pending=%v err=%v", intent, pending, err)
			}
			record, exists, err := identities.Get(ctx, "pending-rename")
			if err != nil || !exists || record.Title != test.wantTitle {
				t.Fatalf("recovered identity = %#v exists=%v err=%v", record, exists, err)
			}
		})
	}
}

func TestBridgeRecoversManualTitleIntentAfterAbruptWriterExit(t *testing.T) {
	const childMarker = "REASONIX_TEST_TITLE_INTENT_ABRUPT_EXIT"
	if os.Getenv(childMarker) == "1" {
		ctx := context.Background()
		sessionDir := appconfig.SessionDir()
		if err := os.MkdirAll(sessionDir, 0o700); err != nil {
			t.Fatal(err)
		}
		path, err := bridgeSessionPath(sessionDir, "abrupt-title")
		if err != nil {
			t.Fatal(err)
		}
		identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
		if err != nil {
			t.Fatal(err)
		}
		if err := identities.Reserve(ctx, sessionDir, sessionidentity.Candidate{ID: "abrupt-title", Path: path}); err != nil {
			t.Fatal(err)
		}
		if err := identities.BeginManualTitleRename(ctx, "abrupt-title", path, "", "Recovered after exit", 0); err != nil {
			t.Fatal(err)
		}
		if err := agent.RenameSession(path, "Recovered after exit"); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Deliberately skip Store.Close and the title commit.
	}
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	cmd := exec.Command(os.Args[0], "-test.run=^TestBridgeRecoversManualTitleIntentAfterAbruptWriterExit$")
	cmd.Env = append(os.Environ(), childMarker+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("abrupt title writer failed: %v\n%s", err, output)
	}
	ctx := context.Background()
	runtime, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: "abrupt-title"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	if got := runtime.Title(); got != "Recovered after exit" {
		t.Fatalf("recovered title after abrupt exit = %q", got)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "abrupt-title")
	if err != nil || !exists || record.Title != "Recovered after exit" || record.TitleSource != sessionidentity.TitleUser {
		t.Fatalf("recovered identity = %#v exists=%v err=%v", record, exists, err)
	}
	if _, pending, err := identities.PendingManualTitleRename(ctx, "abrupt-title"); err != nil || pending {
		t.Fatalf("title intent remains after recovery: pending=%v err=%v", pending, err)
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

func TestBridgeDeletionFenceRejectsWritesAndRetriesCleanup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	var owned *controllerRuntime
	factory := desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		runtime, err := newControllerFactory(nil).Open(ctx, request)
		if err != nil {
			return nil, err
		}
		owned = runtime.(*controllerRuntime)
		owned.removeArtifacts = func(string) error { return errors.New("injected artifact sweep failure") }
		return runtime, nil
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	ctx := context.Background()
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "delete-retry", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteSession("delete-retry"); err == nil {
		t.Fatal("injected artifact sweep failure was ignored")
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	record, exists, err := identities.Get(ctx, "delete-retry")
	_ = identities.Close()
	if err != nil || !exists || record.State != sessionidentity.StateDeleting {
		t.Fatalf("failed deletion state = %#v, %v, %v", record, exists, err)
	}
	if _, err := manager.Submit("delete-retry", "must not be sent"); !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("submit during deletion = %v", err)
	}
	if _, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "delete-retry", WorkspaceRoot: view.WorkspaceRoot}); !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("reopen during deletion = %v", err)
	}
	owned.removeArtifacts = nil
	if err := manager.DeleteSession("delete-retry"); err != nil {
		t.Fatalf("retry deletion: %v", err)
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	record, exists, err = identities.Get(ctx, "delete-retry")
	_ = identities.Close()
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("completed deletion tombstone = %#v, %v, %v", record, exists, err)
	}
	if _, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "delete-retry", WorkspaceRoot: view.WorkspaceRoot}); !errors.Is(err, desktopbridge.ErrSessionConflict) {
		t.Fatalf("tombstoned session reopened: %v", err)
	}
}

func TestBridgeShutdownDoesNotSnapshotDeletingSession(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	factory := desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		runtime, err := newControllerFactory(nil).Open(ctx, request)
		if err != nil {
			return nil, err
		}
		runtime.(*controllerRuntime).removeArtifacts = func(string) error { return errors.New("injected artifact sweep failure") }
		return runtime, nil
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	ctx := context.Background()
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "delete-shutdown", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteSession("delete-shutdown"); err == nil {
		t.Fatal("injected artifact sweep failure was ignored")
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(view.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shutdown recreated deleting transcript: %v", err)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "delete-shutdown")
	if err != nil || !exists || record.State != sessionidentity.StateDeleting {
		t.Fatalf("shutdown deletion state = %#v, %v, %v", record, exists, err)
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
