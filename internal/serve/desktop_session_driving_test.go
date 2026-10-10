package serve

import (
	"context"
	"errors"
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

func TestDesktopSessionDrivingActualAgentServeClient(t *testing.T) {
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
				path = filepath.Join(f.dir, "driving-detached.jsonl")
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
			foreground := f.server.ctl()
			t.Cleanup(func() { ctrl.Cancel(); drivingWaitIdle(t, ctrl); ctrl.Close() })
			state := ctrl.RuntimeStateSnapshot()
			request := controller.SessionDrivingRequest{ProtocolVersion: 1, Action: "capture", Scope: controller.SessionDrivingScope{SessionPendingScope: controller.SessionPendingScope{SessionPath: canonical, RuntimeEpoch: state.RuntimeEpoch}}}
			unauth := httptest.NewRecorder()
			f.server.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/desktop/session-driving", strings.NewReader("{}")))
			if unauth.Code != 401 {
				t.Fatal("driving bypassed authentication", unauth.Code)
			}
			wrong := request
			wrong.Scope.RuntimeEpoch += "-other"
			if _, err := client.SessionDriving(context.Background(), wrong); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("wrong instance captured", err)
			}
			wrong = request
			wrong.Scope.SessionPath = agent.CanonicalSessionPath(filepath.Join(f.dir, "saved-driving.jsonl"))
			saveServeTestSession(t, wrong.Scope.SessionPath)
			if _, err := client.SessionDriving(context.Background(), wrong); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("historical owner adopted", err)
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return true }
			f.server.bindMu.Unlock()
			if _, err := client.SessionDriving(context.Background(), request); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("foreign writer captured", err)
			}
			f.server.bindMu.Lock()
			f.server.runtimeLeaseProbe = func(string) bool { return false }
			f.server.bindMu.Unlock()
			if ds != nil {
				ds.admissionMu.Lock()
				_, err := client.SessionDriving(context.Background(), request)
				ds.admissionMu.Unlock()
				if !errors.Is(err, controller.ErrDrivingChanged) {
					t.Fatal("queued behind retirement gate", err)
				}
				f.server.detachedMu.Lock()
				ds.retiring = true
				f.server.detachedMu.Unlock()
				_, err = client.SessionDriving(context.Background(), request)
				f.server.detachedMu.Lock()
				ds.retiring = false
				f.server.detachedMu.Unlock()
				if !errors.Is(err, controller.ErrDrivingChanged) {
					t.Fatal("retiring owner captured", err)
				}
			}
			capture, err := client.SessionDriving(context.Background(), request)
			if err != nil || ctrl.RuntimeStateSnapshot() != state {
				t.Fatal("capture changed runtime", err)
			}
			request.Scope = capture.Scope
			request.Action = "acquire"
			request.Key = "0123456789abcdef0123456789abcdef"
			if _, err := client.SessionDriving(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			other := request
			other.Key = "1123456789abcdef0123456789abcdef"
			if _, err := client.SessionDriving(context.Background(), other); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("other holder stole authority", err)
			}
			request.Action = "input"
			request.InputRevision = state.Revision
			request.Text = "/new"
			accepted, err := client.SessionDriving(context.Background(), request)
			if err != nil || !accepted.Accepted {
				t.Fatal("literal turn not admitted", err)
			}
			var call submitProviderCall
			select {
			case call = <-p.calls:
			case <-time.After(5 * time.Second):
				t.Fatal("actual provider not called")
			}
			found := false
			for _, message := range call.request.Messages {
				if message.Content == "/new" {
					found = true
				}
			}
			if !found || ctrl.SessionPath() != path || f.server.ctl() != foreground {
				t.Fatal("driving changed owner or dispatched slash command")
			}
			if _, err := client.SessionDriving(context.Background(), request); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("duplicate turn admitted", err)
			}
			request.Action = "state"
			request.InputRevision = 0
			request.Text = ""
			active, err := client.SessionDriving(context.Background(), request)
			if err != nil || !active.Active {
				t.Fatal("running holder not observable", err)
			}
			request.Action = "release"
			if _, err := client.SessionDriving(context.Background(), request); err != nil || call.ctx.Err() != nil {
				t.Fatal("release canceled accepted turn", err)
			}
			if _, err := client.SessionDriving(context.Background(), request); err != nil {
				t.Fatal("spent release not idempotent", err)
			}
			ctrl.Cancel()
			drivingWaitIdle(t, ctrl)
			request.Action = "capture"
			request.Scope.Revision = 0
			request.Scope.ControlVersion = 0
			request.Key = ""
			capture, err = client.SessionDriving(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.Scope = capture.Scope
			request.Action = "acquire"
			request.Key = other.Key
			if _, err := client.SessionDriving(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			ctrl.TrySteer("local input reclaims even while idle")
			request.Action = "state"
			active, err = client.SessionDriving(context.Background(), request)
			if err != nil || active.Active {
				t.Fatal("local input did not revoke holder", err)
			}
			request.Action = "acquire"
			if _, err := client.SessionDriving(context.Background(), request); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("old capture reacquired after local input", err)
			}
			request.Action = "input"
			request.InputRevision = ctrl.RuntimeStateSnapshot().Revision
			request.Text = "must not run"
			if _, err := client.SessionDriving(context.Background(), request); !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("fresh revision bypassed local reclaim", err)
			}
			select {
			case <-p.calls:
				t.Fatal("revoked input reached provider")
			default:
			}
			// Replacing a Controller at exactly the same path is not adoption.
			replacementExec := agent.New(nil, tool.NewRegistry(), agent.NewSession("fixture"), agent.Options{}, event.Discard)
			replacement := control.New(control.Options{Runner: replacementExec, Executor: replacementExec, SessionDir: f.dir, SessionPath: path, Sink: event.Discard})
			t.Cleanup(replacement.Close)
			if ds != nil {
				f.server.detachedMu.Lock()
				ds.ctrl = replacement
				f.server.detachedMu.Unlock()
			} else {
				f.server.mu.Lock()
				f.server.ctrl = replacement
				f.server.mu.Unlock()
			}
			request.Action = "state"
			request.InputRevision = 0
			request.Text = ""
			_, err = client.SessionDriving(context.Background(), request)
			if ds != nil {
				f.server.detachedMu.Lock()
				ds.ctrl = ctrl
				f.server.detachedMu.Unlock()
			} else {
				f.server.mu.Lock()
				f.server.ctrl = ctrl
				f.server.mu.Unlock()
			}
			if !errors.Is(err, controller.ErrDrivingChanged) {
				t.Fatal("same-path replacement adopted original grant", err)
			}
			// A fresh explicit capture/key is allowed after local reclaim. Closing
			// the transport after admission still does not cancel its Agent turn.
			request.Action = "capture"
			request.Scope.Revision = 0
			request.Scope.ControlVersion = 0
			request.Key = ""
			capture, err = client.SessionDriving(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.Scope = capture.Scope
			request.Action = "acquire"
			request.Key = "2123456789abcdef0123456789abcdef"
			if _, err := client.SessionDriving(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			request.Action = "input"
			request.InputRevision = capture.Scope.Revision
			request.Text = "!echo literal-not-shell"
			if _, err := client.SessionDriving(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			select {
			case call = <-p.calls:
			case <-time.After(5 * time.Second):
				t.Fatal("second actual turn not started")
			}
			client.Close()
			if call.ctx.Err() != nil {
				t.Fatal("transport close canceled accepted Agent turn")
			}
		})
	}
}

