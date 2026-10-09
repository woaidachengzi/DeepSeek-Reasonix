package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/remote/controller"
)

type desktopSessionPromptRequest struct {
	ProtocolVersion int `json:"protocolVersion"`
	controller.SessionPromptRequest
}

// Authenticated, owner-scoped user decisions; no history adoption or local
// foreground fallback. The immutable prompt identity selects the resolver.
func (s *Server) desktopSessionPrompt(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input desktopSessionPromptRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if r.URL.RawQuery != "" || d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.ProtocolVersion != 1 {
		http.Error(w, "select one current prompt and a valid decision", 400)
		return
	}
	answer, err := controller.DecodeSessionPromptAnswer(input.SessionPromptRequest)
	if err != nil {
		http.Error(w, "select one current prompt and a valid decision", 400)
		return
	}
	if ctx.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "remote prompt changed; refresh before answering", 409)
		return
	}
	defer s.bindMu.Unlock()
	path, err := s.resolveSessionPath(input.SessionPath)
	if err != nil || agent.CanonicalSessionPath(path) != input.SessionPath {
		http.Error(w, "remote session is unavailable", 404)
		return
	}
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		http.Error(w, "remote decision requires an owned session", 409)
		return
	}
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
			http.Error(w, "remote prompt changed; refresh before answering", 409)
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
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != input.SessionPath || ctx.Err() != nil {
		http.Error(w, "remote prompt changed; refresh before answering", 409)
		return
	}
	questions := make([]event.AskAnswer, len(answer.Questions))
	for i, q := range answer.Questions {
		questions[i] = event.AskAnswer{QuestionID: q.QuestionID, Selected: q.Selected}
	}
	err = owner.ResolvePromptScopedContext(ctx,
		control.PromptResolveScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.RuntimeEpoch, TurnID: input.TurnID},
		control.PromptIdentity{PromptID: input.PromptID, TurnID: input.TurnID, RuntimeEpoch: input.PromptRuntimeEpoch, Kind: control.PromptKind(input.Kind)},
		control.PromptAnswer{Questions: questions, Allow: answer.Allow, Session: answer.Session, Persist: answer.Persist, Action: answer.Action, Feedback: answer.Feedback, Content: answer.Content})
	if err != nil {
		if errors.Is(err, control.ErrPromptResolveScope) || errors.Is(err, control.ErrPromptStaleTurn) || errors.Is(err, control.ErrPromptStaleRuntime) || errors.Is(err, control.ErrPromptAlreadyResolved) || errors.Is(err, control.ErrPromptNotPending) {
			http.Error(w, "remote prompt changed; refresh before answering", 409)
			return
		}
		// A durable resolver can fail after a decision has begun. Do not imply
		// that retry is safe, and never expose private persistence diagnostics.
		http.Error(w, "remote decision was not confirmed; refresh and do not automatically retry", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(controller.SessionPromptReceipt{ProtocolVersion: 1, SessionPromptScope: input.SessionPromptScope, Resolved: true})
}
