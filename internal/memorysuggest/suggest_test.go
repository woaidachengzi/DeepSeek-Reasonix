package memorysuggest

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/memory"
	"reasonix/internal/provider"
)

func TestLoadSessionsUsesOnlyAllowedIdentities(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"allowed.jsonl", "other.jsonl"} {
		session := agent.NewSession("")
		session.Add(provider.Message{Role: provider.RoleUser, Content: "以后请始终用中文回复。"})
		if err := session.Save(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	sessions := LoadSessions(dir, 12, map[string]struct{}{"allowed": {}})
	if len(sessions) != 1 || sessions[0].ID != "allowed" {
		t.Fatalf("LoadSessions() = %#v, want only allowed identity", sessions)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateIsReadOnlyAndReturnsCandidates(t *testing.T) {
	set := memory.Load(memory.Options{CWD: t.TempDir(), UserDir: t.TempDir()})
	session := agent.NewSession("")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "以后请始终用中文回复，除非我明确要求英文。"})
	view := Generate(set, set.CWD, nil, []SuggestionSession{{ID: "local-session", Messages: session.Snapshot()}})
	if !view.Available || view.Memories == nil || view.Skills == nil || len(view.Memories) != 1 {
		t.Fatalf("Generate() = %#v, want one read-only memory candidate", view)
	}
	if got := set.Store.ListAll(); len(got) != 0 {
		t.Fatalf("Generate() wrote memories: %#v", got)
	}
}
