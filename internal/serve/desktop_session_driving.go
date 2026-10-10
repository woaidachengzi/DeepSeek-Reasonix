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

func (s *Server) desktopSessionDriving(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	var input controller.SessionDrivingRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if r.URL.RawQuery != "" || d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || !controller.ValidSessionDrivingRequest(input) {
		http.Error(w, "select one current session driving scope", 400)
		return
	}
	if ctx.Err() != nil || !s.bindMu.TryLock() {
		http.Error(w, "remote driving authority changed", 409)
		return
	}
	defer s.bindMu.Unlock()
	// Published owners only: never resolve a historical path, restore a session,
	// or probe/clean another runtime's write lock as a side effect of a read.
	path := input.Scope.SessionPath
	var owner *control.Controller
	var detachedOwner *detachedSession
	if foreground := s.ctl(); foreground != nil && agent.CanonicalSessionPath(foreground.SessionPath()) == path {
		owner, _ = foreground.(*control.Controller)
	} else {
		s.detachedMu.Lock()
		if detached := s.detached[path]; detached != nil && !detached.retiring {
			owner, _ = detached.ctrl.(*control.Controller)
			detachedOwner = detached
		}
		s.detachedMu.Unlock()
	}
	if detachedOwner != nil {
		if !detachedOwner.admissionMu.TryLock() {
			http.Error(w, "remote driving authority changed", 409)
			return
		}
		defer detachedOwner.admissionMu.Unlock()
		s.detachedMu.Lock()
		valid := s.detached[path] == detachedOwner && !detachedOwner.retiring && detachedOwner.ctrl == owner
		s.detachedMu.Unlock()
		if !valid {
			owner = nil
		}
	}
	if owner == nil || agent.CanonicalSessionPath(owner.SessionPath()) != path || s.sessionMirrored(path) || s.runtimeForeignLeaseReadOnly(path) || ctx.Err() != nil {
		http.Error(w, "remote driving requires a current owned session", 409)
		return
	}
	scope := control.DesktopDrivingScope{TurnSubmitScope: control.TurnSubmitScope{SessionPath: owner.SessionPath(), RuntimeEpoch: input.Scope.RuntimeEpoch, Revision: input.Scope.Revision}, ControlVersion: input.Scope.ControlVersion}
	result := controller.SessionDrivingReceipt{ProtocolVersion: 1, Action: input.Action, Scope: input.Scope}
	var err error
	switch input.Action {
	case "capture":
		var captured control.DesktopDrivingScope
		captured, err = owner.CaptureDesktopDriving(ctx)
		if err == nil && (captured.RuntimeEpoch != input.Scope.RuntimeEpoch || agent.CanonicalSessionPath(captured.SessionPath) != path) {
			err = control.ErrDesktopDrivingScope
		}
		if err == nil {
			result.Scope.Revision = captured.Revision
			result.Scope.ControlVersion = captured.ControlVersion
		}
	case "acquire":
		err = owner.AcquireDesktopDriving(ctx, scope, input.Key)
		result.Active = err == nil
	case "state":
		// Unlike capture, state can observe an original holder while running or
		// after local reclaim; never sample/adopt a replacement control version.
		state := owner.RuntimeStateSnapshot()
		if state.RuntimeEpoch != input.Scope.RuntimeEpoch || state.Phase == "closed" {
			err = control.ErrDesktopDrivingScope
		} else {
			result.Active = owner.DesktopDrivingActive(ctx, scope, input.Key)
		}
	case "release":
		err = owner.ReleaseDesktopDriving(ctx, scope, input.Key)
		result.Released = err == nil
	case "input":
		if !owner.DesktopDrivingActive(ctx, scope, input.Key) {
			err = control.ErrDesktopDrivingScope
		} else {
			current := scope.TurnSubmitScope
			current.Revision = input.InputRevision
			err = owner.SubmitDesktopDriving(ctx, current, input.Key, input.Text)
		}
		result.Accepted = err == nil
	}
	if err != nil {
		http.Error(w, "remote driving authority changed", 409)
		return
	}
	if ctx.Err() != nil {
		// A mutation may already have been admitted. Never report a safe retry.
		http.Error(w, "remote driving outcome unknown", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
