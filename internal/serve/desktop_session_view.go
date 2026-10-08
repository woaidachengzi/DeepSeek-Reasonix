package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type desktopSessionView struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	SessionPath     string                      `json:"sessionPath"`
	ReadOnly        bool                        `json:"readOnly"`
	Ownership       string                      `json:"ownership"`
	Current         bool                        `json:"current"`
	ModelRef        string                      `json:"modelRef"`
	Label           string                      `json:"label"`
	RuntimeState    *event.RuntimeStateSnapshot `json:"runtimeState,omitempty"`
	History         []historyMessage            `json:"history"`
}

// Explicit spectator snapshot: no foreground fallback, balance/provider I/O,
// auto-reclaim, lease rebind, resume or prompt replay. bindMu fences controller
// replacement, not streaming token arrival; runtime is an observation sampled
// after history, not a promise of an atomic turn/event transaction.
func (s *Server) desktopSessionView(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) != 1 || len(query["session"]) != 1 || strings.TrimSpace(query.Get("session")) == "" {
		http.Error(w, "select one remote session", 400)
		return
	}
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	path, err := s.resolveSessionPath(query.Get("session"))
	if err != nil || agent.CanonicalSessionPath(path) != query.Get("session") {
		http.Error(w, "remote session is unavailable", 404)
		return
	}
	canonical := agent.CanonicalSessionPath(path)
	foreground := s.ctl()
	v := desktopSessionView{ProtocolVersion: 1, SessionPath: canonical, ReadOnly: true, Ownership: "saved", Current: agent.CanonicalSessionPath(foreground.SessionPath()) == canonical}
	var ctrl control.SessionAPI
	var detached *detachedSession
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		v.Ownership = "external"
	} else {
		if v.Current {
			ctrl = foreground
		} else {
			s.detachedMu.Lock()
			if d := s.detached[canonical]; d != nil {
				detached = d
				if !d.retiring {
					ctrl = d.ctrl
				}
			}
			s.detachedMu.Unlock()
			if detached != nil && ctrl == nil {
				http.Error(w, "remote session is retiring", 409)
				return
			}
		}
	}
	var messages []provider.Message
	if ctrl != nil {
		if agent.CanonicalSessionPath(ctrl.SessionPath()) != canonical {
			http.Error(w, "remote session changed", 409)
			return
		}
		messages = ctrl.History()
		state := runtimeStateOf(ctrl)
		v.RuntimeState = &state
		v.Ownership = "serve"
		v.ModelRef = currentModelRef(ctrl)
		v.Label = ctrl.Label()
		if agent.CanonicalSessionPath(ctrl.SessionPath()) != canonical {
			http.Error(w, "remote session changed", 409)
			return
		}
	} else {
		// Bound saved/external file reads before the common transcript loader.
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			http.Error(w, "remote history is unavailable", 422)
			return
		}
		loaded, err := agent.LoadSession(path)
		if err != nil || loaded == nil {
			http.Error(w, "remote history is unavailable", 422)
			return
		}
		messages = loaded.Messages
	}
	v.History = historyMessages(messages)
	if detached != nil {
		s.detachedMu.Lock()
		valid := s.detached[canonical] == detached && !detached.retiring && detached.ctrl == ctrl
		s.detachedMu.Unlock()
		if !valid {
			http.Error(w, "remote session changed", 409)
			return
		}
	}
	data, err := json.Marshal(v)
	if err != nil || len(data) > 30<<20 || len(v.History) > 100000 {
		http.Error(w, "remote history exceeds display budget", 422)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
