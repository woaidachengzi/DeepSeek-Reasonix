package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	fileencoding "reasonix/internal/fileutil/encoding"
	"reasonix/internal/memory"
)

const maxPreviewMemoryDocBytes = 2 << 20

var previewMemorySettingsMu sync.Mutex
var errPreviewMemoryChanged = errors.New("memory changed on disk; reload before saving")

type previewMemoryDoc struct {
	Path     string `json:"path"`
	Scope    string `json:"scope"`
	Body     string `json:"body"`
	Revision string `json:"revision"`
}

type previewMemoryFact struct {
	ID          string `json:"id"`
	Revision    int    `json:"revision"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Scope       string `json:"scope"`
	Body        string `json:"body"`
	Freshness   string `json:"freshness"`
}

type previewMemoryArchive struct {
	previewMemoryFact
	Path       string `json:"path"`
	ArchivedAt string `json:"archivedAt"`
}

type previewMemorySettingsView struct {
	ProtocolVersion int                            `json:"protocolVersion"`
	WorkspaceRoot   string                         `json:"workspaceRoot"`
	StoreDir        string                         `json:"storeDir"`
	GlobalStoreDir  string                         `json:"globalStoreDir"`
	Docs            []previewMemoryDoc             `json:"docs"`
	Facts           []previewMemoryFact            `json:"facts"`
	Archives        []previewMemoryArchive         `json:"archives"`
	Revisions       []previewMemoryFact            `json:"revisions"`
	Diagnostics     []string                       `json:"diagnostics"`
	LastRecall      desktopbridge.MemoryRecallView `json:"lastRecall"`
}

type previewMemorySettingsChange struct {
	WorkspaceRoot   string `json:"workspaceRoot"`
	Action          string `json:"action"` // save_doc | quick_add | archive | restore | load_revisions | restore_revision
	Path            string `json:"path"`
	Revision        string `json:"revision"`
	Body            string `json:"body"`
	Scope           string `json:"scope"`
	FactID          string `json:"factId"`
	FactRevision    int    `json:"factRevision"`
	HistoryRevision int    `json:"historyRevision"`
	Name            string `json:"name"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Type            string `json:"type"`
}

func loadMemorySetForPreview(workspaceRoot string) (*memory.Set, string, error) {
	root, err := normalizeSkillsWorkspace(workspaceRoot)
	if err != nil {
		return nil, "", err
	}
	if root == "" {
		return nil, "", fmt.Errorf("a workspace is required for memory settings")
	}
	userDir := appconfig.MemoryUserDir()
	if userDir == "" {
		return nil, "", fmt.Errorf("Preview memory directory is unavailable")
	}
	return memory.Load(memory.Options{CWD: root, UserDir: userDir}), root, nil
}

func readPreviewMemoryDoc(path string) (string, string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "missing", nil
	}
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxPreviewMemoryDocBytes {
		return "", "", fmt.Errorf("memory document is not a supported regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if len(raw) > maxPreviewMemoryDocBytes {
		return "", "", fmt.Errorf("memory document is too large")
	}
	hash := sha256.Sum256(raw)
	return string(fileencoding.DecodeToUTF8(raw)), hex.EncodeToString(hash[:]), nil
}

func previewMemoryFactFromCore(f memory.Memory) previewMemoryFact {
	return previewMemoryFact{
		ID: f.ID, Revision: f.Revision, CreatedAt: memoryTimestamp(f.CreatedAt), UpdatedAt: memoryTimestamp(f.UpdatedAt), Name: f.Name, Title: f.Title,
		Description: f.Description, Type: string(f.Type), Scope: string(f.Scope),
		Body: f.Body, Freshness: memory.FreshnessFor(f, time.Now().UTC()),
	}
}

func memoryTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func loadPreviewMemorySettings(workspaceRoot string) (previewMemorySettingsView, error) {
	set, root, err := loadMemorySetForPreview(workspaceRoot)
	if err != nil {
		return previewMemorySettingsView{}, err
	}
	view := previewMemorySettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion, WorkspaceRoot: root,
		StoreDir: set.Store.Dir, GlobalStoreDir: set.Store.GlobalDir,
		Docs: []previewMemoryDoc{}, Facts: []previewMemoryFact{},
		Archives: []previewMemoryArchive{}, Revisions: []previewMemoryFact{}, Diagnostics: []string{},
		LastRecall: desktopbridge.MemoryRecallView{Hits: []desktopbridge.MemoryRecallHit{}},
	}
	seen := map[string]bool{}
	addDoc := func(path, scope string) error {
		path = filepath.Clean(path)
		if seen[path] {
			return nil
		}
		body, revision, err := readPreviewMemoryDoc(path)
		if err != nil {
			return err
		}
		view.Docs = append(view.Docs, previewMemoryDoc{Path: path, Scope: scope, Body: body, Revision: revision})
		seen[path] = true
		return nil
	}
	for _, doc := range set.Docs {
		if err := addDoc(doc.Path, string(doc.Scope)); err != nil {
			return previewMemorySettingsView{}, err
		}
	}
	for _, scope := range []memory.Scope{memory.ScopeUser, memory.ScopeProject, memory.ScopeLocal} {
		if path := set.DocPath(scope); path != "" {
			if err := addDoc(path, string(scope)); err != nil {
				return previewMemorySettingsView{}, err
			}
		}
	}
	for _, diagnostic := range set.InstructionDiagnostics {
		view.Diagnostics = append(view.Diagnostics, diagnostic.Message)
	}
	for _, fact := range set.Store.ListAll() {
		view.Facts = append(view.Facts, previewMemoryFactFromCore(fact))
	}
	for _, archived := range set.Store.ListArchived() {
		stamp := ""
		if !archived.ArchivedAt.IsZero() {
			stamp = archived.ArchivedAt.UTC().Format(time.RFC3339)
		}
		view.Archives = append(view.Archives, previewMemoryArchive{
			previewMemoryFact: previewMemoryFactFromCore(archived.Memory),
			Path:              archived.Path, ArchivedAt: stamp,
		})
	}
	return view, nil
}

