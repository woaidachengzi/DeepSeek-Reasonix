package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/installsource"
	"reasonix/internal/skill"
)

type previewArchivedSkill struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ArchiveID string `json:"archiveId"`
	Path      string `json:"path"`
	Revision  string `json:"revision"`
}

type previewSkillArchiveRequest struct {
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	WorkspaceRoot string `json:"workspaceRoot"`
	ArchiveID     string `json:"archiveId"`
	Revision      string `json:"revision"`
}

type previewSkillArchiveResult struct {
	ProtocolVersion int                `json:"protocolVersion"`
	BackupPath      string             `json:"backupPath"`
	Settings        skillsSettingsView `json:"settings"`
}

func previewSkillPaths(scope, workspaceRoot, name string) (directory, archiveRoot string, err error) {
	if !appconfig.IsValidSkillName(name) {
		return "", "", fmt.Errorf("invalid skill name")
	}
	var base string
	switch scope {
	case "global":
		base = appconfig.ReasonixHomeDir()
	case "project":
		if workspaceRoot == "" {
			return "", "", fmt.Errorf("project skill requires a workspace")
		}
		base = filepath.Join(workspaceRoot, ".reasonix")
	default:
		return "", "", fmt.Errorf("invalid skill scope")
	}
	if base == "" {
		return "", "", fmt.Errorf("skill profile is unavailable")
	}
	return filepath.Join(base, skill.SkillsDirname, name), filepath.Join(base, "removed-skills"), nil
}

func plainDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill directory is not a plain directory")
	}
	return nil
}

func previewCanonicalSkillRevision(scope, workspaceRoot, name, sourcePath string) (string, error) {
	directory, _, err := previewSkillPaths(scope, workspaceRoot, name)
	if err != nil || sourcePath != filepath.Join(directory, skill.SkillFile) {
		return "", fmt.Errorf("skill is not in the selected scope's standard directory")
	}
	if err := plainDirectory(filepath.Dir(filepath.Dir(directory))); err != nil {
		return "", err
	}
	if err := plainDirectory(filepath.Dir(directory)); err != nil {
		return "", err
	}
	if err := plainDirectory(directory); err != nil {
		return "", err
	}
	info, err := os.Lstat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("skill entry is not a regular file")
	}
	return installsource.DigestSkillDirectory(directory)
}

func previewArchiveIDName(id string) (string, bool) {
	index := strings.LastIndex(id, "--")
	if index <= 0 || len(id[index+2:]) != 24 {
		return "", false
	}
	if _, err := hex.DecodeString(id[index+2:]); err != nil || !appconfig.IsValidSkillName(id[:index]) {
		return "", false
	}
	return id[:index], true
}

func listPreviewArchivedSkills(workspaceRoot string) ([]previewArchivedSkill, error) {
	archived := []previewArchivedSkill{}
	for _, scope := range []string{"global", "project"} {
		if scope == "project" && workspaceRoot == "" {
			continue
		}
		_, root, err := previewSkillPaths(scope, workspaceRoot, "placeholder")
		if err != nil {
			return nil, err
		}
		if err := plainDirectory(root); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if len(archived) >= 100 {
				return archived, nil
			}
			name, ok := previewArchiveIDName(entry.Name())
			if !ok || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(root, entry.Name())
			info, err := os.Lstat(filepath.Join(path, skill.SkillFile))
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if installsource.ValidateSkillFile(filepath.Join(path, skill.SkillFile), name) != nil {
				continue
			}
			revision, err := installsource.DigestSkillDirectory(path)
			if err != nil {
				continue
			}
			archived = append(archived, previewArchivedSkill{Name: name, Scope: scope, ArchiveID: entry.Name(), Path: path, Revision: revision})
		}
	}
	return archived, nil
}

