package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"reasonix/internal/desktopbridge"
)

const previewDesktopCatalogueLimit = 128

// Entries are host-private observations, never execution grants. Remote paths,
// connections and core epochs must not be formatted directly into IM replies.
// Any later command must recheck this same capture against the authoritative
// owner and use scoped core/Serve admission, not recapture a replacement by ID.
type previewDesktopCatalogueEntry struct {
	handle                     string
	local                      *desktopbridge.OwnedCommandView
	remote                     *remoteControllerConnection
	remotePath, remoteEpoch    string
	running, pending, detached bool
}

type previewDesktopCatalogueKey struct {
	local       desktopbridge.OwnedCommandScope
	remote      *remoteControllerConnection
	path, epoch string
}

func (e previewDesktopCatalogueEntry) key() previewDesktopCatalogueKey {
	if e.local != nil {
		return previewDesktopCatalogueKey{local: e.local.Scope}
	}
	return previewDesktopCatalogueKey{remote: e.remote, path: e.remotePath, epoch: e.remoteEpoch}
}

type previewDesktopCatalogue struct {
	manager *desktopbridge.RuntimeManager
	remotes *previewRemoteSessions
	ctx     context.Context
	cancel  context.CancelFunc
	refresh chan struct{}
	mu      sync.Mutex
	closed  bool
	entries map[string]previewDesktopCatalogueEntry
}

func newPreviewDesktopCatalogue(manager *desktopbridge.RuntimeManager, remotes *previewRemoteSessions) *previewDesktopCatalogue {
	ctx, cancel := context.WithCancel(context.Background())
	return &previewDesktopCatalogue{manager: manager, remotes: remotes, ctx: ctx, cancel: cancel, refresh: make(chan struct{}, 1), entries: make(map[string]previewDesktopCatalogueEntry)}
}

func (c *previewDesktopCatalogue) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	clear(c.entries)
}

// Refresh includes every presently published local owner (the current manager
// has one) and every attached remote Serve, including its detached controllers.
// Saved/external history is never a live owner. Reads never attach/resume/open,
// start Serve, resolve prompts, switch foreground or touch a local model.
// Any incomplete/failed read retires the whole catalogue; no silent partial list.
func (c *previewDesktopCatalogue) Refresh(ctx context.Context) (result []previewDesktopCatalogueEntry, err error) {
	if ctx == nil {
		return nil, errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	defer func() { stop(); cancel() }()
	select {
	case c.refresh <- struct{}{}:
		defer func() { <-c.refresh }()
	case <-bounded.Done():
		return nil, errPreviewDesktopBinding
	}
	defer func() {
		if err != nil {
			c.mu.Lock()
			clear(c.entries)
			c.mu.Unlock()
		}
	}()
	if bounded.Err() != nil || c.ctx.Err() != nil {
		return nil, errPreviewDesktopBinding
	}
	var localScope desktopbridge.OwnedCommandScope
	if c.manager != nil {
		if view, ok := c.manager.CommandSnapshot(); ok {
			localScope = view.Scope
			result = append(result, previewDesktopCatalogueEntry{local: &view, running: view.State.Running, pending: view.State.PendingPrompt})
		}
	}
	connections := c.remoteConnections()
	inspected := len(result)
	for _, connection := range connections {
		snapshot, readErr := connection.client.RuntimeStates(bounded)
		if readErr != nil || len(snapshot.Sessions)+inspected > previewDesktopCatalogueLimit {
			return nil, errPreviewDesktopBinding
		}
		inspected += len(snapshot.Sessions)
		for _, row := range snapshot.Sessions {
			if row.State.Phase == "closed" || row.Ownership != "serve" {
				continue
			}
			if c.remotes.getController(connection.view.ID) != connection {
				return nil, errPreviewDesktopBinding
			}
			state := row.State
			result = append(result, previewDesktopCatalogueEntry{remote: connection, remotePath: row.SessionPath, remoteEpoch: state.RuntimeEpoch, running: state.Running, pending: state.PendingPrompt, detached: !row.Current})
		}
	}
	// Reject a changed connection set, including a newly published connection,
	// rather than returning a misleading "all sessions" cut from mixed owners.
	current := c.remoteConnections()
	if len(current) != len(connections) {
		return nil, errPreviewDesktopBinding
	}
	for i := range current {
		if current[i] != connections[i] {
			return nil, errPreviewDesktopBinding
		}
	}
	if c.manager != nil {
		view, ok := c.manager.CommandSnapshot()
		if ok != (localScope.SessionID != "") || ok && view.Scope != localScope {
			return nil, errPreviewDesktopBinding
		}
		if ok {
			result[0].local = &view
			result[0].running = view.State.Running
			result[0].pending = view.State.PendingPrompt
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || bounded.Err() != nil {
		return nil, errPreviewDesktopBinding
	}
	old := make(map[previewDesktopCatalogueKey]string, len(c.entries))
	for id, entry := range c.entries {
		old[entry.key()] = id
	}
	next := make(map[string]previewDesktopCatalogueEntry, len(result))
	seen := make(map[previewDesktopCatalogueKey]bool, len(result))
	for i := range result {
		key := result[i].key()
		if seen[key] {
			return nil, errPreviewDesktopBinding
		}
		seen[key] = true
		id := old[key]
		if id == "" {
			var entropy [16]byte
			if _, err := rand.Read(entropy[:]); err != nil {
				return nil, errPreviewDesktopBinding
			}
			id = "s-" + hex.EncodeToString(entropy[:])
		}
		if _, collision := next[id]; collision {
			return nil, errPreviewDesktopBinding
		}
		result[i].handle = id
		next[id] = result[i]
	}
	c.entries = next
	return clonePreviewCatalogue(result), nil
}

// Capture refreshes the whole current directory and returns only the original
// opaque handle, never a same-path replacement. It is still display-only;
// scoped admission must revalidate the returned instance at execution time.
func (c *previewDesktopCatalogue) Capture(ctx context.Context, handle string) (previewDesktopCatalogueEntry, error) {
	if len(handle) != 34 || handle[:2] != "s-" {
		return previewDesktopCatalogueEntry{}, errPreviewDesktopBinding
	}
	if _, err := hex.DecodeString(handle[2:]); err != nil {
		return previewDesktopCatalogueEntry{}, errPreviewDesktopBinding
	}
	entries, err := c.Refresh(ctx)
	if err != nil {
		return previewDesktopCatalogueEntry{}, err
	}
	for _, entry := range entries {
		if entry.handle == handle {
			return entry, nil
		}
	}
	return previewDesktopCatalogueEntry{}, errPreviewDesktopBinding
}

func (c *previewDesktopCatalogue) remoteConnections() []*remoteControllerConnection {
	if c.remotes == nil {
		return nil
	}
	c.remotes.mu.Lock()
	ids := make([]string, 0, len(c.remotes.controllers))
	for id := range c.remotes.controllers {
		ids = append(ids, id)
	}
	c.remotes.mu.Unlock()
	sort.Strings(ids)
	result := make([]*remoteControllerConnection, 0, len(ids))
	for _, id := range ids {
		if connection := c.remotes.getController(id); connection != nil {
			result = append(result, connection)
		}
	}
	return result
}

func clonePreviewCatalogue(entries []previewDesktopCatalogueEntry) []previewDesktopCatalogueEntry {
	result := append([]previewDesktopCatalogueEntry(nil), entries...)
	for i := range result {
		if result[i].local != nil {
			view := *result[i].local
			view.State.Pending = append([]desktopbridge.OwnedPrompt(nil), view.State.Pending...)
			result[i].local = &view
		}
	}
	return result
}
