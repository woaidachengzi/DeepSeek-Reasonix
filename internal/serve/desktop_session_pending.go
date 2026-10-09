package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

func (s *Server) desktopSessionPending(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input struct {
		ProtocolVersion int `json:"protocolVersion"`
		controller.SessionPendingScope
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if r.URL.RawQuery != "" || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ProtocolVersion != 1 || !controller.ValidSessionPendingScope(input.SessionPendingScope) {
		http.Error(w, "select one current session instance", 400)
		return
	}
	if ctx.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "remote prompt snapshot changed", 409)
		return
	}
	defer s.bindMu.Unlock()
	// Match published owners directly, without a transcript/catalogue disk read.
	var owner *control.Controller
	var detachedOwner *detachedSession
	if foreground := s.ctl(); foreground != nil && agent.CanonicalSessionPath(foreground.SessionPath()) == input.SessionPath {
		owner, _ = foreground.(*control.Controller)
	} else {
		s.detachedMu.Lock()
		if detached := s.detached[input.SessionPath]; detached != nil && !detached.retiring {
			owner, _ = detached.ctrl.(*control.Controller)
			detachedOwner = detached
		}
		s.detachedMu.Unlock()
	}
	if detachedOwner != nil {
		if !detachedOwner.admissionMu.TryLock() {
			http.Error(w, "remote prompt snapshot changed", 409)
			return
		}
		defer detachedOwner.admissionMu.Unlock()
		s.detachedMu.Lock()
		valid := s.detached[input.SessionPath] == detachedOwner && !detachedOwner.retiring && detachedOwner.ctrl == owner
		s.detachedMu.Unlock()
		if !valid {
			owner = nil
		}
	}
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != input.SessionPath || s.sessionMirrored(input.SessionPath) || s.runtimeForeignLeaseReadOnly(input.SessionPath) || ctx.Err() != nil {
		http.Error(w, "remote prompt snapshot requires a current owned session", 409)
		return
	}
	state := owner.RuntimeStateSnapshot()
	if state.SchemaVersion != 1 || state.RuntimeEpoch != input.RuntimeEpoch || state.Revision == 0 || state.Phase == "closed" || state.CancelRequested {
		http.Error(w, "remote prompt snapshot changed", 409)
		return
	}
	result := controller.SessionPendingView{ProtocolVersion: 1, SessionPendingScope: input.SessionPendingScope, Revision: state.Revision, TurnID: state.TurnID, Prompts: []controller.SessionPendingPrompt{}}
	for _, identity := range owner.PendingPromptIdentities() {
		if !state.Running || identity.TurnID != state.TurnID {
			continue
		}
		if len(result.Prompts) >= 32 {
			http.Error(w, "remote prompt snapshot unavailable", 422)
			return
		}
		payload, err := owner.ReadPromptScopedContext(ctx, control.PromptResolveScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.RuntimeEpoch, TurnID: state.TurnID}, identity)
		var wire eventwire.Event
		if err != nil || json.Unmarshal(payload, &wire) != nil {
			http.Error(w, "remote prompt snapshot changed", 409)
			return
		}
		result.Prompts = append(result.Prompts, controller.SessionPendingPrompt{Scope: controller.SessionPromptScope{SessionPath: input.SessionPath, RuntimeEpoch: input.RuntimeEpoch, TurnID: identity.TurnID, PromptID: identity.PromptID, PromptRuntimeEpoch: identity.RuntimeEpoch, Kind: string(identity.Kind)}, Ask: wire.Ask, Approval: wire.Approval, MCPInteraction: wire.MCPInteraction})
	}
	if owner.RuntimeStateSnapshot() != state || state.PendingPrompt != (len(result.Prompts) != 0) || ctx.Err() != nil || !controller.ValidSessionPendingView(result, input.SessionPendingScope) {
		http.Error(w, "remote prompt snapshot changed", 409)
		return
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 2<<20 {
		http.Error(w, "remote prompt snapshot unavailable", 422)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}
