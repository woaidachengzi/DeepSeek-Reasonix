package serve

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"weak"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/eventwire"
	"reasonix/internal/turnevent"
)

const desktopProjectionPageLimit = 64
const desktopProjectionPageTTL = 2 * time.Minute

type desktopProjectionPage struct {
	owner        weak.Pointer[control.Controller]
	path         string
	boundary     turnevent.ProjectionReplayBoundary
	activeTurnID string
	status       string
	replayAfter  uint64
	expires      time.Time
}

type desktopProjectionReplay struct {
	Events            []eventwire.Event `json:"events"`
	FloorSequence     uint64            `json:"floorSeq"`
	LatestSequence    uint64            `json:"latestSeq"`
	NextAfterSequence uint64            `json:"nextAfterSeq"`
	HasMore           bool              `json:"hasMore"`
	RuntimeEpoch      string            `json:"runtimeEpoch,omitempty"`
}

type desktopSessionProjection struct {
	ProtocolVersion     int                     `json:"protocolVersion"`
	SessionPath         string                  `json:"sessionPath"`
	ReadOnly            bool                    `json:"readOnly"`
	Initial             bool                    `json:"initial"`
	History             []desktopHistoryMessage `json:"history"`
	UserSuffix          []desktopHistoryMessage `json:"userSuffix"`
	ActiveTurnID        string                  `json:"activeTurnId,omitempty"`
	TurnStatus          string                  `json:"turnStatus,omitempty"`
	ReplayAfterSequence uint64                  `json:"replayAfterSeq"`
	Replay              desktopProjectionReplay `json:"replay"`
	PageToken           string                  `json:"pageToken,omitempty"`
}

