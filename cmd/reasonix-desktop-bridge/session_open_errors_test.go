package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

func TestBridgeOpenAndSwitchExposeSessionLifecycleConflictCodes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	missingPath, err := bridgeSessionPath(sessionDir, "known-missing")
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.ImportLegacyCatalog(ctx, sessionDir, []sessionidentity.Candidate{{ID: "known-missing", Path: missingPath}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"known-deleting", "known-deleted"} {
		path, err := bridgeSessionPath(sessionDir, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := identities.Reserve(ctx, sessionDir, sessionidentity.Candidate{ID: id, Path: path}); err != nil {
			t.Fatal(err)
		}
		if err := identities.BeginDelete(ctx, id, path); err != nil {
			t.Fatal(err)
		}
		if id == "known-deleted" {
			if err := identities.FinishDelete(ctx, id, path); err != nil {
				t.Fatal(err)
			}
		}
	}
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	server := newBridgeServer(testToken, "lifecycle-codes", manager)
	for _, route := range []string{"/v1/sessions:open", "/v1/sessions:switch"} {
		for _, test := range []struct{ id, code string }{
			{"known-missing", "session_missing"},
			{"known-deleting", "session_deleting"},
			{"known-deleted", "session_deleted"},
		} {
			request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"sessionId":"`+test.id+`"}`))
			request.Header.Set("Authorization", "Bearer "+testToken)
			response := httptest.NewRecorder()
			server.handler().ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("%s %s: HTTP %d, body %s", route, test.id, response.Code, response.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != test.code {
				t.Fatalf("%s %s: error code %q, decode error %v", route, test.id, body.Error.Code, err)
			}
		}
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("opening missing session created transcript: %v", err)
	}
}

func TestBridgeOpenRejectsLegacyPhysicalPathConflictAsHTTP409(t *testing.T) {
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
	transcriptPath, err := bridgeSessionPath(sessionDir, "open-target")
	if err != nil {
		t.Fatal(err)
	}
	transcript := []byte("{}\n")
	if err := os.WriteFile(transcriptPath, transcript, 0o600); err != nil {
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
		{id: "open-target", path: transcriptPath},
		{id: "legacy-owner", path: filepath.Join(aliasDir, filepath.Base(transcriptPath))},
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
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	server := newBridgeServer(testToken, "physical-path-conflict", manager)
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:open", strings.NewReader(`{"sessionId":"open-target"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	server.handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("open legacy physical conflict: HTTP %d, body %s", response.Code, response.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != "conflict" {
		t.Fatalf("physical conflict response code = %q, decode error %v, body %s", body.Error.Code, err, response.Body.String())
	}
	if after, err := os.ReadFile(transcriptPath); err != nil || string(after) != string(transcript) {
		t.Fatalf("open conflict changed transcript: %q, %v", after, err)
	}
}
