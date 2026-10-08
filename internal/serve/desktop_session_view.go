package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

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
	History         []desktopHistoryMessage     `json:"history"`
}

// Identity belongs to the stored/Controller message, not its position in a
// filtered display projection. Legacy files receive deterministic IDs from
// agent.LoadSession; a snapshot must never mint IDs or rewrite those files.
type desktopHistoryMessage struct {
	ID string `json:"id"`
	historyMessage
}

func desktopHistoryMessages(messages []provider.Message) ([]desktopHistoryMessage, error) {
	out := make([]desktopHistoryMessage, 0, len(messages))
	seen := make(map[string]bool, len(messages))
	for _, message := range historyWithoutPinnedContextRevisions(messages) {
		rows := historyMessages([]provider.Message{message})
		if len(rows) == 0 {
			continue
		}
		// All supported projections produce at most one display row per
		// backend entry, including steer/recovery sentinels. Fail explicitly
		// if that contract changes rather than inventing positional suffixes.
		if len(rows) != 1 || message.ID == "" || len(message.ID) > 4096 || !utf8.ValidString(message.ID) || strings.IndexFunc(message.ID, unicode.IsControl) >= 0 || seen[message.ID] {
			return nil, errors.New("remote history identity is invalid")
		}
		seen[message.ID] = true
		out = append(out, desktopHistoryMessage{ID: message.ID, historyMessage: rows[0]})
	}
	return out, nil
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
	v.History, err = desktopHistoryMessages(messages)
	if err != nil {
		http.Error(w, "remote history identity is unavailable", 422)
		return
	}
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
