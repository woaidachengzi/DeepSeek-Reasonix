package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPRemovalDoesNotIgnoreUnreadableLegacySource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	legacy := filepath.Join(home, ".reasonix", "config.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	project := filepath.Join(root, "reasonix.toml")
	before := []byte("[[plugins]]\nname = \"mock\"\ncommand = \"never-started\"\n")
	if err := os.WriteFile(project, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := RemovePluginFromEffectiveSourceForRoot(root, "mock"); err == nil {
		t.Fatal("malformed legacy source did not block removal")
	}
	if got, err := os.ReadFile(project); err != nil || string(got) != string(before) {
		t.Fatal("removal modified configuration before validating all sources")
	}
	if got := loadLegacyMCP(legacy); len(got) != 0 {
		t.Fatal("ordinary startup no longer tolerates malformed legacy configuration")
	}
}
