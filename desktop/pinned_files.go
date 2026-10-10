package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/pinnedcontext"
)

const (
	maxPinnedFileCount   = agent.MaxPinnedContextFiles
	maxPinnedFileSize    = agent.MaxPinnedContextFileBytes
	maxPinnedContextSize = agent.MaxPinnedContextRevisionBytes
)

var (
	errPinnedNotRegular       = pinnedcontext.ErrNotRegular
	errPinnedFileTooLarge     = pinnedcontext.ErrFileTooLarge
	pinnedFileReadHookForTest atomic.Pointer[func()]
)

// Keep the Wails binding's public Go type identity unchanged.
type PinnedFileInfo struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	TokenEstimate int    `json:"tokenEstimate"`
	Error         string `json:"error,omitempty"`
}
type pinnedContextBuild struct {
	Snapshot agent.PinnedContextSnapshot
	Infos    []PinnedFileInfo
}

func pinnedReadHook() {
	if hook := pinnedFileReadHookForTest.Load(); hook != nil {
		(*hook)()
	}
}
func normalizePinnedRelPath(path string) (string, error) { return pinnedcontext.NormalizePath(path) }
func readPinnedWorkspaceFile(root, path string) (string, []byte, int64, error) {
	return pinnedcontext.ReadFile(root, path, pinnedReadHook)
}
func buildPinnedContext(root string, files []string) pinnedContextBuild {
	build := pinnedcontext.Build(root, files, pinnedReadHook)
	infos := make([]PinnedFileInfo, len(build.Infos))
	for i, info := range build.Infos {
		infos[i] = PinnedFileInfo(info)
	}
	return pinnedContextBuild{Snapshot: build.Snapshot, Infos: infos}
}
func pinnedContextIssueReason(err error) agent.PinnedContextIssueReason {
	return pinnedcontext.IssueReason(err)
}
func pinnedContextLoader(root string) control.PinnedContextLoader {
	return pinnedcontext.Loader(root, pinnedReadHook)
}

func pinnedInfoForPath(infos []PinnedFileInfo, path string) (PinnedFileInfo, bool) {
	for _, info := range infos {
		if info.Path == path {
			return info, true
		}
	}
	return PinnedFileInfo{}, false
}

func (t *WorkspaceTab) setPinnedFiles(files []string) {
	t.setPinnedFilesState(files, nil)
}

func (t *WorkspaceTab) setPinnedFilesState(files, pendingLegacy []string) {
	if t == nil {
		return
	}
	t.pinnedFilesMu.Lock()
	t.PinnedFiles = append([]string(nil), files...)
	t.pendingLegacyPinnedFiles = append([]string(nil), pendingLegacy...)
	t.pinnedFilesMu.Unlock()
}

func (t *WorkspaceTab) pinnedFilesState() ([]string, []string) {
	if t == nil {
		return []string{}, []string{}
	}
	t.pinnedFilesMu.RLock()
	defer t.pinnedFilesMu.RUnlock()
	return append([]string{}, t.PinnedFiles...), append([]string{}, t.pendingLegacyPinnedFiles...)
}

func (t *WorkspaceTab) retainLegacyPinnedFiles(files []string) {
	normalized, err := normalizePinnedContextFiles(files)
	if err != nil {
		normalized = []string{}
	}
	t.setPinnedFilesState(normalized, files)
}

func (t *WorkspaceTab) pendingLegacyPinnedFilesForPersistence() []string {
	_, pending := t.pinnedFilesState()
	return pending
}

func (t *WorkspaceTab) clearPendingLegacyPinnedFiles() {
	if t == nil {
		return
	}
	t.pinnedFilesMu.Lock()
	t.pendingLegacyPinnedFiles = nil
	t.pinnedFilesMu.Unlock()
}

