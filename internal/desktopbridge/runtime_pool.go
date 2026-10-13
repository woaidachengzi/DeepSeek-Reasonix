package desktopbridge

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// RuntimePool owns one independent RuntimeManager per live session. Selection
// belongs to navigation only; it never cancels, rebuilds, or transfers a turn.
// Per-slot locks retain the existing command, prompt, terminal and epoch fences.
type RuntimePool struct {
	factory    RuntimeFactory
	navigation sync.Mutex
	mu         sync.RWMutex
	slots      map[string]*RuntimeManager
	lastUsed   map[string]uint64
	useClock   uint64
	capacity   int
	selected   string
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewRuntimePool(factory RuntimeFactory) *RuntimePool {
	ctx, cancel := context.WithCancel(context.Background())
	return &RuntimePool{factory: factory, slots: make(map[string]*RuntimeManager), lastUsed: make(map[string]uint64), capacity: DefaultResidentSessions, ctx: ctx, cancel: cancel}
}

func (p *RuntimePool) slot(id string) (*RuntimeManager, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	slot := p.slots[strings.TrimSpace(id)]
	if slot == nil {
		return nil, ErrSessionNotFound
	}
	return slot, nil
}

// Opens are serialized to preserve explicit navigation order. Turns and all
// session-scoped actions bypass this lock and can proceed across slots.
func (p *RuntimePool) Open(ctx context.Context, request OpenRequest) (SessionView, error) {
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	if !validSessionID(request.SessionID) {
		return SessionView{}, ErrInvalidSessionID
	}
	p.navigation.Lock()
	defer p.navigation.Unlock()
	if err := ctx.Err(); err != nil {
		return SessionView{}, err
	}
	if admission, ok := p.factory.(RuntimeOpenAdmission); ok {
		if err := admission.ValidateOpen(request); err != nil {
			return SessionView{}, err
		}
	}
	if err := p.makeRoom(request.SessionID); err != nil {
		return SessionView{}, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return SessionView{}, ErrClosed
	}
	slot := p.slots[request.SessionID]
	created := slot == nil
	if created {
		slot = NewRuntimeManager(p.factory)
		p.slots[request.SessionID] = slot
	}
	p.mu.Unlock()
	bounded, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.ctx, cancel)
	defer cancel()
	defer stop()
	view, err := slot.Open(bounded, request)
	if err == nil {
		err = bounded.Err()
	}
	if err != nil {
		if created {
			p.mu.Lock()
			delete(p.slots, request.SessionID)
			p.mu.Unlock()
			_ = slot.Shutdown()
		}
		return SessionView{}, err
	}
	// An existing slot may have continued running since its initial open.
	if current, ok := slot.SessionSnapshot(request.SessionID); ok {
		view = current
	} else {
		return SessionView{}, ErrSessionConflict
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return SessionView{}, ErrClosed
	}
	p.selected = request.SessionID
	p.useClock++
	p.lastUsed[request.SessionID] = p.useClock
	return view, nil
}

// Called under navigation. Only unselected, atomically idle owners may retire.
// All live owners count toward the bound; active work is never force-cancelled.
func (p *RuntimePool) makeRoom(id string) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrClosed
	}
	if p.slots[id] != nil || len(p.slots) < p.capacity {
		p.mu.RUnlock()
		return nil
	}
	type candidate struct {
		id   string
		used uint64
		slot *RuntimeManager
	}
	var candidates []candidate
	for key, slot := range p.slots {
		if key != p.selected {
			candidates = append(candidates, candidate{key, p.lastUsed[key], slot})
		}
	}
	p.mu.RUnlock()
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].used == candidates[j].used {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].used < candidates[j].used
	})
	for _, item := range candidates {
		retired, err := item.slot.tryRetireIdle()
		if !retired {
			continue
		}
		p.mu.Lock()
		delete(p.slots, item.id)
		delete(p.lastUsed, item.id)
		p.mu.Unlock()
		return err
	}
	return ErrRuntimeCapacity
}