func persistPreviewMemorySettings(change previewMemorySettingsChange) (previewMemorySettingsView, error) {
	if len(change.Body) > maxPreviewMemoryDocBytes || len(change.Path) > 4096 || len(change.FactID) > 256 || len(change.Name) > 256 || len(change.Title) > 1024 || len(change.Description) > 4096 {
		return previewMemorySettingsView{}, fmt.Errorf("memory setting is too large")
	}
	set, root, err := loadMemorySetForPreview(change.WorkspaceRoot)
	if err != nil {
		return previewMemorySettingsView{}, err
	}
	previewMemorySettingsMu.Lock()
	defer previewMemorySettingsMu.Unlock()
	switch change.Action {
	case "save_doc":
		path := filepath.Clean(change.Path)
		allowed := false
		for _, doc := range set.Docs {
			if filepath.Clean(doc.Path) == path {
				allowed = true
				break
			}
		}
		for _, scope := range []memory.Scope{memory.ScopeUser, memory.ScopeProject, memory.ScopeLocal} {
			if target := set.DocPath(scope); target != "" && filepath.Clean(target) == path {
				allowed = true
				break
			}
		}
		if !allowed {
			return previewMemorySettingsView{}, fmt.Errorf("memory document is outside the current workspace")
		}
		_, revision, err := readPreviewMemoryDoc(path)
		if err != nil {
			return previewMemorySettingsView{}, err
		}
		if revision != change.Revision {
			return previewMemorySettingsView{}, errPreviewMemoryChanged
		}
		if _, err := set.WriteDoc(path, change.Body); err != nil {
			return previewMemorySettingsView{}, err
		}
	case "quick_add":
		var scope memory.Scope
		switch change.Scope {
		case "user":
			scope = memory.ScopeUser
		case "project":
			scope = memory.ScopeProject
		case "local":
			scope = memory.ScopeLocal
		default:
			return previewMemorySettingsView{}, fmt.Errorf("invalid memory scope")
		}
		if strings.TrimSpace(change.Body) == "" || len(change.Body) > 4096 {
			return previewMemorySettingsView{}, fmt.Errorf("note is empty or too long")
		}
		path := set.DocPath(scope)
		if path == "" {
			return previewMemorySettingsView{}, fmt.Errorf("memory scope is unavailable")
		}
		if err := memory.AppendDoc(path, change.Body); err != nil {
			return previewMemorySettingsView{}, err
		}
	case "archive":
		if strings.TrimSpace(change.FactID) == "" || change.FactRevision < 1 {
			return previewMemorySettingsView{}, fmt.Errorf("memory fact identity is required")
		}
		fact, found := set.Store.Read(change.FactID)
		if !found || fact.Revision != change.FactRevision {
			return previewMemorySettingsView{}, errPreviewMemoryChanged
		}
		if _, err := set.Store.Archive(change.FactID); err != nil {
			return previewMemorySettingsView{}, err
		}
	case "save_fact":
		if strings.TrimSpace(change.FactID) == "" || change.FactRevision < 1 {
			return previewMemorySettingsView{}, fmt.Errorf("memory fact identity is required")
		}
		fact, found := set.Store.Read(change.FactID)
		if !found || fact.Revision != change.FactRevision {
			return previewMemorySettingsView{}, errPreviewMemoryChanged
		}
		name := strings.TrimSpace(change.Name)
		if name == "" || strings.TrimSpace(change.Description) == "" || strings.TrimSpace(change.Body) == "" {
			return previewMemorySettingsView{}, fmt.Errorf("memory name, description, and body are required")
		}
		_, err := set.Store.SaveWithOptions(memory.Memory{
			ID: fact.ID, Name: name, Title: strings.TrimSpace(change.Title),
			Description: strings.TrimSpace(change.Description), Type: memory.NormalizeType(change.Type),
			Scope: fact.Scope, Body: strings.TrimSpace(change.Body),
		}, memory.SaveOptions{ExpectedRevision: fact.Revision, RequireExpectedRevision: true})
		if err != nil {
			if strings.Contains(err.Error(), "revision conflict") {
				return previewMemorySettingsView{}, errPreviewMemoryChanged
			}
			return previewMemorySettingsView{}, err
		}
	case "load_revisions":
		if strings.TrimSpace(change.FactID) == "" {
			return previewMemorySettingsView{}, fmt.Errorf("memory fact identity is required")
		}
		fact, found := set.Store.Read(change.FactID)
		if !found || change.FactRevision > 0 && fact.Revision != change.FactRevision {
			return previewMemorySettingsView{}, errPreviewMemoryChanged
		}
		next, err := loadPreviewMemorySettings(root)
		if err != nil {
			return previewMemorySettingsView{}, err
		}
		for _, revision := range set.Store.Revisions(fact.ID) {
			next.Revisions = append(next.Revisions, previewMemoryFactFromCore(revision))
		}
		return next, nil
	case "restore_revision":
		if strings.TrimSpace(change.FactID) == "" || change.FactRevision < 1 || change.HistoryRevision < 1 {
			return previewMemorySettingsView{}, fmt.Errorf("memory revision identity is required")
		}
		fact, found := set.Store.Read(change.FactID)
		if !found || fact.Revision != change.FactRevision {
			return previewMemorySettingsView{}, errPreviewMemoryChanged
		}
		if _, err := set.Store.Restore(fact.ID, change.HistoryRevision); err != nil {
			return previewMemorySettingsView{}, err
		}
	case "restore":
		if strings.TrimSpace(change.Path) == "" {
			return previewMemorySettingsView{}, fmt.Errorf("archived memory path is required")
		}
		if _, err := set.Store.RestoreArchived(change.Path); err != nil {
			return previewMemorySettingsView{}, err
		}
	default:
		return previewMemorySettingsView{}, fmt.Errorf("invalid memory settings action")
	}
	return loadPreviewMemorySettings(root)
}