func drivingWaitIdle(t *testing.T, ctrl *control.Controller) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := ctrl.RuntimeStateSnapshot()
		if !ctrl.Running() && !state.Running && state.Phase == "idle" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("actual Controller did not commit idle state")
}

func TestDesktopSessionDrivingStrictBody(t *testing.T) {
	closeDesktopReadTestUsage(t)
	f, _ := desktopViewFixture(t)
	t.Cleanup(func() { closeDesktopReadTestUsage(t) })
	for _, body := range []string{
		`{"protocolVersion":1,"action":"capture","scope":{"sessionPath":"x","runtimeEpoch":"e","revision":0}}`,
		`{"protocolVersion":1,"action":"capture","scope":{"sessionPath":"x","runtimeEpoch":"e","revision":0,"controlVersion":null}}`,
		`{"protocolVersion":1,"action":"capture","scope":{"sessionPath":"x","runtimeEpoch":"e","revision":0,"controlVersion":0,"extra":1}}`,
		`{"protocolVersion":1,"action":"capture","scope":{"sessionPath":"x","runtimeEpoch":"e","revision":0,"controlVersion":0},"extra":1}`,
		`{} {}`,
	} {
		w := httptest.NewRecorder()
		f.server.desktopSessionDriving(w, httptest.NewRequest(http.MethodPost, "/desktop/session-driving", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal("invalid body accepted", w.Code)
		}
	}
}
