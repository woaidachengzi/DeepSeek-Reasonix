// Package sessionarchive keeps reversible desktop visibility state separate
// from transcripts and the versioned identity database. Old packages can still
// read every session when rolling back; no artifact is moved or removed.
package sessionarchive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"reasonix/internal/desktopbridge/sessionpath"
)

const maxBytes = 4 << 20
const MaxEntries = 10000

type Entry struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	ArchivedAtMS  int64  `json:"archivedAtMs"`
}

type document struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// The host holds the profile's exclusive lifetime lock. This mutex also
// serializes bridge requests against factory admission within that process.
var mu sync.Mutex

func read(path string) ([]Entry, error) {
	parent, err := os.Lstat(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid session archive directory")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("invalid session archive file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var doc document
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes || json.Unmarshal(data, &doc) != nil || doc.Version != 1 || len(doc.Entries) > MaxEntries {
		return nil, fmt.Errorf("invalid session archive document")
	}
	seen := make(map[string]bool, len(doc.Entries))
	for _, entry := range doc.Entries {
		if !sessionpath.ValidID(entry.SessionID) || seen[entry.SessionID] || entry.ArchivedAtMS <= 0 || len(entry.Title) > 1024 || len(entry.WorkspaceRoot) > 4096 {
			return nil, fmt.Errorf("invalid session archive entry")
		}
		seen[entry.SessionID] = true
	}
	if doc.Entries == nil {
		doc.Entries = []Entry{}
	}
	return doc.Entries, nil
}

func List(path string) ([]Entry, error) {
	mu.Lock()
	defer mu.Unlock()
	return read(path)
}

func Contains(path, id string) (bool, error) {
	entries, err := List(path)
	for _, entry := range entries {
		if entry.SessionID == id {
			return true, err
		}
	}
	return false, err
}

// Set commits before publishing the new list. A refusal leaves the previous
// state intact; retries preserve the original archive date.
func Set(path string, entry Entry, archived bool) ([]Entry, error) {
	mu.Lock()
	defer mu.Unlock()
	if !sessionpath.ValidID(entry.SessionID) {
		return nil, fmt.Errorf("invalid session archive identifier")
	}
	entries, err := read(path)
	if err != nil {
		return nil, err
	}
	index := -1
	for i, item := range entries {
		if item.SessionID == entry.SessionID {
			index = i
			break
		}
	}
	if archived && index >= 0 || !archived && index < 0 {
		return entries, nil
	}
	if archived {
		if len(entries) >= MaxEntries {
			return nil, fmt.Errorf("session archive limit reached")
		}
		entry.ArchivedAtMS = time.Now().UnixMilli()
		entries = append(entries, entry)
	} else {
		entries = append(entries[:index], entries[index+1:]...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ArchivedAtMS > entries[j].ArchivedAtMS })
	data, err := json.Marshal(document{Version: 1, Entries: entries})
	if err != nil || len(data) > maxBytes {
		return nil, fmt.Errorf("session archive is too large")
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid session archive directory")
	}
	f, err := os.CreateTemp(dir, ".session-archives-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return nil, err
	}
	return entries, nil
}
