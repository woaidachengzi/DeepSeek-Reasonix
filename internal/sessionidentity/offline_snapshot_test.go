package sessionidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCopyVerifiedSnapshotFileHashesBytesWhileCopying(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.jsonl")
	destination := filepath.Join(root, "snapshot.jsonl")
	contents := []byte("{\"role\":\"user\",\"content\":\"snapshot\"}\n")
	if err := os.WriteFile(source, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := copyVerifiedSnapshotFile(context.Background(), source, destination)
	if err != nil {
		t.Fatalf("copyVerifiedSnapshotFile: %v", err)
	}
	expectedHash := sha256.Sum256(contents)
	if result.Size != int64(len(contents)) || result.SHA256 != hex.EncodeToString(expectedHash[:]) {
		t.Fatalf("snapshot file metadata = %#v; want size %d and streamed SHA-256", result, len(contents))
	}
	copied, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(contents) {
		t.Fatalf("copied contents = %q; want %q", copied, contents)
	}
}

func TestOfflineSnapshotVerifiesAndStagesCompleteProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "preview-profile")
	sessionDir := filepath.Join(profile, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-tauri-first.jsonl")
	writeTranscript(t, transcript)
	sidecar := transcript + ".meta"
	if err := os.WriteFile(sidecar, []byte("metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(profile, "empty-memory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "MEMORY.md"), []byte("long-term memory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "run-helper.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(profile, "desktop", "session-state-v1.sqlite")
	workspace := filepath.Join(root, "project-workspace")
	writer, err := Open(ctx, identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Import(ctx, sessionDir, []Candidate{{ID: "tauri-first", Path: transcript}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-first", Title: "First", WorkspaceRoot: workspace}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyOfflineSnapshot(ctx, snapshot)
	if err != nil || manifest.Version != offlineSnapshotVersion || len(manifest.Files) < 4 {
		t.Fatalf("verified snapshot = %#v, %v", manifest, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(snapshot)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("snapshot permissions = %#v, %v", info, err)
		}
	}
	recoveryParent := filepath.Join(root, "recovery")
	if err := os.Mkdir(recoveryParent, 0o700); err != nil {
		t.Fatal(err)
	}
	staged, err := StageOfflineSnapshot(ctx, snapshot, recoveryParent)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		executable, err := os.Stat(filepath.Join(staged, "profile", "run-helper.sh"))
		if err != nil || executable.Mode().Perm() != 0o700 {
			t.Fatalf("staged executable permissions = %#v, %v", executable, err)
		}
	}
	if info, err := os.Stat(filepath.Join(staged, "profile", "empty-memory")); err != nil || !info.IsDir() {
		t.Fatalf("empty profile directory was not restored: %#v, %v", info, err)
	}
	for _, rel := range []string{"sessions/tauri-tauri-first.jsonl", "sessions/tauri-tauri-first.jsonl.meta", "MEMORY.md"} {
		original, err := os.ReadFile(filepath.Join(profile, rel))
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := os.ReadFile(filepath.Join(staged, "profile", rel))
		if err != nil || !reflect.DeepEqual(original, recovered) {
			t.Fatalf("staged recovery differs at %s: %v", rel, err)
		}
	}
	originalCatalog, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	recoveredCatalog, err := os.ReadFile(filepath.Join(staged, "catalog", "workbench-sessions.json"))
	if err != nil || !reflect.DeepEqual(originalCatalog, recoveredCatalog) {
		t.Fatalf("catalog recovery differs: %v", err)
	}
	restoredDB, err := OpenReadOnly(ctx, filepath.Join(staged, "profile", "desktop", "session-state-v1.sqlite"), filepath.Join(staged, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := restoredDB.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != "tauri-first" {
		t.Fatalf("restored identity database = %#v, %v", records, err)
	}
	wantRestoredTranscript := filepath.Join(staged, "profile", "sessions", "tauri-tauri-first.jsonl")
	if records[0].Path != wantRestoredTranscript {
		t.Fatalf("restored identity path = %q, want relocated profile path %q", records[0].Path, wantRestoredTranscript)
	}
	if err := restoredDB.Close(); err != nil {
		t.Fatal(err)
	}
	relocated, err := Open(ctx, filepath.Join(staged, "profile", "desktop", "session-state-v1.sqlite"), filepath.Join(staged, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	defer relocated.Close()
	if err := relocated.Import(ctx, filepath.Join(staged, "profile", "sessions"), []Candidate{{ID: "tauri-first", Path: wantRestoredTranscript}}); err != nil {
		t.Fatalf("reconcile relocated identity: %v", err)
	}
	if err := relocated.Close(); err != nil {
		t.Fatal(err)
	}

	// Exercise the legacy-catalog recovery path against the relocated transcript
	// rather than merely checking that the catalog bytes were copied. A scratch
	// identity store avoids replacing or mutating the restored identity DB.
	replayed, err := Open(ctx, filepath.Join(staged, "profile", "desktop", "catalog-replay.sqlite"), filepath.Join(staged, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := replayed.ImportWorkbenchCatalog(ctx, filepath.Join(staged, "profile", "sessions"), filepath.Join(staged, "catalog", "workbench-sessions.json")); err != nil {
		_ = replayed.Close()
		t.Fatalf("replay restored workbench catalog: %v", err)
	}
	replayedRecords, err := replayed.List(ctx)
	if err != nil || len(replayedRecords) != 1 {
		_ = replayed.Close()
		t.Fatalf("replayed catalog records = %#v, %v", replayedRecords, err)
	}
	replayedRecord := replayedRecords[0]
	if replayedRecord.ID != "tauri-first" || replayedRecord.Path != wantRestoredTranscript ||
		replayedRecord.Title != "First" || replayedRecord.WorkspaceRoot != workspace || replayedRecord.State != StateReady {
		_ = replayed.Close()
		t.Fatalf("replayed catalog record = %#v", replayedRecord)
	}
	if err := replayed.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineSnapshotRestoresSQLiteSessionEventsAndWatermarks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "preview-profile")
	sessionDir := filepath.Join(profile, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(sessionDir, "tauri-tauri-event-recovery.jsonl")
	transcriptBytes := []byte("{\"role\":\"system\",\"content\":\"isolated recovery\"}\n")
	if err := os.WriteFile(transcript, transcriptBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	eventPath := transcript[:len(transcript)-len(".jsonl")] + ".events.jsonl"
	createdAt := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	eventPayload := json.RawMessage(`{"schema_version":2,"type":"log","at":"2026-09-26T00:00:00Z","generation":1}`)
	eventProjection := append(append([]byte(nil), eventPayload...), '\n')
	if err := os.WriteFile(eventPath, eventProjection, 0o600); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(profile, "desktop", "session-state-v1.sqlite")
	writer, err := Open(ctx, identityPath, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Import(ctx, sessionDir, []Candidate{{ID: "tauri-event-recovery", Path: transcript}}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.AppendEvents(ctx, "tauri-event-recovery", []SessionEvent{{
		ID: "event-log", Type: "log", CreatedAt: createdAt.UnixMilli(), Payload: eventPayload,
	}}); err != nil {
		t.Fatal(err)
	}
	eventDigest := sha256.Sum256(eventProjection)
	if err := writer.MarkEventProjection(ctx, "tauri-event-recovery", 1, 1, hex.EncodeToString(eventDigest[:])); err != nil {
		t.Fatal(err)
	}
	checkpointDigest := sha256.Sum256(transcriptBytes)
	if err := writer.MarkCheckpointProjection(ctx, "tauri-event-recovery", 1, 1, hex.EncodeToString(checkpointDigest[:])); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "workbench-sessions.json")
	if err := os.WriteFile(catalog, []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyOfflineSnapshot(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifestFiles := make(map[string]bool, len(manifest.Files))
	for _, item := range manifest.Files {
		manifestFiles[item.Path] = true
	}
	for _, path := range []string{
		"profile/desktop/session-state-v1.sqlite",
		"profile/sessions/tauri-tauri-event-recovery.events.jsonl",
		"profile/sessions/tauri-tauri-event-recovery.jsonl",
	} {
		if !manifestFiles[path] {
			t.Fatalf("offline snapshot omitted required session member %q", path)
		}
	}
	recoveryParent := filepath.Join(root, "recovery")
	if err := os.Mkdir(recoveryParent, 0o700); err != nil {
		t.Fatal(err)
	}
	staged, err := StageOfflineSnapshot(ctx, snapshot, recoveryParent)
	if err != nil {
		t.Fatal(err)
	}
	stagedProfile := filepath.Join(staged, "profile")
	stagedDB, err := OpenReadOnly(ctx, filepath.Join(stagedProfile, "desktop", "session-state-v1.sqlite"), stagedProfile)
	if err != nil {
		t.Fatalf("open staged event store: %v", err)
	}
	status, err := stagedDB.EventStreamStatus(ctx, "tauri-event-recovery")
	if err != nil || status.Generation != 1 || status.LastSequence != 1 || status.ProjectionSequence != 1 || status.ProjectionSHA256 != hex.EncodeToString(eventDigest[:]) ||
		status.CheckpointSequence != 1 || status.CheckpointSHA256 != hex.EncodeToString(checkpointDigest[:]) {
		t.Fatalf("restored event stream watermarks = %#v, %v", status, err)
	}
	rows, err := stagedDB.ReadEvents(ctx, "tauri-event-recovery", 0, 10)
	if err != nil || len(rows) != 1 || rows[0].Sequence != 1 || rows[0].ID != "event-log" || !reflect.DeepEqual(rows[0].Payload, eventPayload) {
		t.Fatalf("restored SQLite events = %#v, %v", rows, err)
	}
	if err := stagedDB.Close(); err != nil {
		t.Fatal(err)
	}
	stagedProjection, err := os.ReadFile(filepath.Join(stagedProfile, "sessions", "tauri-tauri-event-recovery.events.jsonl"))
	if err != nil || !reflect.DeepEqual(stagedProjection, eventProjection) {
		t.Fatalf("restored event projection differs: %v", err)
	}
	stagedTranscript, err := os.ReadFile(filepath.Join(stagedProfile, "sessions", "tauri-tauri-event-recovery.jsonl"))
	if err != nil || !reflect.DeepEqual(stagedTranscript, transcriptBytes) {
		t.Fatalf("restored transcript checkpoint differs: %v", err)
	}
	continued, err := Open(ctx, filepath.Join(stagedProfile, "desktop", "session-state-v1.sqlite"), stagedProfile)
	if err != nil {
		t.Fatalf("reopen staged event store for continuation: %v", err)
	}
	defer continued.Close()
	if _, err := continued.AppendEvents(ctx, "tauri-event-recovery", []SessionEvent{{
		ID: "event-next", Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"event-next"}`),
	}}); err != nil {
		t.Fatalf("append after offline restore: %v", err)
	}
	if last, err := continued.LastEventSequence(ctx, "tauri-event-recovery"); err != nil || last != 2 {
		t.Fatalf("restored stream continuation sequence = %d, %v; want 2", last, err)
	}
}

func TestOfflineSnapshotRestoresPendingManualTitleIntentAtNewProfilePath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "preview-profile")
	sessionDir := filepath.Join(profile, "sessions")
	transcript := filepath.Join(sessionDir, "tauri-tauri-title.jsonl")
	writeTranscript(t, transcript)
	sidecar := []byte(`{"id":"tauri-tauri-title","custom_title":"Chosen title"}`)
	if err := os.WriteFile(transcript+".meta", sidecar, 0o600); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(profile, "desktop", "session-state-v1.sqlite")
	writer, err := Open(ctx, identityPath, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Import(ctx, sessionDir, []Candidate{{ID: "tauri-title", Path: transcript}}); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.BeginManualTitleRename(ctx, "tauri-title", transcript, "", "Chosen title", 0); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-title"}})
	backupParent := filepath.Join(root, "backups")
	recoveryParent := filepath.Join(root, "recovery")
	for _, parent := range []string{backupParent, recoveryParent} {
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StageOfflineSnapshot(ctx, snapshot, recoveryParent)
	if err != nil {
		t.Fatal(err)
	}
	restoredProfile := filepath.Join(staged, "profile")
	restoredTranscript := filepath.Join(restoredProfile, "sessions", "tauri-tauri-title.jsonl")
	restoredSidecar, err := os.ReadFile(restoredTranscript + ".meta")
	if err != nil || !reflect.DeepEqual(restoredSidecar, sidecar) {
		t.Fatalf("restored title sidecar = %q, %v", restoredSidecar, err)
	}
	var restoredMeta struct {
		CustomTitle string `json:"custom_title"`
	}
	if err := json.Unmarshal(restoredSidecar, &restoredMeta); err != nil {
		t.Fatal(err)
	}
	restoredDBPath := filepath.Join(restoredProfile, "desktop", "session-state-v1.sqlite")
	reader, err := OpenReadOnly(ctx, restoredDBPath, restoredProfile)
	if err != nil {
		t.Fatal(err)
	}
	intent, pending, err := reader.PendingManualTitleRename(ctx, "tauri-title")
	if err != nil || !pending || intent.Path != restoredTranscript || intent.NewTitle != restoredMeta.CustomTitle ||
		intent.ExpectedRevision != 0 || intent.PreviousSidecarTitle != "" {
		_ = reader.Close()
		t.Fatalf("restored title intent = %#v pending=%v err=%v", intent, pending, err)
	}
	recoveries, err := reader.ListPendingManualTitleRecoveries(ctx)
	if err != nil || len(recoveries) != 1 || recoveries[0].ID != "tauri-title" || recoveries[0].State != StateReady {
		_ = reader.Close()
		t.Fatalf("restored title recovery list = %#v, %v", recoveries, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	restoredWriter, err := Open(ctx, restoredDBPath, restoredProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := restoredWriter.CommitManualTitleRename(ctx, "tauri-title", restoredTranscript); err != nil {
		_ = restoredWriter.Close()
		t.Fatalf("commit restored title intent: %v", err)
	}
	restoredRecord, exists, err := restoredWriter.Get(ctx, "tauri-title")
	if err != nil || !exists || restoredRecord.Title != "Chosen title" ||
		restoredRecord.TitleSource != TitleUser || restoredRecord.TitleRevision != 1 {
		_ = restoredWriter.Close()
		t.Fatalf("restored committed title = %#v exists=%v err=%v", restoredRecord, exists, err)
	}
	if _, pending, err := restoredWriter.PendingManualTitleRename(ctx, "tauri-title"); err != nil || pending {
		_ = restoredWriter.Close()
		t.Fatalf("restored title intent remained pending: pending=%v err=%v", pending, err)
	}
	if err := restoredWriter.Close(); err != nil {
		t.Fatal(err)
	}

	source, err := OpenReadOnly(ctx, identityPath, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if original, pending, err := source.PendingManualTitleRename(ctx, "tauri-title"); err != nil || !pending ||
		original.Path != transcript || original.NewTitle != "Chosen title" {
		t.Fatalf("source title intent changed during recovery: %#v pending=%v err=%v", original, pending, err)
	}
	if original, exists, err := source.Get(ctx, "tauri-title"); err != nil || !exists ||
		original.Title != "" || original.TitleRevision != 0 {
		t.Fatalf("source identity title changed during recovery: %#v exists=%v err=%v", original, exists, err)
	}
	if originalSidecar, err := os.ReadFile(transcript + ".meta"); err != nil || !reflect.DeepEqual(originalSidecar, sidecar) {
		t.Fatalf("source title sidecar changed during recovery: %q, %v", originalSidecar, err)
	}
}

func TestOfflineSnapshotRejectsTamperingAndNestedDestination(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	if _, err := CreateOfflineSnapshot(ctx, profile, catalog, filepath.Join(profile, "sessions")); err == nil {
		t.Fatal("snapshot accepted a destination inside its source profile")
	}
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(snapshot, "profile", "sessions", "tauri-tauri-one.jsonl")
	if err := os.WriteFile(member, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil {
		t.Fatal("tampered snapshot verified")
	}
	if _, err := StageOfflineSnapshot(ctx, snapshot, root); err == nil {
		t.Fatal("tampered snapshot was staged for recovery")
	}
	original, err := os.ReadFile(filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	if err != nil || string(original) != "{}\n" {
		t.Fatalf("source profile changed: %q, %v", original, err)
	}
}

func TestOfflineSnapshotRejectsNonPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not enforced on Windows")
	}
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(snapshot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil || !strings.Contains(err.Error(), "non-private permissions") {
		t.Fatalf("snapshot with public root permissions verified: %v", err)
	}
	if err := os.Chmod(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}

	member := filepath.Join(snapshot, "profile", "sessions", "tauri-tauri-one.jsonl")
	if err := os.Chmod(member, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil || !strings.Contains(err.Error(), "non-private permissions") {
		t.Fatalf("snapshot with public member permissions verified: %v", err)
	}
	if err := os.Chmod(member, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("private snapshot did not verify after restoring permissions: %v", err)
	}
}

func TestOfflineSnapshotRequiresCatalogOutsideProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(profile, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent); err == nil {
		t.Fatal("snapshot accepted a catalog that is already inside the profile")
	}
	entries, err := os.ReadDir(backupParent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected overlapping catalog left backup artifacts: %v", entries)
	}
}

func TestOfflineSnapshotRefusesToStageInsideSourceProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatal(err)
	}
	beforeNames := make([]string, len(beforeEntries))
	for i, entry := range beforeEntries {
		beforeNames[i] = entry.Name()
	}
	if _, err := StageOfflineSnapshot(ctx, snapshot, profile); err == nil {
		t.Fatal("staging accepted the snapshot's source profile as its destination")
	}
	afterEntries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatal(err)
	}
	afterNames := make([]string, len(afterEntries))
	for i, entry := range afterEntries {
		afterNames[i] = entry.Name()
	}
	if !reflect.DeepEqual(beforeNames, afterNames) {
		t.Fatalf("refused staging changed source profile entries: before=%v after=%v", beforeNames, afterNames)
	}
}

func TestOfflineSnapshotRefusesSourceProfileSymlinkAlias(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	profileAlias := filepath.Join(root, "profile-alias")
	if err := os.Symlink(profile, profileAlias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	manifestPath := filepath.Join(snapshot, "manifest.json")
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest SnapshotManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SourceProfile = profileAlias
	encoded, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatal(err)
	}
	beforeNames := make([]string, len(beforeEntries))
	for i, entry := range beforeEntries {
		beforeNames[i] = entry.Name()
	}
	if _, err := StageOfflineSnapshot(ctx, snapshot, profile); err == nil {
		t.Fatal("staging accepted a symlink alias of the snapshot's source profile")
	}
	afterEntries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatal(err)
	}
	afterNames := make([]string, len(afterEntries))
	for i, entry := range afterEntries {
		afterNames[i] = entry.Name()
	}
	if !reflect.DeepEqual(beforeNames, afterNames) {
		t.Fatalf("refused staging through a source alias changed profile entries: before=%v after=%v", beforeNames, afterNames)
	}
}

func TestOfflineSnapshotRejectsSymlinkAndLeavesIncompleteMarker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	if err := os.Symlink(catalog, filepath.Join(profile, "sessions", "linked-catalog")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err == nil || snapshot == "" {
		t.Fatalf("symlink source snapshot = %q, %v", snapshot, err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete snapshot has a valid manifest: %v", err)
	}
}

// A manifest that forgets a file on disk must not verify: recovery staging
// copies only manifest members, so an unlisted session would be dropped.
func TestOfflineSnapshotRejectsManifestMissingOnDiskFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	// Inject a session file that the published manifest never listed.
	ghost := filepath.Join(snapshot, "profile", "sessions", "tauri-tauri-ghost.jsonl")
	if err := os.WriteFile(ghost, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil {
		t.Fatal("verification accepted a snapshot whose manifest omitted an on-disk file")
	} else if !strings.Contains(err.Error(), "missing from the manifest") {
		t.Fatalf("verification error = %v, want an unlisted-member message", err)
	}
	if _, err := StageOfflineSnapshot(ctx, snapshot, root); err == nil {
		t.Fatal("staging accepted a snapshot whose manifest omitted an on-disk file")
	}
}

// An unlisted directory is equally fatal: empty dirs are part of the profile.
func TestOfflineSnapshotRejectsManifestMissingOnDiskDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, []catalogEntry{{SessionID: "tauri-one"}})
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
	if err != nil {
		t.Fatal(err)
	}
	ghostDir := filepath.Join(snapshot, "profile", "unlisted-empty")
	if err := os.Mkdir(ghostDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshot); err == nil {
		t.Fatal("verification accepted a snapshot whose manifest omitted an on-disk directory")
	}
}
