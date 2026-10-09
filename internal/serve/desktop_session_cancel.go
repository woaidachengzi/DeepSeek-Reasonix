package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
)

type desktopSessionCancelRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	SessionPath     string `json:"sessionPath"`
	RuntimeEpoch    string `json:"runtimeEpoch"`
	TurnID          string `json:"turnId"`
}

// This is a stop command, not a writable projection, saved-session adoption or
// foreground switch. Authentication wraps the route like every Serve command.
func (s *Server) desktopSessionCancel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input desktopSessionCancelRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 40<<10))
	decoder.DisallowUnknownFields()
	if r.URL.RawQuery != "" || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
		input.ProtocolVersion != 1 || input.SessionPath == "" || !desktopImageField(input.SessionPath, 32768) ||
		input.RuntimeEpoch == "" || !desktopImageField(input.RuntimeEpoch, 4096) || input.TurnID == "" || !desktopImageField(input.TurnID, 4096) {
		http.Error(w, "select one current remote turn to stop", 400)
		return
	}
	// Read/bound the body before taking the publication lock. Never queue a
	// delayed mutation behind a foreground replacement or slow upload.
	if ctx.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "remote session changed; refresh before stopping", 409)
		return
	}
	defer s.bindMu.Unlock()
	path, err := s.resolveSessionPath(input.SessionPath)
	if err != nil || agent.CanonicalSessionPath(path) != input.SessionPath {
		http.Error(w, "remote session is unavailable", 404)
		return
	}
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		http.Error(w, "remote turn requires an owned session", 409)
		return
	}
	var owner *control.Controller
	if foreground := s.ctl(); foreground != nil && agent.CanonicalSessionPath(foreground.SessionPath()) == input.SessionPath {
		owner, _ = foreground.(*control.Controller)
	} else {
		s.detachedMu.Lock()
		if detached := s.detached[input.SessionPath]; detached != nil && !detached.retiring {
			owner, _ = detached.ctrl.(*control.Controller)
		}
		s.detachedMu.Unlock()
	}
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != input.SessionPath || ctx.Err() != nil || owner.CancelScopedContext(ctx, control.TurnCancelScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.RuntimeEpoch, TurnID: input.TurnID}) != nil {
		http.Error(w, "remote turn changed; refresh before stopping", 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		desktopSessionCancelRequest
		Cancelled bool `json:"cancelled"`
	}{input, true})
}
