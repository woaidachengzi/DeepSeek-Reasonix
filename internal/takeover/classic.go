package takeover

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
)

// ClassicPlan follows the current Wails project/topic list and its most recent
// metadata-backed JSONL versions. Older v5 experiment directories are not
// automatically treated as current merely because they still exist.
func ClassicPlan(profileCopy, sourceProfile string, supplementary ...[]Row) ([]Row, error) {
	var projects struct {
		GlobalTopics  []string `json:"globalTopics"`
		DeletedTopics []string `json:"deletedTopics"`
		Projects      []struct {
			Root   string   `json:"root"`
			Topics []string `json:"topics"`
		} `json:"projects"`
	}
	data, err := os.ReadFile(filepath.Join(profileCopy, "desktop-projects.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	var tabs struct {
		Tabs []struct {
			TopicID       string `json:"topicId"`
			SessionPath   string `json:"sessionPath"`
			WorkspaceRoot string `json:"workspaceRoot"`
		} `json:"tabs"`
	}
	if data, err := os.ReadFile(filepath.Join(profileCopy, "desktop-tabs.json")); err == nil {
		if err := json.Unmarshal(data, &tabs); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	type scope struct {
		Root   string
		Topics []string
	}
	scopes := []scope{{Root: "", Topics: projects.GlobalTopics}}
	for _, project := range projects.Projects {
		scopes = append(scopes, scope{Root: project.Root, Topics: project.Topics})
	}
	deleted := map[string]bool{}
	for _, id := range projects.DeletedTopics {
		deleted[id] = true
	}
	rows := []Row{}
	fallback := map[string]Row{}
	for _, group := range supplementary {
		for _, row := range group {
			if row.TopicID != "" {
				fallback[row.WorkspaceRoot+"\x00"+row.TopicID] = row
			}
		}
	}
	for _, scope := range scopes {
		workspace := scope.Root
		if workspace == "" {
			workspace = filepath.Join(sourceProfile, "global-workspace")
		}
		if !filepath.IsAbs(workspace) {
			return nil, fmt.Errorf("saved workspace root is not absolute")
		}
		base := filepath.Join(profileCopy, "projects", appconfig.WorkspaceSlug(workspace))
		sessions, err := agent.ListSessionOrder(filepath.Join(base, "sessions"))
		if err != nil {
			return nil, err
		}
		titles, err := classicTitles(filepath.Join(base, "desktop", "topic-state-v1.sqlite"))
		if err != nil {
			return nil, err
		}
		if scope.Root == "" {
			globalTitles, err := classicTitles(filepath.Join(profileCopy, "desktop", "topic-state-v1.sqlite"))
			if err != nil {
				return nil, err
			}
			for id, title := range globalTitles {
				titles[id] = title
			}
		}
		for _, topic := range scope.Topics {
			if deleted[topic] {
				continue
			}
			var chosen *agent.SessionOrderInfo
			for index := range sessions {
				if sessions[index].TopicID == topic {
					chosen = &sessions[index]
					break
				}
			}
			// The open tab is the user's exact selected version, not a guessed
			// newer recovery copy or an old default branch.
			for _, tab := range tabs.Tabs {
				if tab.TopicID != topic || tab.WorkspaceRoot != scope.Root || tab.SessionPath == "" {
					continue
				}
				rel, err := filepath.Rel(sourceProfile, tab.SessionPath)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return nil, fmt.Errorf("saved tab transcript escapes its profile")
				}
				candidate := filepath.Join(profileCopy, rel)
				for index := range sessions {
					if sessions[index].Path == candidate && sessions[index].TopicID == topic {
						chosen = &sessions[index]
						break
					}
				}
			}
			if chosen == nil {
				if row, exists := fallback[scope.Root+"\x00"+topic]; exists {
					row.Archived = false
					if title := strings.TrimSpace(titles[topic]); title != "" {
						row.Title = title
					}
					rows = append(rows, row)
					continue
				}
				return nil, fmt.Errorf("saved topic %s has no readable selected transcript; review before takeover", topic)
			}
			rel, err := filepath.Rel(profileCopy, chosen.Path)
			if err != nil {
				return nil, err
			}
			original := filepath.Join(sourceProfile, rel)
			id, err := sessionpath.LegacyID(sourceProfile, original)
			if err != nil {
				return nil, err
			}
			title := strings.TrimSpace(titles[topic])
			if title == "" {
				title = strings.TrimSpace(chosen.TopicTitle)
			}
			if title == "" {
				title = strings.TrimSpace(chosen.CustomTitle)
			}
			if title == "" {
				title = "对话"
			}
			rows = append(rows, Row{OriginalRelativePath: filepath.ToSlash(rel), SourceID: topic, SessionID: id, WorkspaceRoot: scope.Root, Title: title, TopicID: topic, CreatedAt: chosen.CreatedAt})
		}
	}
	// Current topic membership supersedes stale v5 active rows. Archived v5
	// history remains available, unless the current user explicitly deleted it.
	current := map[string]bool{}
	for _, row := range rows {
		current[row.WorkspaceRoot+"\x00"+row.TopicID] = true
	}
	for _, row := range fallback {
		if row.Archived && !deleted[row.TopicID] && !current[row.WorkspaceRoot+"\x00"+row.TopicID] {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func classicTitles(path string) (map[string]string, error) {
	values := map[string]string{}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("topic metadata must be a plain SQLite file")
	}
	// The snapshot was taken with writers stopped. Do not let SQLite create
	// journal/shared-memory files inside the verified snapshot.
	if _, err := os.Stat(path + "-wal"); err == nil {
		return nil, fmt.Errorf("topic metadata has a WAL; stage and checkpoint its private copy before takeover")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("immutable", "1")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT topic_id,title FROM topics")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		values[id] = title
	}
	return values, rows.Err()
}
