package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/control"
)

type desktopSessionSubmitRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	SessionPath     string `json:"sessionPath"`
	RuntimeEpoch    string `json:"runtimeEpoch"`
	Revision        uint64 `json:"revision"`
	Text            string `json:"text"`
}

// This endpoint admits user text only to an already-owned idle runtime. It
// never interprets commands, restores saved sessions, or changes the foreground.
func (s *Server) desktopSessionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input desktopSessionSubmitRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if r.URL.RawQuery != "" || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
		input.ProtocolVersion != 1 || input.SessionPath == "" || !desktopImageField(input.SessionPath, 32768) ||
		input.RuntimeEpoch == "" || !desktopImageField(input.RuntimeEpoch, 4096) || input.Revision == 0 || input.Revision > 9_007_199_254_740_991 ||
		strings.TrimSpace(input.Text) == "" || len(input.Text) > 512<<10 || !utf8.ValidString(input.Text) || strings.ContainsRune(input.Text, 0) {
		http.Error(w, "select one idle remote session and enter a message", 400)
		return
	}
	if ctx.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "remote session changed; refresh before sending", 409)
		return
	}
	defer s.bindMu.Unlock()
	path, err := s.resolveSessionPath(input.SessionPath)
	if err != nil || agent.CanonicalSessionPath(path) != input.SessionPath {
		http.Error(w, "remote session is unavailable", 404)
		return
	}
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		http.Error(w, "remote message requires an owned session", 409)
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
		// The idle-close watcher and model rebuild use this same owner gate.
		// Checking retiring only while looking up the registry leaves a gap in
		// which an idle owner can retire before its running reservation is made.
		if !detachedOwner.admissionMu.TryLock() {
			http.Error(w, "remote session changed; refresh before sending", 409)
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
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != input.SessionPath || ctx.Err() != nil ||
		owner.SubmitScopedContext(ctx, control.TurnSubmitScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.RuntimeEpoch, Revision: input.Revision}, input.Text) != nil {
		http.Error(w, "remote session changed; refresh before sending", 409)
		return
	}
	// Accepted means admission was reserved, not completion or durable provider
	// success. Echo only the admission scope, never prompt/config/private state.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		ProtocolVersion int    `json:"protocolVersion"`
		SessionPath     string `json:"sessionPath"`
		RuntimeEpoch    string `json:"runtimeEpoch"`
		Revision        uint64 `json:"revision"`
		Accepted        bool   `json:"accepted"`
	}{1, input.SessionPath, input.RuntimeEpoch, input.Revision, true})
}
