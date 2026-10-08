package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/fileref"
)

const workspaceReferenceWalkLimit = 10000

func isBareWorkspaceFilename(path string) bool {
	name := strings.TrimSpace(path)
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\:\x00")
}

// findWorkspaceFilename accepts only a unique, exact filename in the visible
// workspace. It never follows directory symlinks, and an incomplete scan must
// not silently pick the first match. The caller revalidates the real path.
func findWorkspaceFilename(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", workspaceFileAccessError(err)
	}
	visited := 0
	match := ""
	err = filepath.WalkDir(resolvedRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return desktopbridge.ErrWorkspaceFileUnavailable
		}
		if path == resolvedRoot {
			return nil
		}
		visited++
		if visited > workspaceReferenceWalkLimit {
			return desktopbridge.ErrWorkspaceFileUnavailable
		}
		rel, err := filepath.Rel(resolvedRoot, path)
		if err != nil {
			return desktopbridge.ErrWorkspaceFileUnavailable
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if fileref.SkipEntry(rel, entry.Name(), true) || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			// Virtual environments can have arbitrary names (e.g. czsc_env).
			// Their dependencies must not exhaust the citation search budget.
			if marker, err := os.Lstat(filepath.Join(path, "pyvenv.cfg")); err == nil && marker.Mode().IsRegular() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != name || fileref.SkipEntry(rel, entry.Name(), false) || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return desktopbridge.ErrWorkspaceFileUnavailable
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if match != "" {
			return desktopbridge.ErrWorkspaceFileAmbiguous
		}
		match = rel
		return nil
	})
	if err != nil {
		return "", err
	}
	if match == "" {
		return "", desktopbridge.ErrWorkspaceFileNotFound
	}
	return match, nil
}

func workspaceFileAccessError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return desktopbridge.ErrWorkspaceFileNotFound
	}
	if errors.Is(err, os.ErrPermission) {
		return desktopbridge.ErrWorkspaceFileUnavailable
	}
	return err
}
