package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

const (
	legacyProjectFoldersFile = "desktop-projects.json"
	maxProjectFoldersFile    = 4 << 20
	maxProjectFolderCount    = 10_000
)

type projectFolder struct {
	Root  string `json:"root"`
	Title string `json:"title,omitempty"`
}

type projectFoldersResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Projects        []projectFolder `json:"projects"`
}

// projectFolders is a read-only compatibility view of saved workspace
// folders. Wails persists this file with atomic replacement, so a concurrent
// legacy write yields either the old or new complete snapshot. Tauri never
// writes the Wails-owned project file.
func (b *bridgeServer) projectFolders(w http.ResponseWriter, _ *http.Request) {
	home := strings.TrimSpace(appconfig.ReasonixHomeDir())
	if home == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "project folder storage is unavailable")
		return
	}
	path := filepath.Join(home, legacyProjectFoldersFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, projectFoldersResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Projects: []projectFolder{}})
		return
	}
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect saved project folders")
		return
	}
	if !info.Mode().IsRegular() || info.Size() > maxProjectFoldersFile {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved project folder file is invalid")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read saved project folders")
		return
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved project folder file changed while opening")
		return
	}
	body, err := io.ReadAll(io.LimitReader(file, maxProjectFoldersFile+1))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read saved project folders")
		return
	}
	if len(body) > maxProjectFoldersFile {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved project folder file is too large")
		return
	}
	var stored struct {
		Projects []struct {
			Root  string `json:"root"`
			Title string `json:"title"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &stored); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved project folder file is malformed")
		return
	}
	if len(stored.Projects) > maxProjectFolderCount {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "saved project folder file contains too many entries")
		return
	}
	projects := make([]projectFolder, 0, len(stored.Projects))
	seen := make(map[string]struct{}, len(stored.Projects))
	for _, project := range stored.Projects {
		root := project.Root
		if strings.TrimSpace(root) == "" || len(root) > 4096 || strings.IndexFunc(root, unicode.IsControl) >= 0 || !filepath.IsAbs(root) {
			continue
		}
		key := projectFolderKey(root)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		title := strings.TrimSpace(project.Title)
		if utf8.RuneCountInString(title) > 1024 {
			title = string([]rune(title)[:1024])
		}
		projects = append(projects, projectFolder{Root: root, Title: title})
	}
	writeJSON(w, http.StatusOK, projectFoldersResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Projects: projects})
}

func projectFolderKey(root string) string {
	if runtime.GOOS == "windows" {
		normalized := strings.ReplaceAll(root, "/", `\`)
		hasTrailingSeparator := strings.HasSuffix(normalized, `\`)
		trimmed := strings.TrimRight(normalized, `\`)
		if trimmed == "" {
			if strings.HasPrefix(root, "/") {
				return `\`
			}
			return root[:1]
		}
		isDriveRoot := len(trimmed) == 2 && trimmed[1] == ':' &&
			((trimmed[0] >= 'a' && trimmed[0] <= 'z') || (trimmed[0] >= 'A' && trimmed[0] <= 'Z'))
		if hasTrailingSeparator && isDriveRoot {
			trimmed += `\`
		}
		return strings.ToLower(trimmed)
	}
	key := strings.TrimRight(root, "/")
	if key == "" && strings.HasPrefix(root, "/") {
		return "/"
	}
	return key
}
