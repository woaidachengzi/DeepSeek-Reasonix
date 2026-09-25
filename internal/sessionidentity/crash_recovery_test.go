package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	crashRecoveryDBEnv   = "REASONIX_SESSIONIDENTITY_CRASH_DB"
	crashRecoveryRootEnv = "REASONIX_SESSIONIDENTITY_CRASH_ROOT"
	crashRecoveryExit    = 23
)

func TestSessionIdentityRecoversAfterAbruptProcessExit(t *testing.T) {
	if dbPath := os.Getenv(crashRecoveryDBEnv); dbPath != "" {
		if err := runCrashRecoveryWriter(dbPath, os.Getenv(crashRecoveryRootEnv)); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		// Deliberately bypass database/sql and Store cleanup to exercise WAL
		// recovery after the process disappears without a graceful close.
		os.Exit(crashRecoveryExit)
	}

	root := t.TempDir()
	dbPath := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	command := exec.Command(os.Args[0], "-test.run=^TestSessionIdentityRecoversAfterAbruptProcessExit$")
	command.Env = append(os.Environ(), crashRecoveryDBEnv+"="+dbPath, crashRecoveryRootEnv+"="+root)
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != crashRecoveryExit {
		t.Fatalf("crash writer exit = %v (code %d), output: %s", err, exitCode(exitError), output)
	}
	walInfo, err := os.Stat(dbPath + "-wal")
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("crash writer did not leave a WAL for recovery: info=%v err=%v", walInfo, err)
	}

	store, err := Open(context.Background(), dbPath, root)
	if err != nil {
		t.Fatalf("reopen after abrupt exit: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	records, err := store.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("recovered records = %#v, %v", records, err)
	}
	if records[0].Title != "committed title" || records[0].State != StateReady {
		t.Fatalf("recovered identity = %#v; uncommitted update must be rolled back", records[0])
	}
}

func runCrashRecoveryWriter(dbPath, root string) error {
	ctx := context.Background()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return err
	}
	transcript := filepath.Join(sessionDir, "tauri-crash-session.jsonl")
	if err := os.WriteFile(transcript, []byte("{\"role\":\"user\",\"content\":\"committed\"}\n"), 0o600); err != nil {
		return err
	}
	store, err := Open(ctx, dbPath, root)
	if err != nil {
		return err
	}
	if _, err := store.db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=0"); err != nil {
		return err
	}
	if err := store.Import(ctx, sessionDir, []Candidate{{
		ID: "crash-session", Path: transcript, Title: "committed title",
	}}); err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET title=? WHERE id=?", "uncommitted title", "crash-session"); err != nil {
		return err
	}
	// Intentionally leave tx and Store open; the parent verifies SQLite rolls
	// back this final, uncommitted update while retaining the earlier commit.
	return nil
}

func exitCode(err *exec.ExitError) int {
	if err == nil {
		return -1
	}
	return err.ExitCode()
}
