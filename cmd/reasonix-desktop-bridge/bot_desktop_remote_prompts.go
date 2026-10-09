package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/eventwire"
	remotecontroller "reasonix/internal/remote/controller"
)

type previewRemotePromptTicket struct {
	entry     previewDesktopCatalogueEntry
	route     bot.DesktopWatchRoute
	actor     string
	scope     remotecontroller.SessionPromptScope
	payload   json.RawMessage
	issued    time.Time
	attempted bool
}

// Remote tickets never borrow a local manager, raw ID-only API or replacement
// tunnel. Gateway admission/admin and audience filtering remain host duties.
type previewDesktopRemotePrompts struct {
	catalogue *previewDesktopCatalogue
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	tickets   map[string]previewRemotePromptTicket
}

func newPreviewDesktopRemotePrompts(catalogue *previewDesktopCatalogue) *previewDesktopRemotePrompts {
	parent := context.Background()
	if catalogue != nil {
		parent = catalogue.ctx
	}
	ctx, cancel := context.WithCancel(parent)
	return &previewDesktopRemotePrompts{catalogue: catalogue, ctx: ctx, cancel: cancel, tickets: make(map[string]previewRemotePromptTicket)}
}

func (p *previewDesktopRemotePrompts) operation(ctx context.Context) (context.Context, func()) {
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	stop := context.AfterFunc(p.ctx, cancel)
	return bounded, func() { stop(); cancel() }
}

func (p *previewDesktopRemotePrompts) pruneLocked() {
	for id, ticket := range p.tickets {
		// Spent tickets are bounded tombstones until owner/service retirement;
		// a display refresh after expiry must not rearm an unknown dispatch.
		if p.closed || p.ctx.Err() != nil || p.catalogue == nil || !ticket.attempted && (time.Since(ticket.issued) > previewDesktopPromptLifetime || !p.catalogue.pendingEntryCurrent(ticket.entry)) || ticket.entry.remote == nil || p.catalogue.remotes == nil || p.catalogue.remotes.getController(ticket.entry.remote.view.ID) != ticket.entry.remote {
			delete(p.tickets, id)
		}
	}
}

func pendingPromptWire(prompt remotecontroller.SessionPendingPrompt) json.RawMessage {
	raw, _ := json.Marshal(eventwire.Event{Ask: prompt.Ask, Approval: prompt.Approval, MCPInteraction: prompt.MCPInteraction})
	return raw
}

func (p *previewDesktopRemotePrompts) Issue(ctx context.Context, route bot.DesktopWatchRoute, actor string, entry previewDesktopCatalogueEntry, expected remotecontroller.SessionPromptScope) (string, json.RawMessage, error) {
	if ctx == nil || ctx.Err() != nil || p.ctx.Err() != nil || p.catalogue == nil || entry.local != nil || entry.remote == nil || !validPreviewWatcher(previewDesktopWatcher{route, actor}) {
		return "", nil, errPreviewDesktopBinding
	}
	bounded, finish := p.operation(ctx)
	defer finish()
	// No service mutex spans HTTP: Close can cancel and retire pending reads.
	snapshot, err := p.catalogue.ReadPending(bounded, entry)
	if err != nil {
		return "", nil, errPreviewDesktopBinding
	}
	var payload json.RawMessage
	for _, prompt := range snapshot.Prompts {
		if prompt.Scope == expected {
			payload = pendingPromptWire(prompt)
			break
		}
	}
	if len(payload) == 0 || len(payload) > 64<<10 {
		return "", nil, errPreviewDesktopBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked()
	if p.closed || bounded.Err() != nil || p.ctx.Err() != nil || !p.catalogue.pendingEntryCurrent(entry) {
		return "", nil, errPreviewDesktopBinding
	}
	for id, ticket := range p.tickets {
		if ticket.entry.key() == entry.key() && ticket.scope == expected && ticket.route == route && ticket.actor == actor {
			// Display refresh cannot rearm an attempted or unknown decision.
			return id, append(json.RawMessage(nil), ticket.payload...), nil
		}
	}
	if len(p.tickets) >= previewDesktopPromptLimit {
		return "", nil, errPreviewDesktopBinding
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", nil, errPreviewDesktopBinding
	}
	id := "rp-" + hex.EncodeToString(entropy[:])
	if _, exists := p.tickets[id]; exists {
		return "", nil, errPreviewDesktopBinding
	}
	p.tickets[id] = previewRemotePromptTicket{entry: entry, route: route, actor: actor, scope: expected, payload: payload, issued: time.Now()}
	return id, append(json.RawMessage(nil), payload...), nil
}

func (p *previewDesktopRemotePrompts) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || ctx.Err() != nil || p.ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	bounded, finish := p.operation(ctx)
	defer finish()
	p.mu.Lock()
	p.pruneLocked()
	ticket, exists := p.tickets[command.TargetID]
	if !exists || ticket.attempted || ticket.route != command.Route || ticket.actor != command.ActorID || bounded.Err() != nil {
		p.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	answer, err := previewDesktopPromptAnswer(command, ticket.scope, ticket.payload)
	if err != nil {
		p.mu.Unlock()
		return "", err
	}
	// Reserve once before any fresh read or dispatch. An uncertain attempt is
	// not retryable, even if a display refresh sees the same pending identity.
	ticket.attempted = true
	p.tickets[command.TargetID] = ticket
	p.mu.Unlock()
	snapshot, err := p.catalogue.ReadPending(bounded, ticket.entry)
	if err != nil {
		return "", errPreviewDesktopBinding
	}
	current := false
	for _, prompt := range snapshot.Prompts {
		if prompt.Scope == ticket.scope {
			current = true
			break
		}
	}
	if !current || p.ctx.Err() != nil || bounded.Err() != nil || !p.catalogue.pendingEntryCurrent(ticket.entry) || p.catalogue.remotes.getController(ticket.entry.remote.view.ID) != ticket.entry.remote {
		return "", errPreviewDesktopBinding
	}
	// The client and Serve keep the original exact scope through single-shot
	// admission. No mutex on this service is held across external IO.
	_, err = ticket.entry.remote.client.ResolveSessionPrompt(bounded, remotecontroller.SessionPromptRequest{SessionPromptScope: ticket.scope, Answer: json.RawMessage(answer)})
	if err != nil || bounded.Err() != nil || p.ctx.Err() != nil || !p.catalogue.pendingEntryCurrent(ticket.entry) || p.catalogue.remotes.getController(ticket.entry.remote.view.ID) != ticket.entry.remote {
		return "", errPreviewDesktopBinding
	}
	return "已提交该远程提示的决定；这不表示任务或保存已完成。", nil
}

func (p *previewDesktopRemotePrompts) Close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	clear(p.tickets)
}
