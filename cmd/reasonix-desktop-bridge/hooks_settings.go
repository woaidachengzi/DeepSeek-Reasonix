package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/fileutil"
	fileencoding "reasonix/internal/fileutil/encoding"
	"reasonix/internal/hook"
)

const maxHooksSettingsBytes = 1 << 20

var hooksSettingsMu sync.Mutex
var errHooksSettingsConflict = errors.New("hooks settings changed on disk; reload before saving")

type previewHooksSettingsView struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Scope           string          `json:"scope"`
	Path            string          `json:"path"`
	ProjectRoot     string          `json:"projectRoot"`
	Revision        string          `json:"revision"`
	Hooks           json.RawMessage `json:"hooks"`
	Events          []string        `json:"events"`
}

type previewHooksSettingsChange struct {
	Scope         string          `json:"scope"`
	WorkspaceRoot string          `json:"workspaceRoot"`
	Revision      string          `json:"revision"`
	Hooks         json.RawMessage `json:"hooks"`
}

func hooksSettingsPath(scope, workspaceRoot string) (path, root string, err error) {
	switch scope {
	case "global":
		return hook.GlobalSettingsPath(""), "", nil
	case "project":
		root, err = normalizeSkillsWorkspace(workspaceRoot)
		if err != nil {
			return "", "", err
		}
		if root == "" {
			return "", "", fmt.Errorf("project workspace is required")
		}
		return hook.ProjectSettingsPath(root), root, nil
	default:
		return "", "", fmt.Errorf("invalid hook scope")
	}
}

func readHooksSettingsDocument(path string) (map[string]json.RawMessage, string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, "missing", nil
	}
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxHooksSettingsBytes {
		return nil, "", fmt.Errorf("hooks settings file is not a supported regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxHooksSettingsBytes {
		return nil, "", fmt.Errorf("hooks settings file is too large")
	}
	revisionBytes := sha256.Sum256(raw)
	decoded := fileencoding.DecodeToUTF8(raw)
	var document map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &document); err != nil || document == nil {
		return nil, "", fmt.Errorf("hooks settings file is not a JSON object")
	}
	return document, hex.EncodeToString(revisionBytes[:]), nil
}

func validateHooksJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("hooks object is required")
	}
	var byEvent map[string][]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byEvent); err != nil || byEvent == nil {
		return nil, fmt.Errorf("hooks must be a JSON object of event arrays")
	}
	if len(byEvent) > len(hook.Events) {
		return nil, fmt.Errorf("too many hook events")
	}
	count := 0
	for event, items := range byEvent {
		if !slices.Contains(hook.Events, hook.Event(event)) {
			return nil, fmt.Errorf("unknown hook event %q", event)
		}
		count += len(items)
		if count > 256 {
			return nil, fmt.Errorf("too many hooks")
		}
		for _, item := range items {
			if item == nil {
				return nil, fmt.Errorf("hook entry must be an object")
			}
			var command string
			if err := json.Unmarshal(item["command"], &command); err != nil || strings.TrimSpace(command) == "" || len(command) > 4096 {
				return nil, fmt.Errorf("hook command is required and must be at most 4096 bytes")
			}
			command = hook.NormalizeCommand(strings.TrimSpace(command))
			item["command"], _ = json.Marshal(command)
			for _, field := range []string{"match", "description", "cwd"} {
				if value, ok := item[field]; ok {
					var s string
					if err := json.Unmarshal(value, &s); err != nil || len(s) > 4096 {
						return nil, fmt.Errorf("invalid hook %s", field)
					}
					if field == "match" && s != "" && s != "*" {
						if _, err := regexp.Compile("^(?:" + s + ")$"); err != nil {
							return nil, fmt.Errorf("invalid hook match pattern")
						}
					}
				}
			}
			if value, ok := item["timeout"]; ok {
				var timeout int
				if err := json.Unmarshal(value, &timeout); err != nil || timeout < 0 || timeout > 600000 {
					return nil, fmt.Errorf("invalid hook timeout")
				}
			}
		}
	}
	return json.Marshal(byEvent)
}

func loadHooksSettings(scope, workspaceRoot string) (previewHooksSettingsView, error) {
	path, root, err := hooksSettingsPath(scope, workspaceRoot)
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	document, revision, err := readHooksSettingsDocument(path)
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	hooks := json.RawMessage(`{}`)
	if value, ok := document["hooks"]; ok {
		hooks, err = validateHooksJSON(value)
		if err != nil {
			return previewHooksSettingsView{}, err
		}
	}
	events := make([]string, 0, len(hook.Events))
	for _, event := range hook.Events {
		events = append(events, string(event))
	}
	return previewHooksSettingsView{ProtocolVersion: desktopbridge.ProtocolVersion, Scope: scope,
		Path: path, ProjectRoot: root, Revision: revision, Hooks: hooks, Events: events}, nil
}

func persistHooksSettings(change previewHooksSettingsChange) (previewHooksSettingsView, error) {
	path, _, err := hooksSettingsPath(change.Scope, change.WorkspaceRoot)
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	hooks, err := validateHooksJSON(change.Hooks)
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	hooksSettingsMu.Lock()
	defer hooksSettingsMu.Unlock()
	document, revision, err := readHooksSettingsDocument(path)
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	if revision != change.Revision {
		return previewHooksSettingsView{}, errHooksSettingsConflict
	}
	document["hooks"] = hooks
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return previewHooksSettingsView{}, err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return previewHooksSettingsView{}, err
	}
	if err := fileutil.AtomicWriteFileStrict(path, body, 0o600); err != nil {
		return previewHooksSettingsView{}, err
	}
	return loadHooksSettings(change.Scope, change.WorkspaceRoot)
}
