package takeover

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/provider"
	"reasonix/internal/sessionarchive"
	"reasonix/internal/sessionidentity"
)

func TestWritePreservesOriginalFilesAndCompletesIndependentTauriState(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "config.toml")
	if err := os.WriteFile(original, []byte("original config"), 0600); err != nil {
		t.Fatal(err)
	}
	history := []provider.Message{{Role: provider.RoleSystem, Content: "system"}, {Role: provider.RoleUser, Content: "old question"}, {Role: provider.RoleAssistant, Content: "old answer"}}
	rows := []Row{{SourceID: "a", SessionID: "takeover-a", Title: "原始对话", Export: Export{Messages: history, ModelMessages: history, ModelRef: "provider/model"}}, {SourceID: "b", SessionID: "takeover-b", Title: "归档对话", Archived: true, Export: Export{Messages: history, ModelMessages: history, ModelRef: "provider/model"}}}
	receipt, err := Write(context.Background(), root, "/private/backup", rows)
	if err != nil || len(receipt.Rows) != 2 {
		t.Fatalf("write: %+v %v", receipt, err)
	}
	if raw, err := os.ReadFile(original); err != nil || string(raw) != "original config" {
		t.Fatalf("changed original config: %s %v", raw, err)
	}
	path, _ := sessionpath.TranscriptPath(filepath.Join(root, "sessions"), rows[0].SessionID)
	loaded, err := agent.LoadSession(path)
	if err != nil || len(loaded.Snapshot()) != 3 {
		t.Fatalf("history: %v", err)
	}
	identities, err := sessionidentity.OpenReadOnly(context.Background(), filepath.Join(root, "desktop", "session-state-v1.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	list, err := identities.List(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("identity list: %+v %v", list, err)
	}
	archives, err := sessionarchive.List(filepath.Join(root, "desktop", "session-archives-v1.json"))
	if err != nil || len(archives) != 1 || archives[0].SessionID != "takeover-b" {
		t.Fatalf("archives: %+v %v", archives, err)
	}
	if _, err := Write(context.Background(), root, "/private/backup", rows); err == nil {
		t.Fatal("replaced existing takeover")
	}
}
