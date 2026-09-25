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

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/sessionidentity"
)

// The inventory endpoint is the host's read-only view of what the profile
// holds. It must answer before any identity database exists, and it must not
// create one.
func TestSessionInventoryIsReadOnlyAndAuthenticated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)

	sessionDir := appconfig.SessionDir()
	if sessionDir == "" {
		t.Fatal("session directory did not resolve for the test home")
	}
	transcript := filepath.Join(sessionDir, "tauri-tauri-orphan.jsonl")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(home, "workbench-sessions.json")
	catalog := []map[string]string{{"sessionId": "tauri-missing", "title": "Missing"}}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := newBridgeServer(testToken, "instance", nil)
	handler := bridge.handler()

	unauthorized := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated inventory status = %d", unauthorizedResponse.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory?catalog="+catalogPath, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory status = %d, body = %s", response.Code, response.Body.String())
	}

	var body sessionInventoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	if body.ProtocolVersion != 1 || body.SessionDir != sessionDir {
		t.Fatalf("inventory header = %#v", body)
	}
	if body.IdentityStore {
		t.Fatal("inventory reported an identity store that does not exist")
	}
	// The scanned transcript is a candidate, and the catalog entry without a
	// file is reported as missing rather than claimable.
	unclaimed := false
	missing := false
	for _, entry := range body.Entries {
		switch entry.ID {
		case "tauri-orphan":
			unclaimed = entry.Claim == "unclaimed_file" && entry.Exists
		case "tauri-missing":
			missing = entry.Claim == "missing_file" && !entry.Exists
		}
	}
	if !unclaimed || !missing {
		t.Fatalf("inventory entries = %#v (unclaimed=%v missing=%v)", body.Entries, unclaimed, missing)
	}
	if len(body.Unclaimed) != 1 || body.Unclaimed[0] != transcript {
		t.Fatalf("unclaimed = %#v", body.Unclaimed)
	}

	// Reading the listing must not bring an identity database into existence.
	identityPath := appconfig.DesktopSessionIdentityPath()
	if identityPath == "" {
		t.Fatal("identity path did not resolve for the test home")
	}
	if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
		t.Fatalf("inventory created the identity database: %v", err)
	}

	// Once a profile does have an identity store, the listing reads it through
	// the read-only opener and reports the registered session.
	registered, err := sessionidentity.Open(context.Background(), identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := registered.Import(context.Background(), sessionDir, []sessionidentity.Candidate{
		{ID: "tauri-present", Path: filepath.Join(sessionDir, "tauri-tauri-present.jsonl"), Title: "Present"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registered.Close(); err != nil {
		t.Fatal(err)
	}

	withStore := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
	withStore.Header.Set("Authorization", "Bearer "+testToken)
	withStoreResponse := httptest.NewRecorder()
	handler.ServeHTTP(withStoreResponse, withStore)
	if withStoreResponse.Code != http.StatusOK {
		t.Fatalf("inventory with a store status = %d, body = %s", withStoreResponse.Code, withStoreResponse.Body.String())
	}
	var withStoreBody sessionInventoryResponse
	if err := json.Unmarshal(withStoreResponse.Body.Bytes(), &withStoreBody); err != nil {
		t.Fatal(err)
	}
	if !withStoreBody.IdentityStore {
		t.Fatalf("inventory did not report the existing identity store")
	}
	for _, entry := range withStoreBody.Entries {
		if entry.ID == "tauri-present" && (!entry.Registered || entry.Claim != "registered") {
			t.Fatalf("registered entry = %#v", entry)
		}
	}
}

func TestSessionInventoryReportsSidecarTitleDriftWithoutChangingSources(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-title-drift.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: "title-drift", Path: path, Title: "Original title"}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := agent.RenameSession(path, "Original title"); err != nil {
		t.Fatal(err)
	}
	readInventory := func() sessionInventoryResponse {
		request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("inventory status=%d body=%s", response.Code, response.Body.String())
		}
		var body sessionInventoryResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	if clean := readInventory(); len(clean.Errors) != 0 {
		t.Fatalf("matching title was reported dirty: %#v", clean.Errors)
	}
	if err := agent.RenameSession(path, "Uncommitted sidecar title"); err != nil {
		t.Fatal(err)
	}
	dirty := readInventory()
	if len(dirty.Errors) == 0 || !strings.Contains(strings.Join(dirty.Errors, " "), "title metadata differs") {
		t.Fatalf("sidecar-only title change was not reported: %#v", dirty)
	}
	if strings.Contains(strings.Join(dirty.Errors, " "), "Uncommitted sidecar title") {
		t.Fatalf("inventory error leaked sidecar title: %#v", dirty.Errors)
	}
	for _, entry := range dirty.Entries {
		if entry.Source == sessionidentity.InventoryFromIdentity && entry.ID == "title-drift" && entry.Detail != "" {
			t.Fatalf("title drift mislabeled the readable transcript: %#v", entry)
		}
	}
	identities, err = sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, "title-drift")
	if err != nil || !exists || record.Title != "Original title" {
		t.Fatalf("read-only inventory changed identity: %#v exists=%v err=%v", record, exists, err)
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok || meta.CustomTitle != "Uncommitted sidecar title" {
		t.Fatalf("read-only inventory changed sidecar: %#v ok=%v err=%v", meta, ok, err)
	}
	outsideMeta := filepath.Join(t.TempDir(), "outside.meta")
	if err := os.WriteFile(outsideMeta, []byte(`{"custom_title":"outside private title"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agent.BranchMetaPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideMeta, agent.BranchMetaPath(path)); err != nil {
		t.Skipf("sidecar symlinks unavailable: %v", err)
	}
	linked := readInventory()
	if len(linked.Errors) == 0 || !strings.Contains(strings.Join(linked.Errors, " "), "title metadata is unreadable") ||
		strings.Contains(strings.Join(linked.Errors, " "), "outside private title") {
		t.Fatalf("symlinked title sidecar inventory = %#v", linked.Errors)
	}
}

func TestSessionInventoryRequiresMirrorForUserTitleButNotLegacyTitle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manualPath := filepath.Join(sessionDir, "tauri-manual.jsonl")
	legacyPath := filepath.Join(sessionDir, "tauri-legacy.jsonl")
	for _, path := range []string{manualPath, legacyPath} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{
		{ID: "manual", Path: manualPath},
		{ID: "legacy", Path: legacyPath, Title: "Legacy catalog title", Position: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := identities.SetTitle(ctx, "manual", 0, "Chosen title", sessionidentity.TitleManualRename); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	if err := agent.RenameSession(manualPath, "Chosen title"); err != nil {
		t.Fatal(err)
	}
	readErrors := func() []string {
		request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("inventory status=%d body=%s", response.Code, response.Body.String())
		}
		var body sessionInventoryResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Errors
	}
	if errors := readErrors(); len(errors) != 0 {
		t.Fatalf("matching user title or absent legacy sidecar was reported dirty: %#v", errors)
	}
	if err := os.Remove(agent.BranchMetaPath(manualPath)); err != nil {
		t.Fatal(err)
	}
	if errors := readErrors(); len(errors) != 1 || !strings.Contains(errors[0], "manual") {
		t.Fatalf("missing user-title mirror was not reported: %#v", errors)
	}
	if err := agent.RenameSession(manualPath, ""); err != nil {
		t.Fatal(err)
	}
	if errors := readErrors(); len(errors) != 1 || !strings.Contains(errors[0], "manual") {
		t.Fatalf("empty user-title mirror was not reported: %#v", errors)
	}
}

func TestSessionInventoryFlagsPendingTitleIntentBeforeSidecarChanges(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tauri-pending-title.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: "pending-title", Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := identities.BeginManualTitleRename(ctx, "pending-title", path, "", "Private new title", 0); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory status=%d body=%s", response.Code, response.Body.String())
	}
	var body sessionInventoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Errors) != 1 || !strings.Contains(body.Errors[0], "pending-title: session title rename recovery is pending") ||
		strings.Contains(response.Body.String(), "Private new title") {
		t.Fatalf("pending intent inventory = %#v", body.Errors)
	}
}

// The identity path is the store's own location and must not be settable by the
// caller, or a request could point the store outside the profile.
func TestSessionInventoryIgnoresACallerSuppliedIdentityPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)

	elsewhere := filepath.Join(t.TempDir(), "state.sqlite")
	bridge := newBridgeServer(testToken, "instance", nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory?identity="+elsewhere, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory status = %d", response.Code)
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("a caller-supplied identity path was used: %v", err)
	}
}

func TestSessionInventoryExposesLegacyPhysicalPathConflicts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	sessionDir := appconfig.SessionDir()
	transcript := filepath.Join(sessionDir, "tauri-shared.jsonl")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(home, "sessions-alias")
	if err := os.Symlink(sessionDir, aliasDir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	identities, err := sessionidentity.Open(context.Background(), identityPath, appconfig.SessionProfileRoot())
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
	for position, relative := range []string{
		filepath.ToSlash(filepath.Join("sessions", filepath.Base(transcript))),
		filepath.ToSlash(filepath.Join("sessions-alias", filepath.Base(transcript))),
	} {
		id := []string{"legacy-one", "legacy-two"}[position]
		if _, err := db.Exec(`INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, id, relative, position); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/inventory", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "instance", nil).handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory status = %d, body = %s", response.Code, response.Body.String())
	}
	var body sessionInventoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy-one", "legacy-two"} {
		found := false
		for _, entry := range body.Entries {
			if entry.ID == id {
				found = entry.Claim == sessionidentity.ClaimPathConflict && strings.Contains(entry.Detail, "physical transcript")
			}
		}
		if !found {
			t.Fatalf("inventory omitted physical conflict for %s: %#v", id, body)
		}
	}
	if len(body.Errors) == 0 {
		t.Fatalf("inventory response omitted conflict errors: %#v", body)
	}
}
