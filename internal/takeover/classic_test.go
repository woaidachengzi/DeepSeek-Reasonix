package takeover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/provider"
)

func TestClassicPlanUsesCurrentTabsBeforeStaleV5AndKeepsMissingHistoryExplicit(t *testing.T) {
	copyRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	dir := filepath.Join(copyRoot, "projects", appconfig.WorkspaceSlug("/project"), "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	makeSession := func(name string, when time.Time) string {
		t.Helper()
		path := filepath.Join(dir, name+".jsonl")
		s := agent.NewSession("system")
		s.Add(provider.Message{Role: provider.RoleUser, Content: "question"})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: name})
		if err := s.SaveIfAbsent(path); err != nil {
			t.Fatal(err)
		}
		if err := agent.SaveBranchMetaPreserveUpdated(path, agent.BranchMeta{ID: agent.BranchID(path), TopicID: "current", TopicTitle: "current title", Scope: "project", WorkspaceRoot: "/project"}); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
		return path
	}
	selected := makeSession("selected", time.Now().Add(-time.Hour))
	makeSession("newer-copy", time.Now())
	projects := `{"globalTopics":[],"deletedTopics":["deleted"],"projects":[{"root":"/project","topics":["current","missing","deleted"]}]}`
	if err := os.WriteFile(filepath.Join(copyRoot, "desktop-projects.json"), []byte(projects), 0600); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(copyRoot, selected)
	tabs, _ := json.Marshal(map[string]any{"tabs": []map[string]string{{"topicId": "current", "workspaceRoot": "/project", "sessionPath": filepath.Join(sourceRoot, rel)}}})
	if err := os.WriteFile(filepath.Join(copyRoot, "desktop-tabs.json"), tabs, 0600); err != nil {
		t.Fatal(err)
	}
	fallback := []Row{{SourceID: "old-v5-current", SessionID: "takeover-old-current", WorkspaceRoot: "/project", TopicID: "current"}, {SourceID: "missing-v5", SessionID: "takeover-missing", WorkspaceRoot: "/project", TopicID: "missing"}, {SourceID: "unlisted-active", SessionID: "takeover-unlisted", WorkspaceRoot: "/project", TopicID: "unlisted"}, {SourceID: "deleted-v5", SessionID: "takeover-deleted", WorkspaceRoot: "/project", TopicID: "deleted", Archived: true}}
	rows, err := ClassicPlan(copyRoot, sourceRoot, fallback)
	if err != nil || len(rows) != 2 {
		t.Fatalf("plan: %+v %v", rows, err)
	}
	if rows[0].OriginalRelativePath != filepath.ToSlash(rel) || rows[0].SourceID != "current" {
		t.Fatalf("lost exact selected current history: %+v", rows[0])
	}
	if rows[1].SourceID != "missing-v5" || rows[1].OriginalRelativePath != "" {
		t.Fatalf("missing topic fallback: %+v", rows[1])
	}
	if _, err := ClassicPlan(copyRoot, sourceRoot); err == nil {
		t.Fatal("missing saved history silently dropped")
	}
}
