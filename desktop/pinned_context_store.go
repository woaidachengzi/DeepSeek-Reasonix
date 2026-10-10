package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/fileutil"
	"reasonix/internal/pinnedcontext"
	"reasonix/internal/store"
)

const (
	pinnedContextSchemaVersion = 1
	maxPinnedContextStateBytes = 64 * 1024
)

type pinnedContextState = pinnedcontext.State

func emptyPinnedContextState(path string) pinnedContextState { return pinnedcontext.EmptyState(path) }
func normalizePinnedContextFiles(files []string) ([]string, error) {
	return pinnedcontext.NormalizeFiles(files)
}
func loadPinnedContextState(path string) (pinnedContextState, error) {
	return pinnedcontext.LoadState(path)
}

func savePinnedContextState(sessionPath string, files []string) error {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return fmt.Errorf("session is not ready")
	}
	normalized, err := normalizePinnedContextFiles(files)
	if err != nil {
		return err
	}
	state := emptyPinnedContextState(sessionPath)
	state.Files = normalized
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if len(raw) > maxPinnedContextStateBytes {
		return fmt.Errorf("pinned context state exceeds %d bytes", maxPinnedContextStateBytes)
	}
	return fileutil.AtomicWriteFileStrict(store.SessionPinnedContext(sessionPath), raw, 0o600)
}

func loadOrMigratePinnedContextState(sessionPath string, legacy []string) (pinnedContextState, error) {
	if strings.TrimSpace(sessionPath) == "" {
		state := emptyPinnedContextState("")
		files, err := normalizePinnedContextFiles(legacy)
		state.Files = files
		return state, err
	}
	state, err := loadPinnedContextState(sessionPath)
	if err != nil || len(state.Files) > 0 || len(legacy) == 0 {
		return state, err
	}
	if _, statErr := os.Stat(store.SessionPinnedContext(sessionPath)); statErr == nil {
		return state, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return state, statErr
	}
	files, err := normalizePinnedContextFiles(legacy)
	if err != nil {
		return state, err
	}
	if err := savePinnedContextState(sessionPath, files); err != nil {
		return state, err
	}
	state.Files = files
	return state, nil
}

func copyPinnedContextState(sourcePath, targetPath string) error {
	if strings.TrimSpace(sourcePath) == "" || strings.TrimSpace(targetPath) == "" {
		return nil
	}
	if _, err := os.Stat(store.SessionPinnedContext(sourcePath)); errors.Is(err, os.ErrNotExist) {
		return savePinnedContextState(targetPath, []string{})
	} else if err != nil {
		return err
	}
	state, err := loadPinnedContextState(sourcePath)
	if err != nil {
		return err
	}
	return savePinnedContextState(targetPath, state.Files)
}

func loadPinnedContextStateOrEmpty(sessionPath, logMessage string) pinnedContextState {
	state, err := loadPinnedContextState(sessionPath)
	if err == nil {
		return state
	}
	slog.Warn(logMessage, "session", agent.BranchID(sessionPath), "err", err)
	return emptyPinnedContextState(sessionPath)
}

func prepareStartupPinnedContext(tab *WorkspaceTab, startupPath, persistedPath string) {
	if startupPath != "" {
		migratePendingLegacyPinnedFiles(tab, startupPath)
		if len(tab.pendingLegacyPinnedFilesForPersistence()) > 0 {
			return
		}
		state := loadPinnedContextStateOrEmpty(startupPath, "desktop: load startup pinned context")
		tab.setPinnedFiles(state.Files)
	} else if strings.TrimSpace(persistedPath) != "" {
		// A rejected persisted path must not seed its replacement. A pathless
		// legacy entry keeps its cache until the one-time migration runs.
		tab.setPinnedFiles(nil)
	}
}

func restoreTabPinnedContext(tab *WorkspaceTab, legacy []string) {
	state, err := loadOrMigratePinnedContextState(tab.SessionPath, legacy)
	if err != nil {
		tab.retainLegacyPinnedFiles(legacy)
		slog.Warn("desktop: restore pinned context", "err", err)
		return
	}
	if strings.TrimSpace(tab.SessionPath) == "" && len(legacy) > 0 {
		tab.setPinnedFilesState(state.Files, legacy)
		return
	}
	tab.setPinnedFiles(state.Files)
}

func migratePendingLegacyPinnedFiles(tab *WorkspaceTab, sessionPath string) {
	legacy := tab.pendingLegacyPinnedFilesForPersistence()
	if len(legacy) == 0 || strings.TrimSpace(sessionPath) == "" {
		return
	}
	sidecar := store.SessionPinnedContext(sessionPath)
	if _, err := os.Stat(sidecar); err == nil {
		if _, loadErr := loadPinnedContextState(sessionPath); loadErr == nil {
			tab.clearPendingLegacyPinnedFiles()
		}
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err := savePinnedContextState(sessionPath, legacy); err != nil {
		slog.Warn("desktop: migrate pending legacy pinned context", "err", err)
		return
	}
	tab.clearPendingLegacyPinnedFiles()
}

func pinnedContextStateForSessionBinding(tab *WorkspaceTab, sessionPath string) (pinnedContextState, bool) {
	pendingLegacy := tab.pendingLegacyPinnedFilesForPersistence()
	_, sidecarErr := os.Stat(store.SessionPinnedContext(sessionPath))
	state, err := loadPinnedContextState(sessionPath)
	if err != nil {
		slog.Warn("desktop: load session pinned context", "session", agent.BranchID(sessionPath), "err", err)
	}
	preserveLegacy := len(pendingLegacy) > 0 && (errors.Is(sidecarErr, os.ErrNotExist) || err != nil)
	return state, preserveLegacy
}

func applyPinnedContextSessionBinding(tab *WorkspaceTab, state pinnedContextState, preserveLegacy bool) {
	if !preserveLegacy {
		tab.setPinnedFiles(state.Files)
	}
}
