package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

func TestPendingSessionTitleRecoveriesExposeOldCatalogIndependentEntry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "older-than-recent-catalog"
	path, err := sessionpath.TranscriptPath(sessionDir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "project")
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{
		ID: id, Path: path, Title: "Before rename", WorkspaceRoot: workspaceRoot,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginManualTitleRename(ctx, id, path, "", "After rename", 0); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", desktopbridge.NewRuntimeManager(newControllerFactory(nil))).handler()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/title-recovery", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	read := func() pendingSessionTitleRecoveriesResponse {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("title recovery status=%d body=%s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), path) || strings.Contains(response.Body.String(), `"path"`) ||
			strings.Contains(response.Body.String(), "After rename") {
			t.Fatalf("title recovery response leaked transcript or uncommitted title: %s", response.Body.String())
		}
		var body pendingSessionTitleRecoveriesResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	body := read()
	if body.ProtocolVersion != desktopbridge.ProtocolVersion || len(body.Sessions) != 1 ||
		body.Sessions[0].ID != id || body.Sessions[0].Title != "Before rename" ||
		body.Sessions[0].WorkspaceRoot != workspaceRoot || body.Sessions[0].State != sessionidentity.StateReady {
		t.Fatalf("pending title recovery = %#v", body)
	}
	if err := agent.RenameSession(path, "After rename"); err != nil {
		t.Fatal(err)
	}
	identities, err = sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := recoverBridgeManualTitleRename(ctx, identities, id, path); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if after := read(); len(after.Sessions) != 0 {
		t.Fatalf("completed title recovery remains pending: %#v", after.Sessions)
	}
}

func TestOfflineSnapshotRestoresBridgeManualTitleRenameAtBothCrashPoints(t *testing.T) {
	for _, tc := range []struct {
		name           string
		sidecarWritten bool
		wantTitle      string
		wantRevision   int64
	}{
		{name: "before sidecar write", wantTitle: "Before rename"},
		{name: "after sidecar write", sidecarWritten: true, wantTitle: "After rename", wantRevision: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			profile := filepath.Join(root, "preview-profile")
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			sessionDir := appconfig.SessionDir()
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				t.Fatal(err)
			}
			const id = "tauri-restore-title"
			path, err := sessionpath.TranscriptPath(sessionDir, id)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			identityPath := appconfig.DesktopSessionIdentityPath()
			writer, err := sessionidentity.Open(ctx, identityPath, profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: id, Path: path, Title: "Before rename"}}); err != nil {
				t.Fatal(err)
			}
			if err := writer.BeginManualTitleRename(ctx, id, path, "", "After rename", 0); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if tc.sidecarWritten {
				if err := agent.RenameSession(path, "After rename"); err != nil {
					t.Fatal(err)
				}
			}
			originalSidecar, sidecarErr := os.ReadFile(path + ".meta")
			if sidecarErr != nil && !os.IsNotExist(sidecarErr) {
				t.Fatal(sidecarErr)
			}

			catalog := filepath.Join(root, "workbench-sessions.json")
			if err := os.WriteFile(catalog, []byte(`[{"sessionId":"tauri-restore-title","title":"Before rename"}]`), 0o600); err != nil {
				t.Fatal(err)
			}
			backupParent := filepath.Join(root, "backups")
			recoveryParent := filepath.Join(root, "recovery")
			for _, parent := range []string{backupParent, recoveryParent} {
				if err := os.Mkdir(parent, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := sessionidentity.CreateOfflineSnapshot(ctx, profile, catalog, backupParent)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := sessionidentity.StageOfflineSnapshot(ctx, snapshot, recoveryParent)
			if err != nil {
				t.Fatal(err)
			}
			restoredProfile := filepath.Join(staged, "profile")
			t.Setenv("REASONIX_HOME", restoredProfile)
			t.Setenv("REASONIX_STATE_HOME", restoredProfile)
			restoredPath, err := sessionpath.TranscriptPath(appconfig.SessionDir(), id)
			if err != nil {
				t.Fatal(err)
			}
			handler := newBridgeServer(testToken, "instance", desktopbridge.NewRuntimeManager(newControllerFactory(nil))).handler()
			readRecoveries := func() pendingSessionTitleRecoveriesResponse {
				request := httptest.NewRequest(http.MethodGet, "/v1/sessions/title-recovery", nil)
				request.Header.Set("Authorization", "Bearer "+testToken)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("title recovery status=%d body=%s", response.Code, response.Body.String())
				}
				if strings.Contains(response.Body.String(), restoredPath) || strings.Contains(response.Body.String(), `"path"`) ||
					strings.Contains(response.Body.String(), "After rename") {
					t.Fatalf("title recovery response leaked transcript or uncommitted title: %s", response.Body.String())
				}
				var body pendingSessionTitleRecoveriesResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			if before := readRecoveries(); len(before.Sessions) != 1 || before.Sessions[0].ID != id ||
				before.Sessions[0].Title != "Before rename" {
				t.Fatalf("staged pending recoveries = %#v", before)
			}
			restored, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), restoredProfile)
			if err != nil {
				t.Fatal(err)
			}
			if err := recoverBridgeManualTitleRename(ctx, restored, id, restoredPath); err != nil {
				t.Fatal(err)
			}
			record, exists, err := restored.Get(ctx, id)
			if err != nil || !exists || record.Title != tc.wantTitle || record.TitleRevision != tc.wantRevision {
				t.Fatalf("restored title = %#v exists=%v err=%v", record, exists, err)
			}
			if _, pending, err := restored.PendingManualTitleRename(ctx, id); err != nil || pending {
				t.Fatalf("restored title intent pending=%v err=%v", pending, err)
			}
			if err := restored.Close(); err != nil {
				t.Fatal(err)
			}
			if after := readRecoveries(); len(after.Sessions) != 0 {
				t.Fatalf("recovered title intent remains listed: %#v", after)
			}

			source, err := sessionidentity.OpenReadOnly(ctx, identityPath, profile)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			if _, pending, err := source.PendingManualTitleRename(ctx, id); err != nil || !pending {
				t.Fatalf("source title intent pending=%v err=%v", pending, err)
			}
			if sourceRecord, exists, err := source.Get(ctx, id); err != nil || !exists ||
				sourceRecord.Title != "Before rename" || sourceRecord.TitleRevision != 0 {
				t.Fatalf("source title = %#v exists=%v err=%v", sourceRecord, exists, err)
			}
			currentSidecar, currentErr := os.ReadFile(path + ".meta")
			if (sidecarErr == nil) != (currentErr == nil) || string(currentSidecar) != string(originalSidecar) {
				t.Fatalf("source title sidecar changed: before=%q (%v), after=%q (%v)", originalSidecar, sidecarErr, currentSidecar, currentErr)
			}
		})
	}
}
