package sessionidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/store"
)

// InventorySource says where a candidate row was found. Rows are candidates:
// listing one never claims it.
type InventorySource string

const (
	InventoryFromIdentity  InventorySource = "identity"
	InventoryFromWorkbench InventorySource = "workbench"
	InventoryFromScan      InventorySource = "scan"
)

// InventoryClaim explains whether a row can be registered, and why not.
type InventoryClaim string

const (
	// Claimable rows may be registered by a later import.
	Claimable InventoryClaim = "claimable"
	// ClaimRegistered means the identity store already owns this ID.
	ClaimRegistered InventoryClaim = "registered"
	// ClaimUnclaimed is a file no catalog entry explains. Registering it would
	// assert an identity the host never recorded, so it stays a candidate.
	ClaimUnclaimed InventoryClaim = "unclaimed_file"
	// ClaimMissingFile is a catalog entry whose transcript is absent; it must be
	// reported, never reopened as an empty session with the same ID.
	ClaimMissingFile InventoryClaim = "missing_file"
	// ClaimPathConflict means different IDs want the same lexical or physical
	// transcript path.
	ClaimPathConflict InventoryClaim = "path_conflict"
	// ClaimPathChanged means a registered ID now maps to a different path.
	ClaimPathChanged InventoryClaim = "path_changed"
	// ClaimInvalidFile is a transcript name that yields no safe session ID.
	ClaimInvalidFile InventoryClaim = "invalid_file"
	// ClaimUnreadable covers a transcript that exists but cannot be inspected.
	ClaimUnreadable InventoryClaim = "unreadable_file"
)