// PinFile updates the tab-local cache. Durable desktop mutations go through
// PinFileForTab so the session sidecar and controller change atomically.
func (t *WorkspaceTab) PinFile(relPath string) (PinnedFileInfo, error) {
	if t == nil {
		return PinnedFileInfo{}, errors.New("tab is nil")
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	files := t.GetPinnedFiles()
	if slices.Contains(files, clean) {
		build := buildPinnedContext(t.WorkspaceRoot, files)
		info, _ := pinnedInfoForPath(build.Infos, clean)
		return info, nil
	}
	if len(files) >= maxPinnedFileCount {
		return PinnedFileInfo{}, fmt.Errorf("at most %d files can be pinned", maxPinnedFileCount)
	}
	candidate := append(files, clean)
	candidate, err = normalizePinnedContextFiles(candidate)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	build := buildPinnedContext(t.WorkspaceRoot, candidate)
	info, ok := pinnedInfoForPath(build.Infos, clean)
	if !ok {
		return PinnedFileInfo{}, errors.New("pinned file could not be inspected")
	}
	if info.Error != "" {
		return PinnedFileInfo{}, errors.New(info.Error)
	}
	t.setPinnedFiles(candidate)
	return info, nil
}

func (t *WorkspaceTab) UnpinFile(relPath string) error {
	if t == nil {
		return errors.New("tab is nil")
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return err
	}
	files := t.GetPinnedFiles()
	next := make([]string, 0, len(files))
	for _, path := range files {
		if path != clean {
			next = append(next, path)
		}
	}
	t.setPinnedFiles(next)
	return nil
}

func (t *WorkspaceTab) GetPinnedFiles() []string {
	if t == nil {
		return []string{}
	}
	t.pinnedFilesMu.RLock()
	defer t.pinnedFilesMu.RUnlock()
	return append([]string{}, t.PinnedFiles...)
}

func (t *WorkspaceTab) GetPinnedFilesInfo() []PinnedFileInfo {
	if t == nil {
		return []PinnedFileInfo{}
	}
	return buildPinnedContext(t.WorkspaceRoot, t.GetPinnedFiles()).Infos
}

func estimateTokensFromBytes(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	tok := int(bytes / 4)
	if tok == 0 {
		return 1
	}
	return tok
}

func (a *App) mutatePinnedFiles(tabID, relPath string, pin bool) (PinnedFileInfo, string, error) {
	unlockRuntime := a.lockRuntimeMutation("pinned context")
	defer unlockRuntime()
	tab := a.tabByID(tabID)
	if tab == nil {
		return PinnedFileInfo{}, "", errors.New("tab not found")
	}
	tab.turnStartMu.Lock()
	defer tab.turnStartMu.Unlock()

	a.mu.RLock()
	if a.tabs[tab.ID] != tab || tab.removed {
		a.mu.RUnlock()
		return PinnedFileInfo{}, "", errors.New("tab changed while updating pinned context")
	}
	root := tab.WorkspaceRoot
	ctrl := tab.Ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return PinnedFileInfo{}, "", a.workspaceNotReadyErr(tab)
	}
	if ctrl.RuntimeStatus().Running {
		return PinnedFileInfo{}, "", control.ErrTurnRunning
	}
	sessionPath := ctrl.SessionPath()
	state, err := loadPinnedContextState(sessionPath)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	oldFiles := append([]string(nil), state.Files...)
	candidate := append([]string(nil), oldFiles...)
	alreadyPinned := false
	if pin {
		alreadyPinned = slices.Contains(candidate, clean)
		if !alreadyPinned && len(candidate) >= maxPinnedFileCount {
			return PinnedFileInfo{}, "", fmt.Errorf("at most %d files can be pinned", maxPinnedFileCount)
		}
		if !alreadyPinned {
			candidate = append(candidate, clean)
		}
	} else {
		next := make([]string, 0, len(candidate))
		for _, path := range candidate {
			if path != clean {
				next = append(next, path)
			}
		}
		candidate = next
	}
	candidate, err = normalizePinnedContextFiles(candidate)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	build := buildPinnedContext(root, candidate)
	info := PinnedFileInfo{Path: clean}
	if pin {
		var ok bool
		info, ok = pinnedInfoForPath(build.Infos, clean)
		if !ok {
			return PinnedFileInfo{}, "", errors.New("pinned file could not be inspected")
		}
		if info.Error != "" && !alreadyPinned {
			return PinnedFileInfo{}, "", errors.New(info.Error)
		}
	}
	if candidateChanged := strings.Join(oldFiles, "\x00") != strings.Join(candidate, "\x00"); candidateChanged {
		if err := savePinnedContextState(sessionPath, candidate); err != nil {
			return PinnedFileInfo{}, "", err
		}
	}
	tab.setPinnedFiles(candidate)
	return info, tab.ID, nil
}

func (a *App) PinFileForTab(tabID, relPath string) (PinnedFileInfo, error) {
	info, changedTabID, err := a.mutatePinnedFiles(tabID, relPath, true)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	if changedTabID != "" {
		a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: changedTabID, Meta: a.MetaForTab(changedTabID)})
	}
	return info, nil
}

func (a *App) UnpinFileForTab(tabID, relPath string) error {
	_, changedTabID, err := a.mutatePinnedFiles(tabID, relPath, false)
	if err != nil {
		return err
	}
	if changedTabID != "" {
		a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: changedTabID, Meta: a.MetaForTab(changedTabID)})
	}
	return nil
}

func (a *App) GetPinnedFilesForTab(tabID string) ([]PinnedFileInfo, error) {
	tab := a.tabByID(tabID)
	if tab == nil {
		return []PinnedFileInfo{}, errors.New("tab not found")
	}
	a.mu.RLock()
	if a.tabs[tab.ID] != tab || tab.removed {
		a.mu.RUnlock()
		return []PinnedFileInfo{}, errors.New("tab not found")
	}
	root := tab.WorkspaceRoot
	ctrl := tab.Ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return []PinnedFileInfo{}, a.workspaceNotReadyErr(tab)
	}
	state, err := loadPinnedContextState(ctrl.SessionPath())
	if err != nil {
		return []PinnedFileInfo{}, err
	}
	infos := buildPinnedContext(root, state.Files).Infos
	if infos == nil {
		infos = []PinnedFileInfo{}
	}
	return infos, nil
}
