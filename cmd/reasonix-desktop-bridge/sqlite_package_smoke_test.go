package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	encodingbinary "encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/provider"
	"reasonix/internal/sessionidentity"
)

func TestSQLiteActualPackageImportAtomicity(t *testing.T) {
	binary := os.Getenv("REASONIX_SQLITE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	for _, mode := range []string{"transaction_abort", "hardlink_alias", "symlink_alias", "repeat_import"} {
		t.Run(mode, func(t *testing.T) {
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			dir := appconfig.SessionDir()
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			ids := []string{"atomic-first", "atomic-second"}
			paths := make([]string, 2)
			original := []byte("{\"role\":\"user\",\"content\":\"private atomic fixture\"}\n")
			for i, id := range ids {
				path, err := sessionpath.TranscriptPath(dir, id)
				if err != nil {
					t.Fatal(err)
				}
				paths[i] = path
				if i == 1 && mode == "hardlink_alias" {
					if err := os.Link(paths[0], path); err != nil {
						t.Fatal(err)
					}
				} else if i == 1 && mode == "symlink_alias" {
					if err := os.Symlink(paths[0], path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			dbPath := appconfig.DesktopSessionIdentityPath()
			writer, err := sessionidentity.Open(context.Background(), dbPath, profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if mode == "transaction_abort" {
				db, err := sql.Open("sqlite", dbPath)
				if err != nil {
					t.Fatal(err)
				}
				// Raise only after proving first insert occurred in this same
				// transaction. The subsequent list must prove it was rolled back.
				_, err = db.Exec(`CREATE TRIGGER owned_import_fault BEFORE INSERT ON sessions
				WHEN NEW.id='atomic-second' BEGIN
				SELECT CASE WHEN EXISTS(SELECT 1 FROM sessions WHERE id='atomic-first')
				THEN RAISE(ABORT,'owned-confirmed-first-insert')
				ELSE RAISE(ABORT,'owned-missing-first-insert') END; END`)
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
				t.Fatal("fixture already has registered identities")
			}
			firstTitle, secondTitle := "First title", "Second title"
			request := importLegacyCatalogRequest{Sessions: []legacyCatalogEntry{{SessionID: ids[0], Title: &firstTitle}, {SessionID: ids[1], Title: &secondTitle}}}
			if status, _ := p.call(t, "POST", "/v1/sessions/import-catalog", request, false); status != 401 {
				t.Fatal("catalog import bypassed auth")
			}
			status, reply := p.call(t, "POST", "/v1/sessions/import-catalog", request, true)
			if mode == "repeat_import" {
				if status != 200 {
					t.Fatal("valid import rejected", status)
				}
				first := list()
				if len(first.Sessions) != 2 || first.Sessions[0].ID != ids[0] || first.Sessions[1].ID != ids[1] || first.Sessions[0].Title != firstTitle || first.Sessions[1].Title != secondTitle {
					t.Fatal("valid import lost identity or title")
				}
				// Stale legacy metadata cannot overwrite already imported rows.
				firstTitle, secondTitle = "Stale first", "Stale second"
				if status, _ := p.call(t, "POST", "/v1/sessions/import-catalog", request, true); status != 200 {
					t.Fatal("repeat import rejected", status)
				}
				after := list()
				firstJSON, _ := json.Marshal(first)
				afterJSON, _ := json.Marshal(after)
				if !bytes.Equal(firstJSON, afterJSON) {
					t.Fatal("repeat import changed identity metadata or directory revision")
				}
			} else {
				if status != 409 {
					t.Fatal("conflicting or interrupted batch accepted", status)
				}
				if mode == "transaction_abort" && !bytes.Contains(reply, []byte("owned-confirmed-first-insert")) {
					t.Fatal("failure did not occur after first transactional insert")
				}
				after := list()
				if len(after.Sessions) != 0 || after.SnapshotID != before.SnapshotID {
					t.Fatal("failed import left partial identities or revision")
				}
			}
			p.stop(t)
			// Reopen using the same actual package: rollback is durable, not
			// merely hidden by a cached response in the first process.
			p = startSQLitePackagedSidecar(t, binary, profile, "")
			want := 0
			if mode == "repeat_import" {
				want = 2
			}
			if len(list().Sessions) != want {
				t.Fatal("restart changed transaction outcome")
			}
			p.stop(t)
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, original) {
					t.Fatal("import rewrote JSONL", err)
				}
			}
		})
	}
}

func TestSQLiteActualPackageAbruptExitWAL(t *testing.T) {
	binary := os.Getenv("REASONIX_SQLITE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed_recovery", true: "corrupt_rejection"}[corrupt], func(t *testing.T) {
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			if err := os.WriteFile(appconfig.UserConfigPath(), []byte("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"openai\"\nbase_url=\"http://127.0.0.1:1/v1\"\nmodels=[\"alpha\"]\ndefault=\"alpha\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(appconfig.SessionDir(), 0700); err != nil {
				t.Fatal(err)
			}
			const id = "sqlite-wal-owned"
			path, err := sessionpath.TranscriptPath(appconfig.SessionDir(), id)
			if err != nil {
				t.Fatal(err)
			}
			session := agent.NewSession("Preserved title")
			session.Add(provider.Message{Role: provider.RoleUser, Content: "synthetic crash-preserved question"})
			if err := session.Save(path); err != nil {
				t.Fatal(err)
			}
			dbPath := appconfig.DesktopSessionIdentityPath()
			writer, err := sessionidentity.Open(context.Background(), dbPath, profile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			if err := writer.Import(context.Background(), appconfig.SessionDir(), []sessionidentity.Candidate{{ID: id, Path: path, Title: "Preserved title"}}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			p := startSQLitePackagedSidecar(t, binary, profile, "")
			if status, _ := p.call(t, "POST", "/v1/sessions:open", map[string]string{"sessionId": id}, true); status != 200 {
				t.Fatal("package failed to open owned WAL session", status)
			}
			if status, _ := p.call(t, "PATCH", "/v1/sessions/"+id+"/title", map[string]string{"title": "Committed package title"}, true); status != 200 {
				t.Fatal("package rename did not commit", status)
			}
			read := func(path string) []byte {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			jsonl := read(path)
			p.kill(t)
			wal := read(dbPath + "-wal")
			if len(wal) <= 32 {
				t.Fatal("actual package did not leave nonempty WAL after crash")
			}
			if corrupt {
				wal[len(wal)-1] ^= 0xff
				if err := os.WriteFile(dbPath+"-wal", wal, 0600); err != nil {
					t.Fatal(err)
				}
			}
			beforeDB := read(dbPath)
			// Prove the committed title is still only in WAL, rather than
			// accepting a nonempty but irrelevant WAL beside a checkpointed DB.
			mainCopy := filepath.Join(t.TempDir(), "main-only.sqlite")
			if err := os.WriteFile(mainCopy, beforeDB, 0600); err != nil {
				t.Fatal(err)
			}
			mainDB, err := sql.Open("sqlite", mainCopy)
			if err != nil {
				t.Fatal(err)
			}
			var mainTitle string
			err = mainDB.QueryRow("SELECT title FROM sessions WHERE id=?", id).Scan(&mainTitle)
			closeErr := mainDB.Close()
			if err != nil || closeErr != nil || mainTitle != "Preserved title" {
				t.Fatal("commit was not exclusively WAL-backed", err, closeErr)
			}
			refusal := ""
			if corrupt {
				refusal = "WAL"
			}
			restarted := startSQLitePackagedSidecar(t, binary, profile, refusal)
			if corrupt {
				if restarted != nil || !bytes.Equal(read(dbPath), beforeDB) || !bytes.Equal(read(dbPath+"-wal"), wal) {
					t.Fatal("corrupt WAL rejection altered evidence")
				}
			} else {
				status, reply := restarted.call(t, "GET", "/v1/sessions", nil, true)
				var doc sessionListResponse
				if status != 200 || json.Unmarshal(reply, &doc) != nil || len(doc.Sessions) != 1 || doc.Sessions[0].ID != id || doc.Sessions[0].Title != "Committed package title" || doc.Sessions[0].Missing {
					t.Fatal("WAL recovery lost committed package title", status)
				}
				restarted.stop(t)
			}
			if !bytes.Equal(read(path), jsonl) {
				t.Fatal("WAL recovery or rejection rewrote JSONL")
			}
		})
	}
}

func (p *sqlitePackagedSidecar) kill(t *testing.T) {
	t.Helper()
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal("owned crash child kill failed", err)
	}
	select {
	case err := <-p.done:
		p.stopped = true
		if err == nil {
			t.Fatal("crash child exited normally")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owned crash child survived")
	}
}

// Opt-in: exercise the exact packaged sidecar, not an in-process handler.
// All profiles are synthetic; no model, GUI, clipboard or real migration.
func TestSQLiteActualPackageCompatibilityAndOfflineRestore(t *testing.T) {
	binary := os.Getenv("REASONIX_SQLITE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	binaryBytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal("selected package binary unavailable", err)
	}
	binaryDigest := sha256.Sum256(binaryBytes)
	t.Logf("packaged sidecar SHA256 %x", binaryDigest)
	t.Cleanup(func() {
		after, err := os.ReadFile(binary)
		if err != nil || sha256.Sum256(after) != binaryDigest {
			t.Error("selected package binary changed during acceptance")
		}
	})
	for _, mode := range []string{"v8", "future", "corrupt", "corrupt_page", "orphan_wal", "offline_restore"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			sessionDir := appconfig.SessionDir()
			if err := os.MkdirAll(sessionDir, 0700); err != nil {
				t.Fatal(err)
			}
			const id = "sqlite-owned-history"
			path, err := sessionpath.TranscriptPath(sessionDir, id)
			if err != nil {
				t.Fatal(err)
			}
			original := []byte("{\"role\":\"user\",\"content\":\"synthetic preserved history\"}\n")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			identityPath := appconfig.DesktopSessionIdentityPath()
			writer, err := sessionidentity.Open(ctx, identityPath, profile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			if err := writer.Import(ctx, sessionDir, []sessionidentity.Candidate{{ID: id, Path: path, Title: "Preserved title"}}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "v8", "future":
				db, err := sql.Open("sqlite", identityPath)
				if err != nil {
					t.Fatal(err)
				}
				statement := "PRAGMA user_version=999"
				if mode == "v8" {
					statement = "ALTER TABLE session_event_streams DROP COLUMN import_verified; PRAGMA user_version=8"
				}
				_, err = db.Exec(statement)
				closeErr := db.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			case "corrupt":
				if err := os.WriteFile(identityPath, []byte("not a sqlite database\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt_page":
				db, err := sql.Open("sqlite", identityPath)
				if err != nil {
					t.Fatal(err)
				}
				var rootPage int
				err = db.QueryRow("SELECT rootpage FROM sqlite_master WHERE name='sessions'").Scan(&rootPage)
				closeErr := db.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
				data, err := os.ReadFile(identityPath)
				if err != nil || len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
					t.Fatal("invalid page-corruption fixture", err)
				}
				pageSize := int(encodingbinary.BigEndian.Uint16(data[16:18]))
				if pageSize == 1 {
					pageSize = 65536
				}
				offset := (rootPage - 1) * pageSize
				if rootPage <= 1 || offset < 100 || offset >= len(data) {
					t.Fatal("unsafe fixture page offset")
				}
				data[offset] = 0 // Invalid btree page type; keep schema/header valid.
				if err := os.WriteFile(identityPath, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "orphan_wal":
				if err := os.Remove(identityPath); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(identityPath+"-wal", []byte("orphan evidence\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			readFile := func(path string) []byte {
				t.Helper()
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			var dbBefore []byte
			if mode != "orphan_wal" {
				dbBefore = readFile(identityPath)
			}
			refusal := ""
			switch mode {
			case "future":
				refusal = "newer than supported"
			case "corrupt":
				refusal = "invalid SQLite header"
			case "corrupt_page":
				refusal = "quick check"
			case "orphan_wal":
				refusal = "sidecar"
			}
			p := startSQLitePackagedSidecar(t, binary, profile, refusal)
			if p != nil {
				if status, _ := p.call(t, "GET", "/v1/sessions", nil, false); status != 401 {
					t.Fatal("package bypassed authentication")
				}
				request := syncSessionCatalogRequest{Sessions: []sessionidentity.WorkbenchOrderEntry{{ID: id}}}
				status, _ := p.call(t, "POST", "/v1/sessions/sync-catalog", request, true)
				if status != 200 {
					t.Fatal("compatible database rejected", status)
				}
				p.assertIdentity(t, id)
				p.stop(t)
			}
			if !bytes.Equal(readFile(path), original) {
				t.Fatal("SQLite operation rewrote JSONL")
			}
			if mode == "future" || mode == "corrupt" || mode == "corrupt_page" {
				if !bytes.Equal(readFile(identityPath), dbBefore) {
					t.Fatal("rejection rewrote database")
				}
			}
			if mode == "orphan_wal" {
				if _, err := os.Lstat(identityPath); !os.IsNotExist(err) {
					t.Fatal("orphan rejection created main database")
				}
				if string(readFile(identityPath+"-wal")) != "orphan evidence\n" {
					t.Fatal("orphan evidence changed")
				}
			}
			if mode == "v8" {
				db, err := sql.Open("sqlite", identityPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var version int
				if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 9 {
					t.Fatal("package failed v8 migration", version, err)
				}
			}
			if mode != "offline_restore" {
				return
			}
			// All packaged writers have exited before taking the full snapshot.
			catalog := filepath.Join(t.TempDir(), "workbench-sessions.json")
			if err := os.WriteFile(catalog, []byte(`[{"sessionId":"sqlite-owned-history","title":"Preserved title"}]`), 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := sessionidentity.CreateOfflineSnapshot(ctx, profile, catalog, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			staged, err := sessionidentity.StageOfflineSnapshot(ctx, snapshot, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			restored := filepath.Join(staged, "profile")
			sourceDB := sha256.Sum256(readFile(identityPath))
			recovery := startSQLitePackagedSidecar(t, binary, restored, "")
			recovery.assertIdentity(t, id)
			recovery.stop(t)
			if sha256.Sum256(readFile(identityPath)) != sourceDB || !bytes.Equal(readFile(path), original) {
				t.Fatal("restore changed stopped source")
			}
			t.Setenv("REASONIX_HOME", restored)
			t.Setenv("REASONIX_STATE_HOME", restored)
			restoredPath, err := sessionpath.TranscriptPath(appconfig.SessionDir(), id)
			if err != nil || !bytes.Equal(readFile(restoredPath), original) {
				t.Fatal("restored JSONL differs", err)
			}
		})
	}
}

type sqlitePackagedSidecar struct {
	cmd         *exec.Cmd
	done        chan error
	base, ready string
	stopped     bool
	sequence    int
}

func startSQLitePackagedSidecar(t *testing.T, binary, profile, refusal string) *sqlitePackagedSidecar {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready.json")
	cmd := exec.Command(binary, sqlitePackagedSidecarArgs(runtime.GOOS, ready, os.Getpid())...)
	cmd.Dir = profile
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "REASONIX_HOME=" + profile, "REASONIX_STATE_HOME=" + profile, "REASONIX_CACHE_HOME=" + t.TempDir(), "REASONIX_CREDENTIALS_STORE=file"}
	cmd.Stdin = strings.NewReader(testToken + "\n")
	var diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &diagnostic, &diagnostic
	if err := cmd.Start(); err != nil {
		t.Fatal("packaged sidecar start failed", err)
	}
	p := &sqlitePackagedSidecar{cmd: cmd, done: make(chan error, 1), ready: ready}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() {
		if !p.stopped {
			_ = cmd.Process.Kill()
			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				t.Error("owned child survived cleanup")
			}
		}
	})
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-p.done:
			p.stopped = true
			if refusal != "" && err != nil && strings.Contains(diagnostic.String(), refusal) && strings.Contains(diagnostic.String(), "session identity") {
				if _, err := os.Lstat(ready); !os.IsNotExist(err) {
					t.Fatal("refused startup published readiness")
				}
				return nil
			}
			t.Fatal("packaged sidecar exited before readiness", err)
		case <-deadline.C:
			t.Fatal("packaged sidecar not ready")
		case <-ticker.C:
			data, err := os.ReadFile(ready)
			if err != nil {
				continue
			}
			var doc readyFile
			if json.Unmarshal(data, &doc) != nil {
				continue
			}
			if refusal != "" {
				t.Fatal("unsafe database passed startup gate")
			}
			host, _, err := net.SplitHostPort(doc.Address)
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
				t.Fatal("unsafe readiness address")
			}
			p.base = "http://" + doc.Address
			return p
		}
	}
}

func sqlitePackagedSidecarArgs(platform, ready string, parent int) []string {
	args := []string{"--ready-file", ready, "--launch-id", "sqlite-package-fixture"}
	// Only the macOS launcher implements this parent-ownership contract.
	// Linux/Windows must use the ordinary standalone protocol, as their hosts do.
	if platform == "darwin" {
		args = append(args, "--host-pid", strconv.Itoa(parent))
	}
	return args
}

func TestSQLitePackagedSidecarArgs(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			args := sqlitePackagedSidecarArgs(platform, "/private-fixture/ready.json", 1234)
			want := "--ready-file /private-fixture/ready.json --launch-id sqlite-package-fixture"
			if platform == "darwin" {
				want += " --host-pid 1234"
			}
			if strings.Join(args, " ") != want {
				t.Fatal("incorrect platform-specific package launch arguments")
			}
		})
	}
}

func (p *sqlitePackagedSidecar) call(t *testing.T, method, path string, body any, auth bool) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, p.base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.sequence++
	req.Header.Set(requestIDHeader, "sqlite-package-"+strconv.Itoa(p.sequence))
	if auth {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("packaged request failed", err)
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, reply
}

func (p *sqlitePackagedSidecar) assertIdentity(t *testing.T, id string) {
	t.Helper()
	status, reply := p.call(t, "GET", "/v1/sessions", nil, true)
	var doc sessionListResponse
	if status != 200 || json.Unmarshal(reply, &doc) != nil || len(doc.Sessions) != 1 || doc.Sessions[0].ID != id || doc.Sessions[0].Title != "Preserved title" || doc.Sessions[0].Missing {
		t.Fatal("packaged identity readback differs", status)
	}
}

func (p *sqlitePackagedSidecar) stop(t *testing.T) {
	t.Helper()
	if status, _ := p.call(t, "POST", "/v1:shutdown", map[string]any{}, true); status != 202 {
		t.Fatal("normal shutdown rejected", status)
	}
	select {
	case err := <-p.done:
		p.stopped = true
		if err != nil {
			t.Fatal("normal shutdown failed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("normal shutdown timed out")
	}
	if _, err := os.Lstat(p.ready); !os.IsNotExist(err) {
		t.Fatal("readiness survived shutdown")
	}
}
