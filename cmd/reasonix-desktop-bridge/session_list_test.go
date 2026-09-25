package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/sessionidentity"
)

func TestSessionListEmptyDoesNotCreateIdentityStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	handler := newBridgeServer(testToken, "instance", nil).handler()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty list status=%d body=%s", response.Code, response.Body.String())
	}
	var body sessionListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || body.Sessions == nil || len(body.Sessions) != 0 {
		t.Fatalf("empty session list = %#v", body)
	}
	if _, err := os.Stat(appconfig.DesktopSessionIdentityPath()); !os.IsNotExist(err) {
		t.Fatalf("read-only listing created identity database: %v", err)
	}
}

func TestSessionListEndpointPagesAndProjectsSafeFields(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]sessionidentity.Candidate, 5)
	for i, id := range []string{"page-a", "page-b", "page-c", "page-d", "page-e"} {
		path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
		if err := os.WriteFile(path, []byte("secret transcript body"), 0o600); err != nil {
			t.Fatal(err)
		}
		candidates[i] = sessionidentity.Candidate{ID: id, Path: path, WorkspaceRoot: "/work/project", Title: "Title " + id, Position: 4}
	}
	if err := identities.Import(context.Background(), sessionDir, candidates); err != nil {
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	for id, state := range map[string]string{"page-b": "deleted", "page-d": "deleting", "page-c": "missing"} {
		if _, err := db.ExecContext(context.Background(), "UPDATE sessions SET state=? WHERE id=?", state, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	handler := newBridgeServer(testToken, "instance", nil).handler()
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/sessions", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	snapshotRequest := httptest.NewRequest(http.MethodGet, "/v1/sessions/snapshot?workspaceRoot=%2Fwork%2Fproject", nil)
	snapshotRequest.Header.Set("Authorization", "Bearer "+testToken)
	snapshotResponse := httptest.NewRecorder()
	handler.ServeHTTP(snapshotResponse, snapshotRequest)
	if snapshotResponse.Code != http.StatusOK {
		t.Fatalf("session snapshot status=%d body=%s", snapshotResponse.Code, snapshotResponse.Body.String())
	}
	var snapshot sessionListResponse
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode session snapshot: %v", err)
	}
	if snapshot.ProtocolVersion != 1 || snapshot.Total != 3 || len(snapshot.Sessions) != 3 || snapshot.NextCursor != nil || snapshot.SnapshotID == "" {
		t.Fatalf("complete session snapshot = %#v", snapshot)
	}
	if strings.Contains(snapshotResponse.Body.String(), candidates[0].Path) || strings.Contains(snapshotResponse.Body.String(), "secret transcript body") {
		t.Fatalf("session snapshot exposed a path or transcript: %s", snapshotResponse.Body.String())
	}

	fetch := func(rawQuery string) (sessionListResponse, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/v1/sessions?"+rawQuery, nil)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("session list status=%d body=%s", response.Code, response.Body.String())
		}
		var body sessionListResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode session list: %v", err)
		}
		return body, response.Body.String()
	}

	first, raw := fetch("limit=2&workspaceRoot=%2Fwork%2Fproject")
	if first.ProtocolVersion != 1 || first.Total != 3 || len(first.Sessions) != 2 || first.NextCursor == nil {
		t.Fatalf("first session page = %#v", first)
	}
	if first.Sessions[0].ID != "page-a" || first.Sessions[1].ID != "page-c" || !first.Sessions[1].Missing {
		t.Fatalf("first page order/state = %#v", first.Sessions)
	}
	if first.SnapshotID == "" || first.NextCursor.SnapshotID != first.SnapshotID {
		t.Fatalf("first page cursor is not bound to its snapshot: %#v", first)
	}
	if strings.Contains(raw, candidates[0].Path) || strings.Contains(raw, "secret transcript body") {
		t.Fatalf("session list exposed a path or transcript: %s", raw)
	}
	values := url.Values{
		"limit":          {"2"},
		"cursorPosition": {"4"},
		"cursorId":       {first.NextCursor.ID},
		"cursorSnapshot": {first.NextCursor.SnapshotID},
		"workspaceRoot":  {"/work/project"},
	}
	second, _ := fetch(values.Encode())
	if second.Total != 3 || len(second.Sessions) != 1 || second.Sessions[0].ID != "page-e" || second.NextCursor != nil {
		t.Fatalf("second session page = %#v", second)
	}
	if second.SnapshotID != first.SnapshotID {
		t.Fatalf("page snapshot changed without a catalog update: %q then %q", first.SnapshotID, second.SnapshotID)
	}

	changingFirst, _ := fetch("limit=1&workspaceRoot=%2Fwork%2Fproject")
	if changingFirst.NextCursor == nil {
		t.Fatal("expected a continuation cursor for mutation check")
	}
	mutator, err := sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ExecContext(context.Background(), `UPDATE sessions SET title=?, title_source=?,
		title_revision=title_revision+1, updated_at_ms=updated_at_ms+1 WHERE id=?`, "enriched listing title", "user", "page-e"); err != nil {
		_ = mutator.Close()
		t.Fatal(err)
	}
	if err := mutator.Close(); err != nil {
		t.Fatal(err)
	}
	continuationValues := url.Values{
		"limit":          {"1"},
		"cursorPosition": {strconv.Itoa(changingFirst.NextCursor.Position)},
		"cursorId":       {changingFirst.NextCursor.ID},
		"cursorSnapshot": {changingFirst.NextCursor.SnapshotID},
		"workspaceRoot":  {"/work/project"},
	}
	continued, _ := fetch(continuationValues.Encode())
	if continued.SnapshotID != changingFirst.SnapshotID || len(continued.Sessions) != 1 || continued.Sessions[0].ID != "page-c" {
		t.Fatalf("continuation after title-only update = %#v, want stable snapshot and next visible row", continued)
	}

	changingFirst, _ = fetch("limit=1&workspaceRoot=%2Fwork%2Fproject")
	mutator, err = sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ExecContext(context.Background(), "UPDATE sessions SET position=position+10 WHERE id=?", "page-e"); err != nil {
		_ = mutator.Close()
		t.Fatal(err)
	}
	if err := mutator.Close(); err != nil {
		t.Fatal(err)
	}
	staleValues := url.Values{
		"limit":          {"1"},
		"cursorPosition": {strconv.Itoa(changingFirst.NextCursor.Position)},
		"cursorId":       {changingFirst.NextCursor.ID},
		"cursorSnapshot": {changingFirst.NextCursor.SnapshotID},
		"workspaceRoot":  {"/work/project"},
	}
	staleRequest := httptest.NewRequest(http.MethodGet, "/v1/sessions?"+staleValues.Encode(), nil)
	staleRequest.Header.Set("Authorization", "Bearer "+testToken)
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict || !strings.Contains(staleResponse.Body.String(), `"code":"resync_required"`) {
		t.Fatalf("stale continuation = HTTP %d %s, want 409 resync_required", staleResponse.Code, staleResponse.Body.String())
	}

	badCursor := httptest.NewRequest(http.MethodGet, "/v1/sessions?cursorPosition=4", nil)
	badCursor.Header.Set("Authorization", "Bearer "+testToken)
	badResponse := httptest.NewRecorder()
	handler.ServeHTTP(badResponse, badCursor)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("partial cursor status = %d body=%s", badResponse.Code, badResponse.Body.String())
	}
}
