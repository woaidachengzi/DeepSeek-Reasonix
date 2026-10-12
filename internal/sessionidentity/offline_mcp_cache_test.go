package sessionidentity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineSnapshotSkipsOnlyMCPRuntimeCacheAndRetainsServerState(t *testing.T) {
	root := t.TempDir()
	profile, backups := filepath.Join(root, "profile"), filepath.Join(root, "backups")
	if err := os.MkdirAll(backups, 0700); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, filepath.Join(profile, "sessions", "tauri-one.jsonl"))
	catalog := filepath.Join(root, "workbench-sessions.json")
	writeCatalog(t, catalog, nil)
	for _, dir := range []string{"mcp-state/workspace/server", "mcp-state/legacy-server"} {
		cache := filepath.Join(profile, filepath.FromSlash(dir), "cache", "npm", ".bin")
		if err := os.MkdirAll(cache, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(cache)), "runtime.txt"), []byte("regenerable"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(cache, "tool")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := os.WriteFile(filepath.Join(profile, filepath.FromSlash(dir), "config.json"), []byte("important server config"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	important := filepath.Join(profile, "projects", "my-project", "cache", "saved.json")
	if err := os.MkdirAll(filepath.Dir(important), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(important, []byte("project data"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CreateOfflineSnapshot(context.Background(), profile, catalog, backups)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"mcp-state/workspace/server", "mcp-state/legacy-server"} {
		base := filepath.Join(snapshot, "profile", filepath.FromSlash(dir))
		if _, err := os.Stat(filepath.Join(base, "cache")); !os.IsNotExist(err) {
			t.Fatalf("included runtime cache: %v", err)
		}
		if raw, err := os.ReadFile(filepath.Join(base, "config.json")); err != nil || string(raw) != "important server config" {
			t.Fatalf("lost config: %s %v", raw, err)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(snapshot, "profile", "projects", "my-project", "cache", "saved.json")); err != nil || string(raw) != "project data" {
		t.Fatalf("lost project data: %s %v", raw, err)
	}
	if _, err := VerifyOfflineSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
}
