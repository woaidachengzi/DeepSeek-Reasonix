package sessionpath

import (
	"path/filepath"
	"testing"
)

func TestLegacyIdentityBindsPathAndSurvivesProfileMove(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("projects", "中文 workspace", "sessions", "20261012-model.jsonl")
	id, err := LegacyID(root, filepath.Join(root, rel))
	if err != nil || !ValidID(id) {
		t.Fatalf("legacy ID: %q %v", id, err)
	}
	moved := t.TempDir()
	copy, err := LegacyID(moved, filepath.Join(moved, rel))
	if err != nil || copy != id {
		t.Fatalf("profile move changed identity: %q %v", copy, err)
	}
	other, err := LegacyID(root, filepath.Join(root, "sessions", "20261012-model.jsonl"))
	if err != nil || other == id {
		t.Fatal("same basename in different projects collided")
	}
	for _, path := range []string{filepath.Join(root, "..", "outside.jsonl"), filepath.Join(root, "directory"), "relative.jsonl"} {
		if _, err := LegacyID(root, path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}
