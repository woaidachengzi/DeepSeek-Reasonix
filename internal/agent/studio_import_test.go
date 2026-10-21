package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/provider"
	"reasonix/internal/store"
)

func TestStudioImportsAreNotListedOrMaintained(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v4-foreign.jsonl")
	s := NewSession("sys")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "foreign conversation"})
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	files := []string{store.SessionLeaseInfo(path), store.SessionLockFile(path), store.SessionLeaseLock(path)}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("foreign-owned"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ReconcileSessionSidecars(dir); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if raw, err := os.ReadFile(file); err != nil || string(raw) != "foreign-owned" {
			t.Errorf("foreign sidecar changed: %s (%v)", filepath.Base(file), err)
		}
	}
	if sessions, err := ListSessions(dir); err != nil || len(sessions) != 0 {
		t.Fatalf("Studio import appeared in the session list: %+v (%v)", sessions, err)
	}
}

func TestStudioImportsAreNotRecoveryGarbage(t *testing.T) {
	dir := t.TempDir()
	parent, branch, messages := forkRecoveryBranch(t, dir, "v4-import")
	coverBranchInParent(t, parent, messages)
	if got, err := ReclaimableRecoveryBranches(dir, time.Now().Add(48*time.Hour), RecoveryGCGracePeriod); err != nil || len(got) != 0 {
		t.Fatalf("foreign recovery branch selected: %v (%v)", got, err)
	}
	if err := TrashReclaimableRecoveryBranch(branch, dir); err == nil {
		t.Fatal("foreign recovery branch accepted for trash")
	}
	if _, err := os.Stat(branch); err != nil {
		t.Fatalf("foreign recovery branch removed: %v", err)
	}
}

func TestStudioCleanupMarkersAreNotRetried(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "v4-import"+cleanupPendingExt)
	if err := os.WriteFile(marker, []byte(`{"reason":"foreign owner"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if pending, err := ListCleanupPending(dir); err != nil || len(pending) != 0 {
		t.Fatalf("foreign cleanup marker selected: %v (%v)", pending, err)
	}
}
