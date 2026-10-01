package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeReadinessCleanupProtectsOtherFilesReplacementsAndAliases(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "reasonix-tauri-bridge-owned")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "ready.json")
	owned := nativeReadinessParent(path)
	if owned == nil {
		t.Fatal("native private directory not recognized")
	}
	canary := filepath.Join(parent, "other-file")
	if err := os.WriteFile(canary, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, owned)
	if data, err := os.ReadFile(canary); err != nil || string(data) != "keep" {
		t.Fatal("another file was removed")
	}
	if err := os.Rename(parent, parent+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, owned)
	if _, err := os.Stat(parent); err != nil {
		t.Fatal("replacement directory removed")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent+"-original", parent); err != nil {
		t.Fatal(err)
	}
	if nativeReadinessParent(path) != nil {
		t.Fatal("directory alias accepted")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	removeNativeReadinessParent(path, nativeReadinessParent(path))
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatal("owned empty directory remains")
	}
}
