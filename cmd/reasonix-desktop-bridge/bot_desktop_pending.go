package main

import (
	"context"
	"encoding/json"
	"time"

	"reasonix/internal/eventwire"
	remotecontroller "reasonix/internal/remote/controller"
)

// ReadPending uses the original directory capture, never a same-ID/path new
// owner. This is host-private data: the full host must enforce chat/actor/admin
// and audience privacy before displaying it or issuing operation tickets.
func (c *previewDesktopCatalogue) ReadPending(ctx context.Context, entry previewDesktopCatalogueEntry) (remotecontroller.SessionPendingView, error) {
	if ctx == nil || !c.pendingEntryCurrent(entry) {
		return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	defer func() { stop(); cancel() }()
	if bounded.Err() != nil {
		return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
	}
	var result remotecontroller.SessionPendingView
	if entry.local != nil {
		if c.manager == nil {
			return result, errPreviewDesktopBinding
		}
		view, ok := c.manager.CommandSnapshot()
		if !ok || view.Scope != entry.local.Scope {
			return result, errPreviewDesktopBinding
		}
		scope := remotecontroller.SessionPendingScope{SessionPath: view.Scope.SessionPath, RuntimeEpoch: view.Scope.RuntimeEpoch}
		result = remotecontroller.SessionPendingView{ProtocolVersion: 1, SessionPendingScope: scope, Revision: view.State.Revision, TurnID: view.State.TurnID, Prompts: []remotecontroller.SessionPendingPrompt{}}
		if len(view.State.Pending) > 32 {
			return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
		}
		for _, prompt := range view.State.Pending {
			payload, err := c.manager.ReadOwnedPrompt(bounded, view.Scope, prompt)
			var wire eventwire.Event
			if err != nil || json.Unmarshal(payload, &wire) != nil {
				return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
			}
			result.Prompts = append(result.Prompts, remotecontroller.SessionPendingPrompt{Scope: remotecontroller.SessionPromptScope{SessionPath: scope.SessionPath, RuntimeEpoch: scope.RuntimeEpoch, TurnID: prompt.TurnID, PromptID: prompt.ID, PromptRuntimeEpoch: prompt.RuntimeEpoch, Kind: prompt.Kind}, Ask: wire.Ask, Approval: wire.Approval, MCPInteraction: wire.MCPInteraction})
		}
		after, ok := c.manager.CommandSnapshot()
		if !ok || after.Scope != view.Scope || after.State.Revision != view.State.Revision || view.State.PendingPrompt != (len(result.Prompts) != 0) || !remotecontroller.ValidSessionPendingView(result, scope) {
			return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
		}
	} else {
		if entry.remote == nil || c.remotes == nil || c.remotes.getController(entry.remote.view.ID) != entry.remote {
			return result, errPreviewDesktopBinding
		}
		var err error
		result, err = entry.remote.client.ReadSessionPending(bounded, remotecontroller.SessionPendingScope{SessionPath: entry.remotePath, RuntimeEpoch: entry.remoteEpoch})
		if err != nil || c.remotes.getController(entry.remote.view.ID) != entry.remote {
			return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
		}
	}
	if bounded.Err() != nil || !c.pendingEntryCurrent(entry) {
		return remotecontroller.SessionPendingView{}, errPreviewDesktopBinding
	}
	return result, nil
}

func (c *previewDesktopCatalogue) pendingEntryCurrent(entry previewDesktopCatalogueEntry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.entries[entry.handle]
	return !c.closed && c.ctx.Err() == nil && ok && current.key() == entry.key()
}
