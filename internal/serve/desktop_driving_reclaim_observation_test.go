package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/remote/controller"
	"reasonix/internal/tool"
)

func TestDrivingReclaimObservationActualAgentServeAndOriginalGrant(t *testing.T) {
	for _, detached := range []bool{false, true} {
		name := "foreground"
		if detached {
			name = "detached"
		}
		t.Run(name, func(t *testing.T) {
			closeDesktopReadTestUsage(t)
			f, client := desktopViewFixture(t)
			t.Cleanup(func() { closeDesktopReadTestUsage(t) })
			path := f.active
			if detached {
				path = filepath.Join(f.dir, "reclaim-detached.jsonl")
				saveServeTestSession(t, path)
			} else {
				f.server.ctl().Close()
			}
			session, err := agent.LoadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			p := &submitGateProvider{calls: make(chan submitProviderCall, 4)}
			exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
			ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: f.dir, SessionPath: path, Sink: event.Discard})
			canonical := agent.CanonicalSessionPath(path)
			var ds *detachedSession
			if detached {
				ds = &detachedSession{path: canonical, ctrl: ctrl}
				f.server.detachedMu.Lock()
				f.server.detached[canonical] = ds
				f.server.detachedMu.Unlock()
			} else {
				f.server.mu.Lock()
				f.server.ctrl = ctrl
				f.server.mu.Unlock()
			}
			t.Cleanup(func() { ctrl.Cancel(); drivingWaitIdle(t, ctrl); ctrl.Close() })
			foreground := f.server.ctl()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			request := controller.SessionDrivingRequest{ProtocolVersion: 1, Action: "capture", Scope: controller.SessionDrivingScope{SessionPendingScope: controller.SessionPendingScope{SessionPath: canonical, RuntimeEpoch: ctrl.RuntimeStateSnapshot().RuntimeEpoch}}}
			capture, err := client.SessionDriving(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			request.Scope = capture.Scope
			request.Action = "acquire"
			request.Key = "0123456789abcdef0123456789abcdef"
			if _, err := client.SessionDriving(ctx, request); err != nil {
				t.Fatal(err)
			}
			input := controller.DrivingReclaimObservationRequest{ProtocolVersion: 1, Scope: request.Scope, Key: request.Key}
			unauth := httptest.NewRecorder()
			f.server.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/desktop/driving-reclaim-observation", strings.NewReader("{}")))
			if unauth.Code != 401 {
				t.Fatal("reclaim stream bypassed auth")
			}
			validBody, _ := json.Marshal(input)
			for _, bad := range []struct{ target, body, last string }{
				{"/desktop/driving-reclaim-observation?replay=1", string(validBody), ""},
				{"/desktop/driving-reclaim-observation", string(validBody), "old-event"},
				{"/desktop/driving-reclaim-observation", strings.TrimSuffix(string(validBody), "}") + `,"text":"private-input"}`, ""},
				{"/desktop/driving-reclaim-observation", string(validBody) + `{}`, ""},
				{"/desktop/driving-reclaim-observation", strings.Repeat("x", (64<<10)+1), ""},
			} {
				req := httptest.NewRequest(http.MethodPost, bad.target, strings.NewReader(bad.body))
				req.Host = "127.0.0.1"
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: cookieToken, Value: "owned-view-token"})
				req.Header.Set("Last-Event-ID", bad.last)
				response := httptest.NewRecorder()
				f.server.Handler().ServeHTTP(response, req)
				if response.Code != 400 {
					t.Fatal("malformed/replay request accepted", response.Code)
				}
			}
			wrong := input
			wrong.Key = "1123456789abcdef0123456789abcdef"
			if s, err := client.ObserveDrivingReclaim(ctx, wrong); err == nil {
				s.Close()
				t.Fatal("another key observed original grant")
			}
			wrong = input
			wrong.Scope.RuntimeEpoch += "-other"
			if s, err := client.ObserveDrivingReclaim(ctx, wrong); err == nil {
				s.Close()
				t.Fatal("another instance adopted original grant")
			}
			wrong = input
			wrong.Scope.SessionPath = agent.CanonicalSessionPath(filepath.Join(f.dir, "saved-reclaim.jsonl"))
			saveServeTestSession(t, wrong.Scope.SessionPath)
			if s, err := client.ObserveDrivingReclaim(ctx, wrong); err == nil {
				s.Close()
				t.Fatal("saved owner restored for observation")
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return true }
			f.server.bindMu.Unlock()
			if s, err := client.ObserveDrivingReclaim(ctx, input); err == nil {
				s.Close()
				t.Fatal("foreign writer observed")
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return false }
			f.server.bindMu.Unlock()
			if ds != nil {
				ds.admissionMu.Lock()
				s, err := client.ObserveDrivingReclaim(ctx, input)
				ds.admissionMu.Unlock()
				if err == nil {
					s.Close()
					t.Fatal("retirement admission gate bypassed")
				}
			}
			s, err := client.ObserveDrivingReclaim(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			s.Close()
			request.Action = "state"
			active, err := client.SessionDriving(ctx, request)
			if err != nil || !active.Active {
				t.Fatal("closing observation released driving", err)
			}
			request.Action = "input"
			request.InputRevision = ctrl.RuntimeStateSnapshot().Revision
			request.Text = "private accepted remote question"
			if _, err := client.SessionDriving(ctx, request); err != nil {
				t.Fatal(err)
			}
			var call submitProviderCall
			select {
			case call = <-p.calls:
			case <-ctx.Done():
				t.Fatal("actual Agent not called")
			}
			// Original capture is still valid for observation during its own turn.
			s, err = client.ObserveDrivingReclaim(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			f.server.bindMu.Lock()
			unlock := f.server.bindMu.Unlock
			defer func() {
				if unlock != nil {
					unlock()
				}
			}()
			ctrl.TrySteer("private local guidance")
			type readResult struct {
				frame controller.SessionObservationFrame
				err   error
			}
			read := make(chan readResult, 1)
			go func() { frame, err := s.Next(); read <- readResult{frame, err} }()
			select {
			case <-read:
				t.Fatal("busy same-owner gate prematurely ended or published reclaim")
			case <-time.After(100 * time.Millisecond):
			}
			unlock()
			unlock = nil
			result := <-read
			frame, err := result.frame, result.err
			if err != nil || frame.Kind != "reclaimed" {
				t.Fatal(frame, err)
			}
			raw, _ := json.Marshal(frame)
			if string(raw) != `{"protocolVersion":1,"kind":"reclaimed"}` || call.ctx.Err() != nil || f.server.ctl() != foreground {
				t.Fatal("reclaim leaked data, cancelled accepted Agent or changed foreground")
			}
			if replay, err := client.ObserveDrivingReclaim(ctx, input); err == nil {
				replay.Close()
				t.Fatal("spent grant replayed reclaim")
			}
			request.Action = "release"
			request.InputRevision = 0
			request.Text = ""
			if _, err := client.SessionDriving(ctx, request); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Next(); err == nil || call.ctx.Err() != nil {
				t.Fatal("retirement produced another signal or cancelled task", err)
			}
			ctrl.Cancel()
			drivingWaitIdle(t, ctrl)
			request.Action = "capture"
			request.Key = ""
			request.Scope.Revision = 0
			request.Scope.ControlVersion = 0
			capture, err = client.SessionDriving(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			request.Action = "acquire"
			request.Scope = capture.Scope
			request.Key = "1123456789abcdef0123456789abcdef"
			if _, err := client.SessionDriving(ctx, request); err != nil {
				t.Fatal(err)
			}
			input.Scope = request.Scope
			input.Key = request.Key
			s, err = client.ObserveDrivingReclaim(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctrl.SetSessionPath(path)
			if _, err := s.Next(); err == nil {
				t.Fatal("same-path rebind misreported local input")
			}
		})
	}
}
