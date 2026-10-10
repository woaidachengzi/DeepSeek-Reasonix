package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
)

// Explicit package selection is mandatory. No in-process handler stands in
// for scan/apply: every request below reaches the exact release child.
func TestSQLiteActualPackageReviewedScan(t *testing.T) {
	binary := os.Getenv("REASONIX_SQLITE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	for _, mode := range []string{"selected_subset", "changed_transcript", "missing_metadata", "duplicate_selection", "catalog_claimed", "removed_transcript", "transaction_abort"} {
		t.Run(mode, func(t *testing.T) {
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			dir := appconfig.SessionDir()
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			ids := []string{"review-one", "review-two"}
			paths := make([]string, 2)
			contents := []byte("{\"role\":\"user\",\"content\":\"private reviewed scan fixture\"}\n")
			expected := make(map[string][]byte)
			for i, id := range ids {
				path, err := sessionpath.TranscriptPath(dir, id)
				if err != nil {
					t.Fatal(err)
				}
				paths[i] = path
				if err := os.WriteFile(path, contents, 0600); err != nil {
					t.Fatal(err)
				}
				expected[path] = contents
			}
			catalog := filepath.Join(profile, "workbench-sessions.json")
			catalogBytes := []byte("[]\n")
			if err := os.WriteFile(catalog, catalogBytes, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if mode == "transaction_abort" {
				db, err := sql.Open("sqlite", appconfig.DesktopSessionIdentityPath())
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(`CREATE TRIGGER owned_scan_fault BEFORE INSERT ON sessions
				WHEN NEW.id='review-two' AND EXISTS(SELECT 1 FROM sessions WHERE id='review-one') BEGIN
				SELECT RAISE(ABORT,'owned-scan-first-insert'); END`)
				closeErr := db.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			}
			p := startSQLitePackagedSidecar(t, binary, profile, "")
			list := func() sessionListResponse {
				t.Helper()
				status, reply := p.call(t, "GET", "/v1/sessions", nil, true)
				var result sessionListResponse
				if status != 200 || json.Unmarshal(reply, &result) != nil {
					t.Fatal("packaged list failed", status)
				}
				return result
			}
			before := list()
			if len(before.Sessions) != 0 {
				t.Fatal("unclaimed scan files were automatically imported")
			}
			scanRequest := scanImportCandidatesRequest{CatalogPath: catalog}
			for _, route := range []string{"/v1/sessions/scan-import-candidates", "/v1/sessions/import-scan"} {
				if status, _ := p.call(t, "POST", route, scanRequest, false); status != 401 {
					t.Fatal("reviewed scan bypassed auth", status)
				}
			}
			status, reply := p.call(t, "POST", "/v1/sessions/scan-import-candidates", scanRequest, true)
			var scan scanImportCandidatesResponse
			if status != 200 || json.Unmarshal(reply, &scan) != nil || scan.ProtocolVersion != 1 || len(scan.Candidates) != 2 || scan.BlockedCount != 0 || bytes.Contains(reply, []byte(profile)) {
				t.Fatal("packaged scan is not a path-free two-candidate review", status)
			}
			byID := make(map[string]sessionidentity.ScanImportCandidate)
			for _, candidate := range scan.Candidates {
				byID[candidate.ID] = candidate
			}
			title, workspace := "Reviewed title", ""
			selected := make([]sessionidentity.ScanImportSelection, 0, 2)
			for i, id := range ids {
				candidate := byID[id]
				digest := sha256.Sum256(contents)
				if candidate.File != filepath.Base(paths[i]) || candidate.TranscriptSHA256 != hex.EncodeToString(digest[:]) {
					t.Fatal("actual scan fingerprint differs from reviewed private bytes")
				}
				selected = append(selected, sessionidentity.ScanImportSelection{ID: id, Title: &title, WorkspaceRoot: &workspace, TranscriptSHA256: candidate.TranscriptSHA256})
			}
			if after := list(); len(after.Sessions) != 0 || after.SnapshotID != before.SnapshotID {
				t.Fatal("read-only scan changed identity directory")
			}
			switch mode {
			case "selected_subset":
				selected = selected[:1]
			case "changed_transcript":
				expected[paths[1]] = []byte("changed private fixture after review\n")
				if err := os.WriteFile(paths[1], expected[paths[1]], 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_metadata":
				selected[1].Title = nil
			case "duplicate_selection":
				selected[1] = selected[0]
			case "catalog_claimed":
				catalogBytes = []byte("[{\"sessionId\":\"review-two\",\"title\":\"Catalog owner\"}]\n")
				if err := os.WriteFile(catalog, catalogBytes, 0600); err != nil {
					t.Fatal(err)
				}
			case "removed_transcript":
				if err := os.Remove(paths[1]); err != nil {
					t.Fatal(err)
				}
				delete(expected, paths[1])
			}
			status, reply = p.call(t, "POST", "/v1/sessions/import-scan", scanImportApplyRequest{CatalogPath: catalog, Selected: selected}, true)
			want := 0
			if mode == "selected_subset" {
				var applied scanImportApplyResponse
				if status != 200 || json.Unmarshal(reply, &applied) != nil || applied.ProtocolVersion != 1 || applied.Applied != 1 || len(applied.SessionIDs) != 1 || applied.SessionIDs[0] != ids[0] {
					t.Fatal("exact reviewed selection was not applied", status)
				}
				want = 1
			} else if mode == "transaction_abort" {
				// The trigger fires only after the first row exists inside the
				// transaction. Internal SQLite details must stay out of HTTP errors.
				if status != 500 || !bytes.Contains(reply, []byte("unable to apply the reviewed session import")) || bytes.Contains(reply, []byte("owned-scan-first-insert")) || bytes.Contains(reply, []byte(profile)) {
					t.Fatal("did not fail after the first transactional scan insert", status)
				}
			} else if status != 409 {
				t.Fatal("invalidated review was accepted", status)
			}
			after := list()
			if len(after.Sessions) != want || (want == 0 && after.SnapshotID != before.SnapshotID) {
				t.Fatal("rejected review left a partial batch or revision")
			}
			if want == 1 && (after.Sessions[0].ID != ids[0] || after.Sessions[0].Title != title) {
				t.Fatal("reviewed selection metadata differs")
			}
			p.stop(t)
			p = startSQLitePackagedSidecar(t, binary, profile, "")
			if len(list().Sessions) != want {
				t.Fatal("restart changed reviewed import outcome")
			}
			if mode == "selected_subset" {
				status, reply = p.call(t, "POST", "/v1/sessions/scan-import-candidates", scanRequest, true)
				if status != 200 || json.Unmarshal(reply, &scan) != nil || len(scan.Candidates) != 1 || scan.Candidates[0].ID != ids[1] {
					t.Fatal("unselected history was consumed or registered file stayed scan-only")
				}
			}
			p.stop(t)
			for path, wantBytes := range expected {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, wantBytes) {
					t.Fatal("reviewed import rewrote source JSONL", err)
				}
			}
			if mode == "removed_transcript" {
				if _, err := os.Lstat(paths[1]); !os.IsNotExist(err) {
					t.Fatal("import recreated removed transcript")
				}
			}
			got, err := os.ReadFile(catalog)
			if err != nil || !bytes.Equal(got, catalogBytes) {
				t.Fatal("reviewed scan modified its catalog", err)
			}
		})
	}
}
