package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/store"
)

const forcedDeleteMarkerEnv = "REASONIX_TEST_FORCED_DELETE_MARKER"

func TestBridgeRetriesInterruptedDeleteAfterRestart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	factory := desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		runtime, err := newControllerFactory(nil).Open(ctx, request)
		if err != nil {
			return nil, err
		}
		runtime.(*controllerRuntime).removeArtifacts = func(string) error { return errors.New("interrupted sweep") }
		return runtime, nil
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "retry-after-restart", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(view.Path, []byte("transcript left by interrupted deletion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginManualTitleRename(ctx, view.ID, view.Path, "", "Interrupted rename", 0); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteSession(view.ID); err == nil {
		t.Fatal("injected cleanup failure was ignored")
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if _, pending, err := identities.PendingManualTitleRename(ctx, view.ID); err != nil || pending {
		t.Fatalf("deleting fence left title intent: pending=%v err=%v", pending, err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}

	restarted := newBridgeServer(testToken, "restart", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+view.ID, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "retry-after-restart-delete")
	response := httptest.NewRecorder()
	restarted.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("restarted delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Lstat(view.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted transcript survived recovery: %v", err)
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, view.ID)
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("recovered identity = %#v, %v, %v", record, exists, err)
	}
}

func TestBridgeDeleteRecoversAfterForcedProcessExit(t *testing.T) {
	marker := os.Getenv(forcedDeleteMarkerEnv)
	if marker != "" {
		ctx := context.Background()
		transcriptPath := ""
		factory := desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
			runtime, err := newControllerFactory(nil).Open(ctx, request)
			if err != nil {
				return nil, err
			}
			runtime.(*controllerRuntime).removeArtifacts = func(string) error {
				// Simulate a partially completed filesystem sweep: the transcript is
				// gone, but its metadata sidecar remains when the process is killed.
				if err := os.Remove(transcriptPath); err != nil {
					return err
				}
				if err := os.WriteFile(marker, []byte("artifact sweep reached after deleting fence\n"), 0o600); err != nil {
					return err
				}
				select {} // The parent kills this process while the committed deleting state is durable.
			}
			return runtime, nil
		})
		manager := desktopbridge.NewRuntimeManager(factory)
		view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "forced-delete-recovery", WorkspaceRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(view.Path, []byte("transcript must not become an empty session\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		transcriptPath = view.Path
		if err := os.WriteFile(store.SessionMeta(view.Path), []byte("metadata sidecar\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		go func() { _ = manager.DeleteSession(view.ID) }()
		select {} // The parent observes the marker and kills this process in the sweep.
	}

	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	marker = filepath.Join(root, "artifact-sweep-entered")
	cmd := exec.Command(os.Args[0], "-test.run=^TestBridgeDeleteRecoversAfterForcedProcessExit$")
	cmd.Env = append(os.Environ(),
		"REASONIX_HOME="+root,
		"REASONIX_STATE_HOME="+root,
		forcedDeleteMarkerEnv+"="+marker,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start isolated delete writer: %v", err)
	}
	waitChild := make(chan error, 1)
	go func() { waitChild <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Until(deadline) <= 0 {
			_ = cmd.Process.Kill()
			<-waitChild
			t.Fatal("delete child did not reach the artifact sweep after committing deleting state")
		}
		select {
		case err := <-waitChild:
			t.Fatalf("delete child exited before the artifact sweep: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := cmd.Process.Kill(); err != nil {
		<-waitChild
		t.Fatalf("forcibly stop delete child: %v", err)
	}
	if err := <-waitChild; err == nil {
		t.Fatal("delete child unexpectedly exited cleanly")
	}

	ctx := context.Background()
	identityPath := appconfig.DesktopSessionIdentityPath()
	identities, err := sessionidentity.OpenReadOnly(ctx, identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatalf("open identity after forced exit: %v", err)
	}
	record, exists, err := identities.Get(ctx, "forced-delete-recovery")
	if closeErr := identities.Close(); err == nil {
		err = closeErr
	}
	if err != nil || !exists || record.State != sessionidentity.StateDeleting {
		t.Fatalf("identity after forced exit = %#v, exists=%v, err=%v; want deleting", record, exists, err)
	}
	transcriptPath := filepath.Join(appconfig.SessionDir(), "tauri-forced-delete-recovery.jsonl")
	if _, err := os.Stat(transcriptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("simulated partial sweep did not remove transcript: %v", err)
	}
	if _, err := os.Stat(store.SessionMeta(transcriptPath)); err != nil {
		t.Fatalf("simulated partial sweep should leave metadata for retry: %v", err)
	}

	restartedManager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	if _, err := restartedManager.Open(ctx, desktopbridge.OpenRequest{SessionID: "forced-delete-recovery", WorkspaceRoot: root}); err == nil {
		t.Fatal("partially deleted session reopened as an empty conversation")
	}
	restarted := newBridgeServer(testToken, "forced-exit-restart", restartedManager)
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/forced-delete-recovery", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "forced-exit-retry-delete")
	response := httptest.NewRecorder()
	restarted.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("restarted delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(transcriptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery left the transcript behind: %v", err)
	}
	if _, err := os.Stat(store.SessionMeta(transcriptPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery left the metadata sidecar behind: %v", err)
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatalf("reopen identity after delete retry: %v", err)
	}
	defer identities.Close()
	record, exists, err = identities.Get(ctx, "forced-delete-recovery")
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("identity after delete retry = %#v, exists=%v, err=%v; want deleted tombstone", record, exists, err)
	}
}

func TestInterruptedDeleteRetryRefusesLegacyPhysicalAlias(t *testing.T) {
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
	sessionID := "interrupted-duplicate"
	path, err := bridgeSessionPath(sessionDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	transcript := []byte("shared interrupted transcript\n")
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
	for position, identity := range []struct {
		id, path, state string
	}{
		{id: sessionID, path: path, state: "deleting"},
		{id: "legacy-owner", path: filepath.Join(aliasDir, filepath.Base(path)), state: "ready"},
	} {
		relative, err := filepath.Rel(appconfig.SessionProfileRoot(), identity.path)
		if err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, ?, 0, 0)`, identity.id, filepath.ToSlash(relative), position, identity.state); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "duplicate-retry", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+sessionID, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "duplicate-retry-delete")
	response := httptest.NewRecorder()
	server.handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("aliased interrupted delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(transcript) {
		t.Fatalf("interrupted delete retry swept aliased transcript: %q, %v", after, err)
	}
}

func TestBridgeRecoveryCannotDeleteUnownedNormalSession(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "normal-unowned", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(view.Path, []byte("must survive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := newBridgeServer(testToken, "restart", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+view.ID, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "normal-unowned-delete")
	response := httptest.NewRecorder()
	restarted.handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unowned delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(view.Path); err != nil {
		t.Fatalf("unowned transcript was removed: %v", err)
	}
}

func TestBridgeDeleteCanRetireMissingSessionWithoutRecreatingIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := bridgeSessionPath(sessionDir, "retire-missing-session")
	if err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.ImportLegacyCatalog(ctx, sessionDir, []sessionidentity.Candidate{{
		ID: "retire-missing-session", Path: path,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture transcript unexpectedly exists: %v", err)
	}

	server := newBridgeServer(testToken, "retire-missing", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/retire-missing-session", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "retire-missing-delete")
	response := httptest.NewRecorder()
	server.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("missing-session delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-session delete created transcript: %v", err)
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "retire-missing-session")
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("retired identity = %#v, %v, %v", record, exists, err)
	}
}

func TestBridgeDeleteRetryAcceptsDeletedTombstoneWithoutTouchingArtifacts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := bridgeSessionPath(sessionDir, "already-deleted-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old transcript\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: "already-deleted-session", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginDelete(ctx, "already-deleted-session", path); err != nil {
		t.Fatal(err)
	}
	if err := identities.FinishDelete(ctx, "already-deleted-session", path); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	server := newBridgeServer(testToken, "deleted-retry", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	for _, requestID := range []string{"already-deleted-first-retry", "already-deleted-second-retry"} {
		request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/already-deleted-session", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set(requestIDHeader, requestID)
		response := httptest.NewRecorder()
		server.handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("deleted-tombstone retry status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted-tombstone retry recreated or retained transcript: %v", err)
	}
}
