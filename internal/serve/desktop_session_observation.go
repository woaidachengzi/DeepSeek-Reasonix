package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/remote/controller"
)

// Admission holds the published-owner gates only while binding the exact
// subscription. Network writes never hold those gates or Controller locks.
func (s *Server) desktopObservationOwner(path string) *control.Controller {
	if foreground := s.ctl(); foreground != nil && agent.CanonicalSessionPath(foreground.SessionPath()) == path {
		owner, _ := foreground.(*control.Controller)
		return owner
	}
	s.detachedMu.Lock()
	defer s.detachedMu.Unlock()
	if detached := s.detached[path]; detached != nil && !detached.retiring {
		owner, _ := detached.ctrl.(*control.Controller)
		return owner
	}
	return nil
}

// Established observers must distinguish ordinary binding-lock contention
// from retirement. Input admission also holds bindMu while publishing events.
// Wait cancellably, then recheck the exact published owner; never hold a gate
// across network IO or treat a busy gate as permission to publish.
func (s *Server) desktopObservationCurrent(ctx context.Context, retired <-chan struct{}, path string, owner *control.Controller) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-retired:
			return false
		default:
		}
		if s.bindMu.TryLock() {
			current := s.desktopObservationOwner(path) == owner && !s.sessionMirrored(path) && !s.runtimeForeignLeaseReadOnly(path)
			s.bindMu.Unlock()
			select {
			case <-ctx.Done():
				return false
			case <-retired:
				return false
			default:
				return current
			}
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-retired:
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (s *Server) desktopSessionObservation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	admission, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	stop := context.AfterFunc(admission, func() { _ = r.Body.Close() })
	defer cancel()
	defer stop()
	var input controller.SessionObservationRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if r.URL.RawQuery != "" || r.Header.Get("Last-Event-ID") != "" || d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || !controller.ValidSessionObservationRequest(input) {
		http.Error(w, "select one current observation scope", 400)
		return
	}
	if admission.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "observation owner changed", 409)
		return
	}
	path := input.Scope.SessionPath
	owner := s.desktopObservationOwner(path)
	// Detached retirement/model replacement has its own admission gate; do
	// not establish a new observer while that generation is being retired.
	var detached *detachedSession
	if foreground := s.ctl(); foreground == nil || agent.CanonicalSessionPath(foreground.SessionPath()) != path {
		s.detachedMu.Lock()
		detached = s.detached[path]
		s.detachedMu.Unlock()
	}
	if detached != nil {
		if !detached.admissionMu.TryLock() {
			s.bindMu.Unlock()
			http.Error(w, "observation owner changed", 409)
			return
		}
		if s.desktopObservationOwner(path) != owner {
			owner = nil
		}
	}
	var sub *control.DesktopEventSubscription
	if owner != nil && !s.sessionMirrored(path) && !s.runtimeForeignLeaseReadOnly(path) && admission.Err() == nil {
		sub, _ = owner.ObserveDesktopEvents(r.Context(), control.DesktopEventScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.Scope.RuntimeEpoch})
	}
	if detached != nil {
		detached.admissionMu.Unlock()
	}
	s.bindMu.Unlock()
	if sub == nil {
		http.Error(w, "observation requires a current owned session", 409)
		return
	}
	defer sub.Close()
	stop()
	cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	rc := http.NewResponseController(w)
	// A source retirement must also unblock a stalled HTTP write.
	writeCtx, finish := context.WithCancel(r.Context())
	defer finish()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		select {
		case <-sub.Done():
			_ = rc.SetWriteDeadline(time.Now())
		case <-writeCtx.Done():
		}
	}()
	defer func() { finish(); <-workerDone }()
	write := func(frame controller.SessionObservationFrame) bool {
		if rc.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		if json.NewEncoder(w).Encode(frame) != nil || rc.Flush() != nil {
			return false
		}
		return rc.SetWriteDeadline(time.Time{}) == nil
	}
	if !write(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: "ready"}) {
		return
	}
	for {
		frame, err := sub.Read(r.Context())
		if err != nil {
			return
		}
		current := s.desktopObservationCurrent(r.Context(), sub.Done(), path, owner)
		if !current || !write(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: frame.Kind}) {
			return
		}
	}
}