func (p *RuntimePool) Switch(ctx context.Context, request OpenRequest) (SessionView, error) {
	return p.Open(ctx, request)
}

func (p *RuntimePool) selectedSlot() (*RuntimeManager, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, false
	}
	slot := p.slots[p.selected]
	return slot, slot != nil
}

func (p *RuntimePool) Snapshot() (SessionView, bool) {
	if slot, ok := p.selectedSlot(); ok {
		return slot.Snapshot()
	}
	return SessionView{}, false
}

func (p *RuntimePool) SessionSnapshot(id string) (SessionView, bool) {
	if slot, err := p.slot(id); err == nil {
		return slot.SessionSnapshot(strings.TrimSpace(id))
	}
	return SessionView{}, false
}

func (p *RuntimePool) inventory() []*RuntimeManager {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil
	}
	ids := make([]string, 0, len(p.slots))
	for id := range p.slots {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*RuntimeManager, 0, len(ids))
	for _, id := range ids {
		result = append(result, p.slots[id])
	}
	return result
}

func (p *RuntimePool) Snapshots() []SessionView {
	var views []SessionView
	for _, slot := range p.inventory() {
		if view, ok := slot.Snapshot(); ok {
			views = append(views, view)
		}
	}
	return views
}

func (p *RuntimePool) CommandSnapshot() (OwnedCommandView, bool) {
	if slot, ok := p.selectedSlot(); ok {
		return slot.CommandSnapshot()
	}
	return OwnedCommandView{}, false
}

func (p *RuntimePool) CommandSnapshots() []OwnedCommandView {
	var views []OwnedCommandView
	for _, slot := range p.inventory() {
		if view, ok := slot.CommandSnapshot(); ok {
			views = append(views, view)
		}
	}
	return views
}

// Workspace-only MCP settings retain their selected-session meaning. Commands
// that mutate a runtime always dispatch with an explicit session ID.
func (p *RuntimePool) MCPStatus(workspaceRoot string) ([]MCPRuntimeServer, bool) {
	if slot, ok := p.selectedSlot(); ok {
		return slot.MCPStatus(workspaceRoot)
	}
	return nil, false
}

func (p *RuntimePool) DeleteSession(id string) error {
	id = strings.TrimSpace(id)
	p.navigation.Lock()
	defer p.navigation.Unlock()
	slot, err := p.slot(id)
	if err != nil {
		return err
	}
	if err := slot.DeleteSession(id); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.slots, id)
	delete(p.lastUsed, id)
	if p.selected == id {
		p.selected = ""
	}
	p.mu.Unlock()
	return slot.Shutdown()
}

// Archive checks only the targeted owner. Opening that same saved identity is
// excluded until persistence completes; another slot's turn remains untouched.
func (p *RuntimePool) ChangeArchive(id string, archived bool, commit func(bool) error) error {
	id = strings.TrimSpace(id)
	if !validSessionID(id) || commit == nil {
		return ErrInvalidInput
	}
	p.navigation.Lock()
	defer p.navigation.Unlock()
	slot, err := p.slot(id)
	if errors.Is(err, ErrSessionNotFound) {
		return commit(false)
	}
	if err != nil {
		return err
	}
	if err := slot.ChangeArchive(id, archived, commit); err != nil {
		return err
	}
	if archived {
		p.mu.Lock()
		delete(p.slots, id)
		delete(p.lastUsed, id)
		if p.selected == id {
			p.selected = ""
		}
		p.mu.Unlock()
		return slot.Shutdown()
	}
	return nil
}

// Shutdown closes every slot, including background and opening owners, even
// when one close fails. No subsequent navigation or command can be admitted.
func (p *RuntimePool) Shutdown() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.cancel()
	slots := p.slots
	p.slots = nil
	p.lastUsed = nil
	p.selected = ""
	p.mu.Unlock()
	var result error
	for _, slot := range slots {
		result = errors.Join(result, slot.Shutdown())
	}
	return result
}