func (b *bridgeServer) archiveSkill(w http.ResponseWriter, r *http.Request) {
	var input previewSkillArchiveRequest
	if decodeJSONBody(w, r, 64<<10, &input) != nil || input.ArchiveID != "" || !strings.HasPrefix(input.Revision, "sha256:") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "select a current skill to move to backup")
		return
	}
	root, err := normalizeSkillsWorkspace(input.WorkspaceRoot)
	if err != nil || input.Scope == "project" && root == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid skill workspace")
		return
	}
	b.packageOpsMu.Lock()
	defer b.packageOpsMu.Unlock()
	directory, archiveRoot, err := previewSkillPaths(input.Scope, root, input.Name)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid skill scope or name")
		return
	}
	revision, err := previewCanonicalSkillRevision(input.Scope, root, input.Name, filepath.Join(directory, skill.SkillFile))
	if err != nil || revision != input.Revision {
		writeProtocolError(w, http.StatusConflict, "skill_changed", "skill changed; refresh before moving it to backup")
		return
	}
	if err := os.MkdirAll(archiveRoot, 0o700); err != nil || plainDirectory(archiveRoot) != nil {
		writeProtocolError(w, http.StatusInternalServerError, "backup_failed", "skill backup directory is unavailable")
		return
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "backup_failed", "unable to prepare skill backup")
		return
	}
	backupPath := filepath.Join(archiveRoot, input.Name+"--"+hex.EncodeToString(random[:]))
	if err := os.Rename(directory, backupPath); err != nil {
		writeProtocolError(w, http.StatusConflict, "backup_failed", "skill could not be moved to backup")
		return
	}
	settings, err := loadSkillsSettings(root)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "skill was backed up but inventory could not be refreshed")
		return
	}
	writeJSON(w, http.StatusOK, previewSkillArchiveResult{ProtocolVersion: desktopbridge.ProtocolVersion, BackupPath: backupPath, Settings: settings})
}

func (b *bridgeServer) restoreSkill(w http.ResponseWriter, r *http.Request) {
	var input previewSkillArchiveRequest
	if decodeJSONBody(w, r, 64<<10, &input) != nil || !strings.HasPrefix(input.Revision, "sha256:") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "select a backed-up skill to restore")
		return
	}
	root, err := normalizeSkillsWorkspace(input.WorkspaceRoot)
	name, validID := previewArchiveIDName(input.ArchiveID)
	if err != nil || !validID || name != input.Name || input.Scope == "project" && root == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid skill backup")
		return
	}
	b.packageOpsMu.Lock()
	defer b.packageOpsMu.Unlock()
	directory, archiveRoot, err := previewSkillPaths(input.Scope, root, name)
	if err != nil || plainDirectory(archiveRoot) != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "skill backup is unavailable")
		return
	}
	backupPath := filepath.Join(archiveRoot, input.ArchiveID)
	if err := plainDirectory(backupPath); err != nil {
		writeProtocolError(w, http.StatusConflict, "backup_changed", "skill backup changed; refresh before restoring")
		return
	}
	if err := installsource.ValidateSkillFile(filepath.Join(backupPath, skill.SkillFile), name); err != nil {
		writeProtocolError(w, http.StatusConflict, "backup_changed", "skill backup is invalid; inspect it before restoring")
		return
	}
	revision, err := installsource.DigestSkillDirectory(backupPath)
	if err != nil || revision != input.Revision {
		writeProtocolError(w, http.StatusConflict, "backup_changed", "skill backup changed; refresh before restoring")
		return
	}
	if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		writeProtocolError(w, http.StatusConflict, "skill_exists", "a skill with this name already exists")
		return
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(directory), name+".md")); !errors.Is(err, os.ErrNotExist) {
		writeProtocolError(w, http.StatusConflict, "skill_exists", "a legacy skill with this name already exists")
		return
	}
	if err := plainDirectory(filepath.Dir(filepath.Dir(directory))); err != nil || plainDirectory(filepath.Dir(directory)) != nil || os.Rename(backupPath, directory) != nil {
		writeProtocolError(w, http.StatusConflict, "restore_failed", "skill could not be restored")
		return
	}
	settings, err := loadSkillsSettings(root)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "skill was restored but inventory could not be refreshed")
		return
	}
	writeJSON(w, http.StatusOK, previewSkillArchiveResult{ProtocolVersion: desktopbridge.ProtocolVersion, Settings: settings})
}
