package takeover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/provider"
)

func TestPlanPreservesAuthoritativeMembershipTitlesAndArchive(t *testing.T) {
	root, exports := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "desktop"), 0700); err != nil {
		t.Fatal(err)
	}
	state := `{"version":1,"initialized":true,"workspaceIds":["global","project"],"workspaces":{"global":{"root":"/original/global-workspace","sessionIds":["a"]},"project":{"root":"/project","sessionIds":["b"]}},"archivedSessionIds":["b"],"sessionStates":{"a":{"lifecycle":"active"},"b":{"lifecycle":"archived"}},"presentation":{"a":{"title":"原手工标题"},"b":{"title":"归档标题"}}}`
	path := filepath.Join(root, "desktop", "workspace-state-v1.json")
	if err := os.WriteFile(path, []byte(state), 0600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "orphan"} {
		item := Export{Version: 1, SessionID: id, Title: "older title", Messages: []provider.Message{{Role: provider.RoleSystem, Content: "system"}}, ModelMessages: []provider.Message{{Role: provider.RoleSystem, Content: "system"}}}
		item.Header.SessionID = id
		data, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exports, id+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := Plan(root, "/original", exports)
	if err != nil || len(rows) != 2 || rows[0].Title != "原手工标题" || rows[0].WorkspaceRoot != "" || rows[0].Archived || rows[1].WorkspaceRoot != "/project" || !rows[1].Archived {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	if err := os.Remove(filepath.Join(exports, "b.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Plan(root, "/original", exports); err == nil {
		t.Fatal("missing authoritative history was silently dropped")
	}
}