// InventoryEntry is one row of the read-only session inventory.
type InventoryEntry struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	Registered    bool   `json:"registered"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	Title         string `json:"title,omitempty"`
	// TitleSource is needed by the bridge's local sidecar audit but is not
	// part of the inventory wire format or import-review manifest.
	TitleSource TitleSource     `json:"-"`
	Source      InventorySource `json:"source"`
	Claim       InventoryClaim  `json:"claim"`
	Detail      string          `json:"detail,omitempty"`
}

// InventoryReport is the whole listing plus the summary a reviewer needs.
type InventoryReport struct {
	SessionDir string           `json:"sessionDir"`
	Entries    []InventoryEntry `json:"entries"`
	// Unclaimed lists scanned transcripts that map to no catalog entry.
	Unclaimed []string `json:"unclaimed"`
	// Errors carries per-row problems; an inventory never fails because one
	// transcript is odd.
	Errors []string `json:"errors"`
	// Registered rows came from the same validated identity read as Entries.
	// They are kept only in memory so the bridge can build a bounded directory
	// snapshot without resolving every transcript path a second time.
	registered []Record
}

// VisibleSnapshot builds the host's structural directory snapshot from the
// identity rows already read for this physical inventory. It never substitutes
// for the inventory or the subsequent snapshot-bound page read.
func (report InventoryReport) VisibleSnapshot(limit int) (Page, error) {
	if limit < 1 || limit > MaxVisibleSnapshotSize {
		return Page{}, fmt.Errorf("session directory snapshot limit must be between 1 and %d", MaxVisibleSnapshotSize)
	}
	page := Page{Records: make([]Record, 0, min(len(report.registered), limit))}
	hasher := newVisibleSnapshotHasher()
	for _, record := range report.registered {
		if record.State == StateDeleting || record.State == StateDeleted {
			continue
		}
		if len(page.Records) == limit {
			return Page{}, fmt.Errorf("session directory snapshot exceeds %d entries", limit)
		}
		if record.relativePath == "" {
			return Page{}, errors.New("session inventory omitted an identity relative path")
		}
		hasher.add(record.ID, record.relativePath, record.WorkspaceRoot, int64(record.Position), record.State)
		page.Records = append(page.Records, record)
	}
	page.Total = len(page.Records)
	page.SnapshotID = hasher.sum()
	return page, nil
}

// Inventory lists what the session directory holds and what the identity store
// already knows, without writing anything: it does not create the database,
// register a candidate, or touch a transcript.
//
// identities may be nil, which is how a profile with no identity database yet is
// listed — the common case before Phase 1 imports anything.
//
// catalogPath may be empty. Every row states where it came from and whether it
// could be claimed, so a reviewer can compare the list against the disk before
// anything is registered.
func Inventory(ctx context.Context, identities *Store, sessionDir, catalogPath string) (InventoryReport, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return InventoryReport{}, errors.New("session directory is empty")
	}
	dir, err := filepath.Abs(sessionDir)
	if err != nil {
		return InventoryReport{}, err
	}
	report := InventoryReport{
		SessionDir: dir,
		Entries:    make([]InventoryEntry, 0),
		Unclaimed:  make([]string, 0),
		Errors:     make([]string, 0),
	}

	var registered []Record
	if identities != nil {
		var err error
		registered, err = identities.List(ctx)
		if err != nil {
			return InventoryReport{}, err
		}
	}
	report.registered = registered
	byID := make(map[string]Record, len(registered))
	for _, record := range registered {
		byID[record.ID] = record
	}
	for _, record := range registered {
		entry := InventoryEntry{
			ID:            record.ID,
			Path:          record.Path,
			Registered:    true,
			WorkspaceRoot: record.WorkspaceRoot,
			Title:         record.Title,
			TitleSource:   record.TitleSource,
			Source:        InventoryFromIdentity,
			Claim:         ClaimRegistered,
		}
		entry.Exists, entry.Detail = inspectTranscript(record.Path)
		report.Entries = append(report.Entries, entry)
	}

	claimedPaths := make(map[string]string, len(registered))
	for _, record := range registered {
		claimedPaths[filepath.Clean(record.Path)] = record.ID
	}

	catalogEntries, err := readCatalog(catalogPath)
	if err != nil {
		return InventoryReport{}, err
	}
	catalogIDCount := make(map[string]int, len(catalogEntries))
	for _, entry := range catalogEntries {
		catalogIDCount[entry.SessionID]++
	}
	for position, entry := range catalogEntries {
		path, err := sessionpath.TranscriptPath(dir, entry.SessionID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("workbench entry %d: %v", position, err))
			continue
		}
		row := InventoryEntry{
			ID:            entry.SessionID,
			Path:          path,
			WorkspaceRoot: strings.TrimSpace(entry.WorkspaceRoot),
			Title:         entry.Title,
			Source:        InventoryFromWorkbench,
		}
		if record, ok := byID[entry.SessionID]; ok {
			row.Registered = true
			row.Title = record.Title
			row.WorkspaceRoot = record.WorkspaceRoot
			row.Claim = ClaimRegistered
			if filepath.Clean(record.Path) != filepath.Clean(path) {
				row.Claim = ClaimPathChanged
				row.Detail = fmt.Sprintf("registered path %s", record.Path)
			}
		}
		exists, detail := inspectTranscript(path)
		row.Exists = exists
		if row.Detail == "" {
			row.Detail = detail
		}
		if row.Claim == "" {
			switch {
			case catalogIDCount[entry.SessionID] > 1:
				row.Claim = ClaimPathConflict
				row.Detail = "duplicate session ID in workbench catalog"
				report.Errors = append(report.Errors, fmt.Sprintf("workbench entry %d: %s", position, row.Detail))
			case detail != "" && (exists || detail != "transcript is absent"):
				row.Claim = ClaimUnreadable
				report.Errors = append(report.Errors, fmt.Sprintf("workbench entry %d: %s", position, detail))
			case !exists:
				row.Claim = ClaimMissingFile
			case claimedPaths[filepath.Clean(path)] != "":
				row.Claim = ClaimPathConflict
				row.Detail = fmt.Sprintf("path already belongs to session %s", claimedPaths[filepath.Clean(path)])
			default:
				row.Claim = Claimable
			}
		}
		report.Entries = append(report.Entries, row)
	}

	// Names the identity store or the catalog already explain. Everything else
	// on disk is an unclaimed candidate.
	knownNames := make(map[string]bool, len(registered)+len(catalogEntries))
	for _, record := range registered {
		if filepath.Clean(filepath.Dir(record.Path)) == dir {
			knownNames[filepath.Base(record.Path)] = true
		}
	}
	for _, entry := range catalogEntries {
		path, err := sessionpath.TranscriptPath(dir, entry.SessionID)
		if err != nil {
			continue
		}
		knownNames[filepath.Base(path)] = true
	}

	scanned, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return InventoryReport{}, fmt.Errorf("read session directory: %w", err)
	}
	for _, item := range scanned {
		if item.IsDir() || !store.IsSessionTranscriptName(item.Name()) {
			continue
		}
		path := filepath.Join(dir, item.Name())
		page := filepath.Clean(path)
		if _, taken := claimedPaths[page]; taken {
			continue
		}
		if knownNames[item.Name()] {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(item.Name(), "tauri-"), ".jsonl")
		row := InventoryEntry{
			ID:     id,
			Path:   path,
			Exists: true,
			Source: InventoryFromScan,
		}
		roundTrip, pathErr := sessionpath.TranscriptPath(dir, id)
		if pathErr != nil || roundTrip != path {
			row.Claim = ClaimInvalidFile
			row.Detail = "file name does not round-trip to a bridge session path"
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", item.Name(), row.Detail))
			report.Entries = append(report.Entries, row)
			continue
		}
		if record, ok := byID[id]; ok && filepath.Clean(record.Path) != filepath.Clean(path) {
			row.Claim = ClaimPathChanged
			row.Detail = fmt.Sprintf("registered path %s", record.Path)
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", item.Name(), row.Detail))
			report.Entries = append(report.Entries, row)
			continue
		}
		if exists, detail := inspectTranscript(path); !exists || detail != "" {
			row.Exists = exists
			row.Claim = ClaimUnreadable
			row.Detail = detail
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", item.Name(), detail))
			report.Entries = append(report.Entries, row)
			continue
		}
		row.Claim = ClaimUnclaimed
		row.Detail = "no catalog entry or identity row explains this file"
		report.Unclaimed = append(report.Unclaimed, path)
		report.Entries = append(report.Entries, row)
	}
	markPhysicalPathConflicts(&report)

	sort.SliceStable(report.Entries, func(i, j int) bool {
		if report.Entries[i].Source != report.Entries[j].Source {
			return report.Entries[i].Source < report.Entries[j].Source
		}
		if report.Entries[i].Path != report.Entries[j].Path {
			return report.Entries[i].Path < report.Entries[j].Path
		}
		return report.Entries[i].ID < report.Entries[j].ID
	})
	sort.Strings(report.Unclaimed)
	return report, nil
}

// markPhysicalPathConflicts annotates pre-existing identities that resolve to
// one physical transcript. Inventory is also the read-only audit path for
// databases written before physical-path uniqueness was enforced.
func markPhysicalPathConflicts(report *InventoryReport) {
	type pathOwner struct {
		id, path, resolved string
		info               os.FileInfo
		entryIndex         int
	}
	owners := make([]pathOwner, 0, len(report.Entries))
	for entryIndex, entry := range report.Entries {
		if entry.ID == "" || entry.Path == "" {
			continue
		}
		resolved, err := resolveIdentityPath(entry.Path)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: resolve transcript path: %v", entry.ID, err))
			continue
		}
		info, err := os.Stat(entry.Path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: inspect transcript identity: %v", entry.ID, err))
			continue
		}
		owners = append(owners, pathOwner{id: entry.ID, path: entry.Path, resolved: resolved, info: info, entryIndex: entryIndex})
	}
	conflicts := make(map[int]map[string]bool)
	markPair := func(i, j int) {
		if owners[i].id == owners[j].id {
			return // Multiple report sources can describe the same identity.
		}
		if conflicts[i] == nil {
			conflicts[i] = make(map[string]bool)
		}
		if conflicts[j] == nil {
			conflicts[j] = make(map[string]bool)
		}
		conflicts[i][owners[j].id] = true
		conflicts[j][owners[i].id] = true
	}
	pathBuckets := make(map[string][]int, len(owners))
	fileBuckets := make(map[physicalFileKey][]int, len(owners))
	var unkeyedFiles []int
	for i, owner := range owners {
		pathKey := foldedResolvedPath(owner.resolved)
		for _, j := range pathBuckets[pathKey] {
			if owner.resolved == owners[j].resolved || sameCaseInsensitivePath(owner.resolved, owners[j].resolved) {
				markPair(i, j)
			}
		}
		pathBuckets[pathKey] = append(pathBuckets[pathKey], i)
		if owner.info == nil {
			continue
		}
		if key, ok := fileKey(owner.info); ok {
			for _, j := range fileBuckets[key] {
				if os.SameFile(owner.info, owners[j].info) {
					markPair(i, j)
				}
			}
			for _, j := range unkeyedFiles {
				if os.SameFile(owner.info, owners[j].info) {
					markPair(i, j)
				}
			}
			fileBuckets[key] = append(fileBuckets[key], i)
		} else {
			// Platforms without a stable file key retain the conservative
			// pairwise SameFile check rather than silently missing hard links.
			for j := 0; j < i; j++ {
				if owners[j].info != nil && os.SameFile(owner.info, owners[j].info) {
					markPair(i, j)
				}
			}
			unkeyedFiles = append(unkeyedFiles, i)
		}
	}
	indices := make([]int, 0, len(conflicts))
	for index := range conflicts {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		otherIDs := conflicts[index]
		others := make([]string, 0, len(otherIDs))
		for id := range otherIDs {
			others = append(others, id)
		}
		sort.Strings(others)
		entry := &report.Entries[owners[index].entryIndex]
		entry.Claim = ClaimPathConflict
		entry.Detail = "physical transcript also belongs to session " + strings.Join(others, ", ")
		report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", entry.ID, entry.Detail))
	}
}

// EqualFold uses Unicode simple-fold cycles, not just lowercase mappings.
// Choosing each rune's smallest cycle member gives equal paths the same
// bucket on macOS/Windows while the final comparison remains authoritative.
func foldedResolvedPath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return path
	}
	var folded strings.Builder
	folded.Grow(len(path))
	for _, original := range path {
		minimum := original
		for next := unicode.SimpleFold(original); next != original; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		folded.WriteRune(minimum)
	}
	return folded.String()
}

// inspectTranscript reports whether a path holds a regular file.
func inspectTranscript(path string) (bool, string) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return false, "transcript is absent"
	case err != nil:
		return false, err.Error()
	case info.Mode()&os.ModeSymlink != 0:
		return true, "transcript is a symlink"
	case !info.Mode().IsRegular():
		return true, "transcript is not a regular file"
	}
	return true, ""
}

func readCatalog(catalogPath string) ([]catalogEntry, error) {
	if strings.TrimSpace(catalogPath) == "" {
		return nil, nil
	}
	file, err := os.Open(catalogPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open workbench catalog: %w", err)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read workbench catalog: %w", err)
	}
	if len(encoded) > 1<<20 {
		return nil, errors.New("workbench catalog is too large")
	}
	var entries []catalogEntry
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return nil, fmt.Errorf("decode workbench catalog: %w", err)
	}
	return entries, nil
}
