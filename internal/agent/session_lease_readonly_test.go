package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionLeaseReadOnlyProbeNeverRepairsOrCreatesLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	infoPath := sessionLeaseInfoPath(path)
	info := SessionLeaseInfo{PID: os.Getpid() + 100000, WriterID: "foreign-or-stale"}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(infoPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, bytes := range [][]byte{raw, []byte("corrupt metadata")} {
		if err := os.WriteFile(infoPath, bytes, 0600); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if !SessionLeaseHeldByOtherRuntimeReadOnly(path) {
			t.Fatal("foreign/unknown owner admitted")
		}
		after, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(infoPath)
		if err != nil || string(current) != string(bytes) || len(before) != len(after) {
			t.Fatal("read-only probe repaired metadata or created lock", err)
		}
	}
	if err := os.Remove(infoPath); err != nil {
		t.Fatal(err)
	}
	if SessionLeaseHeldByOtherRuntimeReadOnly(path) {
		t.Fatal("absent metadata became foreign")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 0 {
		t.Fatal("empty observation created files")
	}
	sessionLeaseActiveOwners.Store(canonicalSessionSavePath(path), struct{}{})
	defer sessionLeaseActiveOwners.Delete(canonicalSessionSavePath(path))
	if SessionLeaseHeldByOtherRuntimeReadOnly(path) {
		t.Fatal("active in-process lease rejected")
	}
}
