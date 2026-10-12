// Package takeover adapts a verified, offline Wails export into the Tauri
// profile without replacing the original Wails storage or configuration.
package takeover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/provider"
)

type Export struct {
	Version   int    `json:"version"`
	SessionID string `json:"sessionId"`
	Header    struct {
		SessionID string    `json:"sessionId"`
		CreatedAt time.Time `json:"createdAt"`
	} `json:"header"`
	SourceSHA256  string             `json:"sourceSha256"`
	Title         string             `json:"title"`
	ModelRef      string             `json:"modelRef"`
	Messages      []provider.Message `json:"messages"`
	ModelMessages []provider.Message `json:"modelMessages"`
}

type WorkspaceState struct {
	Version      int      `json:"version"`
	Initialized  bool     `json:"initialized"`
	WorkspaceIDs []string `json:"workspaceIds"`
	Workspaces   map[string]struct {
		Root       string   `json:"root"`
		Title      string   `json:"title"`
		SessionIDs []string `json:"sessionIds"`
	} `json:"workspaces"`
	ArchivedSessionIDs []string `json:"archivedSessionIds"`
	SessionStates      map[string]struct {
		Lifecycle string `json:"lifecycle"`
	} `json:"sessionStates"`
	Presentation map[string]struct {
		TopicID   string `json:"topicId"`
		Title     string `json:"title"`
		SortOrder int    `json:"sortOrder"`
	} `json:"presentation"`
}

type Row struct {
	OriginalRelativePath string    `json:"originalRelativePath,omitempty"`
	SourceID             string    `json:"sourceId"`
	SessionID            string    `json:"sessionId"`
	WorkspaceRoot        string    `json:"workspaceRoot,omitempty"`
	Title                string    `json:"title"`
	TopicID              string    `json:"topicId,omitempty"`
	Archived             bool      `json:"archived"`
	CreatedAt            time.Time `json:"createdAt"`
	Export               Export    `json:"-"`
}

// Plan uses the authoritative workspace membership rather than scanning old
// legacy copies. Exports not listed in that state are not silently imported.
func Plan(profileCopy, sourceProfile, exports string) ([]Row, error) {
	raw, err := os.ReadFile(filepath.Join(profileCopy, "desktop", "workspace-state-v1.json"))
	if err != nil {
		return nil, err
	}
	var state WorkspaceState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if (state.Version < 1 || state.Version > 3) || !state.Initialized {
		return nil, fmt.Errorf("unsupported or uninitialized Wails workspace state")
	}
	archived := map[string]bool{}
	for _, id := range state.ArchivedSessionIDs {
		archived[id] = true
	}
	seen := map[string]bool{}
	rows := []Row{}
	for _, workspaceID := range state.WorkspaceIDs {
		workspace, exists := state.Workspaces[workspaceID]
		if !exists {
			return nil, fmt.Errorf("workspace membership is missing")
		}
		root := strings.TrimSpace(workspace.Root)
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("workspace root must be absolute")
		}
		if filepath.Clean(root) == filepath.Join(sourceProfile, "global-workspace") {
			root = ""
		}
		for _, id := range workspace.SessionIDs {
			if seen[id] {
				return nil, fmt.Errorf("source session belongs to multiple workspaces: %s", id)
			}
			seen[id] = true
			lifecycle := state.SessionStates[id].Lifecycle
			if lifecycle != "active" && lifecycle != "archived" {
				return nil, fmt.Errorf("unsupported source lifecycle for %s", id)
			}
			if filepath.Base(id) != id || id == "." || id == ".." {
				return nil, fmt.Errorf("invalid source session ID")
			}
			data, err := os.ReadFile(filepath.Join(exports, id+".json"))
			if err != nil {
				return nil, err
			}
			var item Export
			if err := json.Unmarshal(data, &item); err != nil {
				return nil, err
			}
			if item.Version != 1 || item.SessionID != id || item.Header.SessionID != id || len(item.Messages) == 0 || len(item.ModelMessages) == 0 {
				return nil, fmt.Errorf("invalid or empty export for %s", id)
			}
			presentation := state.Presentation[id]
			title := strings.TrimSpace(presentation.Title)
			if title == "" {
				title = strings.TrimSpace(item.Title)
			}
			if title == "" {
				title = "对话"
			}
			if len([]rune(title)) > 120 {
				return nil, fmt.Errorf("source title exceeds Tauri title limit; retain it for review")
			}
			rows = append(rows, Row{SourceID: id, SessionID: "takeover-" + id, WorkspaceRoot: root, Title: title, TopicID: presentation.TopicID, Archived: archived[id] || lifecycle == "archived", CreatedAt: item.Header.CreatedAt, Export: item})
		}
	}
	for id := range archived {
		if !seen[id] {
			return nil, fmt.Errorf("archived source is not represented in workspace membership: %s", id)
		}
	}
	return rows, nil
}
