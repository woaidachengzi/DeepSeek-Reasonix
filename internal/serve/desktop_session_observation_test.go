package serve

import (
	"context"
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

func TestDesktopObservationBusyGateCancellation(t *testing.T) {
	for _, retirement := range []bool{false, true} {
		t.Run(map[bool]string{false: "request_cancel", true: "source_retire"}[retirement], func(t *testing.T) {
			s := &Server{}
			s.bindMu.Lock()
			defer s.bindMu.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			retired := make(chan struct{})
			result := make(chan bool, 1)
			go func() { result <- s.desktopObservationCurrent(ctx, retired, "unused", nil) }()
			if retirement {
				close(retired)
			} else {
				cancel()
			}
			select {
			case current := <-result:
				if current {
					t.Fatal("cancelled observer remained current")
				}
			case <-time.After(time.Second):
				t.Fatal("observer cancellation waited for binding lock")
			}
		})
	}
}

func TestDesktopObservationActualAuthenticatedForegroundAndDetached(t *testing.T) {
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
				path = filepath.Join(f.dir, "observed-detached.jsonl")
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
			if detached {
				f.server.detachedMu.Lock()
				f.server.detached[canonical] = &detachedSession{path: canonical, ctrl: ctrl}
				f.server.detachedMu.Unlock()
			} else {
				f.server.mu.Lock()
				f.server.ctrl = ctrl
				f.server.mu.Unlock()
			}
			t.Cleanup(func() { ctrl.Cancel(); drivingWaitIdle(t, ctrl); ctrl.Close() })
			foreground := f.server.ctl()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			input := controller.SessionObservationRequest{ProtocolVersion: 1, Scope: controller.SessionPendingScope{SessionPath: canonical, RuntimeEpoch: ctrl.RuntimeStateSnapshot().RuntimeEpoch}}
			unauth := httptest.NewRecorder()
			f.server.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/desktop/session-observation", strings.NewReader("{}")))
			if unauth.Code != 401 {
				t.Fatal("observation bypassed authentication", unauth.Code)
			}
			wrong := input
			wrong.Scope.RuntimeEpoch += "-other"
			if stream, err := client.ObserveSession(ctx, wrong); err == nil {
				stream.Close()
				t.Fatal("wrong Controller epoch accepted")
			}
			wrong = input
			wrong.Scope.SessionPath = agent.CanonicalSessionPath(filepath.Join(f.dir, "saved-observation.jsonl"))
			saveServeTestSession(t, wrong.Scope.SessionPath)
			if stream, err := client.ObserveSession(ctx, wrong); err == nil {
				stream.Close()
				t.Fatal("historical session restored")
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return true }
			f.server.bindMu.Unlock()
			if stream, err := client.ObserveSession(ctx, input); err == nil {
				stream.Close()
				t.Fatal("foreign write owner observed")
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return false }
			f.server.bindMu.Unlock()
			if detached {
				f.server.detachedMu.Lock()
				ds := f.server.detached[canonical]
				f.server.detachedMu.Unlock()
				ds.admissionMu.Lock()
				blocked, err := client.ObserveSession(ctx, input)
				ds.admissionMu.Unlock()
				if err == nil {
					blocked.Close()
					t.Fatal("observer waited through retirement gate")
				}
			}
			stream, err := client.ObserveSession(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			// Actual HTTP driving admission publishes turn_started while holding
			// bindMu. A busy same-owner gate must not terminate this stream.
			f.server.bindMu.Lock()
			unlock := f.server.bindMu.Unlock
			defer func() {
				if unlock != nil {
					unlock()
				}
			}()
			ctrl.SubmitUserTurn("private observed body", "private display")
			type readResult struct {
				frame controller.SessionObservationFrame
				err   error
			}
			read := make(chan readResult, 1)
			go func() { frame, err := stream.Next(); read <- readResult{frame, err} }()
			select {
			case <-read:
				t.Fatal("busy same-owner gate prematurely ended or published observation")
			case <-time.After(100 * time.Millisecond):
			}
			unlock()
			unlock = nil
			result := <-read
			frame, err := result.frame, result.err
			if err != nil || frame.Kind != "turn_started" {
				t.Fatal(frame, err)
			}
			select {
			case <-p.calls:
				ctrl.Cancel()
			case <-ctx.Done():
				t.Fatal("actual Agent did not run")
			}
			frame, err = stream.Next()
			if err != nil || frame.Kind != "turn_done" {
				t.Fatal(frame, err)
			}
			drivingWaitIdle(t, ctrl)
			if f.server.ctl() != foreground {
				t.Fatal("observation changed foreground")
			}
			ctrl.SetSessionPath(path)
			if _, err := stream.Next(); err == nil {
				t.Fatal("retired source continued stream")
			}
		})
	}
}
