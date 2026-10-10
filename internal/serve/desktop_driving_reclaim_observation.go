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

func (s *Server) desktopDrivingReclaimObservation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	admission, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	stop := context.AfterFunc(admission, func() { _ = r.Body.Close() })
	defer cancel()
	defer stop()
	var input controller.DrivingReclaimObservationRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if r.URL.RawQuery != "" || r.Header.Get("Last-Event-ID") != "" || d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || !controller.ValidDrivingReclaimObservationRequest(input) {
		http.Error(w, "select one original driving observation scope", 400)
		return
	}
	if admission.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "driving observation owner changed", 409)
		return
	}
	path := input.Scope.SessionPath
	owner := s.desktopObservationOwner(path)
	var detached *detachedSession
	if foreground := s.ctl(); foreground == nil || agent.CanonicalSessionPath(foreground.SessionPath()) != path {
		s.detachedMu.Lock()
		detached = s.detached[path]
		s.detachedMu.Unlock()
	}
	if detached != nil {
		if !detached.admissionMu.TryLock() {
			s.bindMu.Unlock()
			http.Error(w, "driving observation owner changed", 409)
			return
		}
		if s.desktopObservationOwner(path) != owner {
			owner = nil
		}
	}
	var observation *control.DesktopDrivingReclaimObservation
	if owner != nil && !s.sessionMirrored(path) && !s.runtimeForeignLeaseReadOnly(path) && admission.Err() == nil {
		scope := control.DesktopDrivingScope{TurnSubmitScope: control.TurnSubmitScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.Scope.RuntimeEpoch, Revision: input.Scope.Revision}, ControlVersion: input.Scope.ControlVersion}
		observation, _ = owner.ObserveDesktopDrivingReclaim(r.Context(), scope, input.Key)
	}
	if detached != nil {
		detached.admissionMu.Unlock()
	}
	s.bindMu.Unlock()
	if observation == nil {
		http.Error(w, "driving observation requires the original active grant", 409)
		return
	}
	defer observation.Close()
	stop()
	cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	rc := http.NewResponseController(w)
	writeCtx, finish := context.WithCancel(r.Context())
	defer finish()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		select {
		case <-observation.Retired():
			_ = rc.SetWriteDeadline(time.Now())
		case <-writeCtx.Done():
		}
	}()
	defer func() { finish(); <-workerDone }()
	write := func(kind string) bool {
		if rc.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		if json.NewEncoder(w).Encode(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: kind}) != nil || rc.Flush() != nil {
			return false
		}
		return rc.SetWriteDeadline(time.Time{}) == nil
	}
	if !write("ready") {
		return
	}
	select {
	case <-r.Context().Done():
		return
	case <-observation.Done():
	}
	if !observation.Reclaimed() {
		return
	}
	current := s.desktopObservationCurrent(r.Context(), observation.Retired(), path, owner)
	select {
	case <-observation.Retired():
		return
	default:
	}
	if !current || !write("reclaimed") {
		return
	}
	// Keep the source lifetime visible after its one signal. The client can
	// continue reading while SDK IO runs; retirement then cancels that send.
	select {
	case <-r.Context().Done():
	case <-observation.Retired():
	}
}