// A spectator read of an already-owned controller, never a saved-session
// adoption or writable lease. bindMu fences replacement; the controller owns
// the transcript/turn cut. Readers must install live listeners before capture.
func (s *Server) desktopSessionProjection(w http.ResponseWriter, r *http.Request) {
	q, err := parseProjectionQuery(r)
	if err != nil {
		http.Error(w, "select one remote projection", 400)
		return
	}
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	now := time.Now()
	for token, page := range s.desktopProjectionPages {
		if !now.Before(page.expires) {
			delete(s.desktopProjectionPages, token)
		}
	}
	path, err := s.resolveSessionPath(q.path)
	if err != nil || agent.CanonicalSessionPath(path) != q.path {
		http.Error(w, "remote session is unavailable", 404)
		return
	}
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		http.Error(w, "remote projection requires an owned session", 409)
		return
	}
	var owner *control.Controller
	var detached *detachedSession
	if foreground := s.ctl(); foreground != nil && agent.CanonicalSessionPath(foreground.SessionPath()) == q.path {
		owner, _ = foreground.(*control.Controller)
	} else {
		s.detachedMu.Lock()
		detached = s.detached[q.path]
		if detached != nil && !detached.retiring {
			owner, _ = detached.ctrl.(*control.Controller)
		}
		s.detachedMu.Unlock()
	}
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != q.path {
		http.Error(w, "remote projection is unavailable", 409)
		return
	}
	response := desktopSessionProjection{ProtocolVersion: 1, SessionPath: q.path, ReadOnly: true, Initial: q.token == "", History: []desktopHistoryMessage{}, UserSuffix: []desktopHistoryMessage{}}
	var replay turnevent.ReplayView
	var continuation desktopProjectionPage
	if response.Initial {
		view, err := owner.TurnProjectionView()
		if err != nil || view.Projection.ResetRequired {
			http.Error(w, "remote projection changed; refresh snapshot", 409)
			return
		}
		response.History, err = desktopHistoryMessages(view.Prefix)
		if err == nil {
			response.UserSuffix, err = desktopHistoryMessages(view.UserSuffix)
		}
		if err != nil {
			http.Error(w, "remote projection identity is unavailable", 422)
			return
		}
		seen := make(map[string]bool, len(response.History)+len(response.UserSuffix))
		for _, rows := range [][]desktopHistoryMessage{response.History, response.UserSuffix} {
			for _, row := range rows {
				if seen[row.ID] {
					http.Error(w, "remote projection identity is unavailable", 422)
					return
				}
				seen[row.ID] = true
			}
		}
		replay = view.Projection.ReplayView
		continuation = desktopProjectionPage{owner: weak.Make(owner), path: q.path, boundary: view.Projection.Boundary, activeTurnID: view.Projection.ActiveTurnID, status: string(view.Projection.TurnStatus), replayAfter: view.Projection.ReplayAfterSequence, expires: now.Add(desktopProjectionPageTTL)}
	} else {
		page, ok := s.desktopProjectionPages[q.token]
		if !ok || page.owner.Value() != owner || page.path != q.path {
			http.Error(w, "remote projection expired or changed; refresh snapshot", 409)
			return
		}
		continuation = page
		replay, err = owner.TurnProjectionReplayPage(page.boundary, q.after)
		if err != nil || replay.ResetRequired {
			delete(s.desktopProjectionPages, q.token)
			http.Error(w, "remote projection changed; refresh snapshot", 409)
			return
		}
	}
	response.ActiveTurnID, response.TurnStatus = continuation.activeTurnID, continuation.status
	response.ReplayAfterSequence = continuation.replayAfter
	response.Replay = desktopProjectionReplay{Events: make([]eventwire.Event, 0, len(replay.Events)), FloorSequence: replay.FloorSequence, LatestSequence: replay.LatestSequence, NextAfterSequence: replay.NextAfterSequence, HasMore: replay.HasMore, RuntimeEpoch: replay.RuntimeEpoch}
	for _, envelope := range replay.Events {
		frame := envelope.Event
		frame.SessionPath = q.path
		response.Replay.Events = append(response.Replay.Events, frame)
	}
	if agent.CanonicalSessionPath(owner.SessionPath()) != q.path {
		http.Error(w, "remote session changed", 409)
		return
	}
	if detached != nil {
		s.detachedMu.Lock()
		valid := s.detached[q.path] == detached && !detached.retiring && detached.ctrl == owner
		s.detachedMu.Unlock()
		if !valid {
			http.Error(w, "remote session changed", 409)
			return
		}
	}
	if replay.HasMore {
		if response.Initial {
			if len(s.desktopProjectionPages) >= desktopProjectionPageLimit {
				http.Error(w, "remote projection busy; retry later", 429)
				return
			}
			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				http.Error(w, "remote projection is unavailable", 503)
				return
			}
			response.PageToken = hex.EncodeToString(nonce[:])
			if _, exists := s.desktopProjectionPages[response.PageToken]; exists {
				http.Error(w, "remote projection is unavailable", 503)
				return
			}
		} else {
			response.PageToken = q.token
		}
	}
	data, err := json.Marshal(response)
	if err != nil || len(data) > 30<<20 || len(response.History)+len(response.UserSuffix) > 100000 {
		http.Error(w, "remote projection exceeds display budget", 422)
		return
	}
	if response.Initial && response.PageToken != "" {
		if !time.Now().Before(continuation.expires) {
			http.Error(w, "remote projection expired; refresh snapshot", 409)
			return
		}
		if s.desktopProjectionPages == nil {
			s.desktopProjectionPages = make(map[string]desktopProjectionPage)
		}
		s.desktopProjectionPages[response.PageToken] = continuation
	}
	// Keep completed handles until their original deadline, allowing a lost
	// final response to be retried without moving the cut or acknowledging it.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

type projectionQuery struct {
	path, token string
	after       uint64
}

func parseProjectionQuery(r *http.Request) (projectionQuery, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return projectionQuery{}, err
	}
	if len(values) != 1 && len(values) != 3 {
		return projectionQuery{}, strconv.ErrSyntax
	}
	for key, entries := range values {
		if (key != "session" && key != "page" && key != "after") || len(entries) != 1 {
			return projectionQuery{}, strconv.ErrSyntax
		}
	}
	if strings.TrimSpace(values.Get("session")) == "" || (len(values) == 3 && (values.Get("page") == "" || values.Get("after") == "")) {
		return projectionQuery{}, strconv.ErrSyntax
	}
	q := projectionQuery{path: values.Get("session"), token: values.Get("page")}
	if q.token != "" {
		if len(q.token) != 32 || strings.ToLower(q.token) != q.token {
			return q, strconv.ErrSyntax
		}
		if _, err := hex.DecodeString(q.token); err != nil {
			return q, err
		}
		raw := values.Get("after")
		q.after, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || q.after > 9_007_199_254_740_991 || strconv.FormatUint(q.after, 10) != raw {
			return q, strconv.ErrSyntax
		}
	}
	return q, nil
}
