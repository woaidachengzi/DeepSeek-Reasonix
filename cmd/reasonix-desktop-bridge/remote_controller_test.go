package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/remote"
	"reasonix/internal/remote/controller"
	"reasonix/internal/remote/sshtest"
)

// Real SSH + direct-tcpip + real HTTP cookie exchange. Serve catalogue bodies
// are fixtures; this does not validate EnsureServe installation or real models.
func controllerFixture(t *testing.T, sessions http.HandlerFunc, views ...http.HandlerFunc) (*bridgeServer, *sshtest.Server, *remote.Client, *atomic.Int32) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, "profile"))
	srv := sshtest.Start(t, sshtest.Options{Password: "owned-password"})
	host, err := remote.ResolveHost(nil, "owned@"+srv.Addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := remote.New(remote.Options{
		Host: host, Auth: remote.AuthOptions{DisableAgent: true, Password: func() (string, error) { return "owned-password", nil }},
		HostKeys:  &remote.HostKeyPolicy{SystemKnownHosts: []string{filepath.Join(home, "no-system-keys")}, ManagedPath: filepath.Join(home, "known_hosts"), Prompt: func(context.Context, remote.HostKeyQuestion) (bool, error) { return true, nil }},
		Keepalive: remote.KeepalivePolicy{Interval: 50 * time.Millisecond, MaxMisses: 1, Timeout: 200 * time.Millisecond},
		Backoff:   remote.BackoffPolicy{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "owned-controller")
	t.Cleanup(bridge.remoteSessions.closeAll)
	attemptCtx, ticket, err := bridge.remoteSessions.begin("owned", context.Background())
	if err != nil || attemptCtx.Err() != nil {
		t.Fatal(err)
	}
	if !bridge.remoteSessions.putCurrent("owned", ticket, client, remote.HostKeyQuestion{}) {
		t.Fatal("SSH publication failed")
	}
	bridge.remoteSessions.finish("owned", ticket)
	var authCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			var body struct {
				Token string `json:"token"`
			}
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != "owned-controller-secret" {
				t.Error("bad backend handshake")
				w.WriteHeader(401)
				return
			}
			authCalls.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned-controller-secret", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err == nil && cookie.Value == "owned-controller-secret" && len(views) == 1 && r.URL.Path == "/desktop/session-image" {
			if r.Method != http.MethodPost || r.URL.RawQuery != "" {
				t.Error("bad session image channel")
			}
			views[0](w, r)
			return
		}
		if err == nil && cookie.Value == "owned-controller-secret" && len(views) == 1 && r.URL.Path == "/desktop/session-view" {
			if r.Method != http.MethodGet || len(r.URL.Query()) != 1 {
				t.Error("bad session view channel")
			}
			views[0](w, r)
			return
		}
		if err != nil || cookie.Value != "owned-controller-secret" || r.URL.Path != "/sessions" || r.URL.RawQuery != "" {
			t.Error("bad catalogue channel")
			w.WriteHeader(401)
			return
		}
		if sessions != nil {
			sessions(w, r)
			return
		}
		_, _ = io.WriteString(w, `[{"name":"remote-only","path":"/remote/session.jsonl","title":"远程对话","turns":2,"apiKey":"not-renderer-safe","token":"owned-controller-secret"}]`)
	}))
	t.Cleanup(server.Close)
	bridge.remoteControllerFactory = func(owner, operation context.Context, ssh *remote.Client, input remoteServeRequest) (*controller.Client, string, error) {
		bound, err := bridge.ensureRemoteServeForward(operation, input, ssh, strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			return nil, "", err
		}
		c, err := controller.Connect(owner, operation, "http://"+bound, "owned-controller-secret")
		return c, input.Workspace + "/resolved", err
	}
	return bridge, srv, client, &authCalls
}

func controllerCall(b *bridgeServer, method, target, body string, authorized bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authorized {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	response := httptest.NewRecorder()
	b.handler().ServeHTTP(response, req)
	return response
}

func attachController(t *testing.T, b *bridgeServer, workspace string) remoteControllerView {
	t.Helper()
	body, _ := json.Marshal(remoteControllerRequest{Name: "owned", Workspace: workspace})
	response := controllerCall(b, http.MethodPost, "/v1/remote/controllers", string(body), true)
	if response.Code != 200 {
		t.Fatalf("attach %d: %s", response.Code, response.Body.String())
	}
	var view remoteControllerResponse
	if json.Unmarshal(response.Body.Bytes(), &view) != nil || view.ProtocolVersion != 1 || !controllerHandle(view.Controller.ID) || !view.Controller.ReadOnly {
		t.Fatalf("invalid safe handle: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "owned-controller-secret") || strings.Contains(response.Body.String(), "http:") {
		t.Fatal("credential/endpoint reached response")
	}
	return view.Controller
}

func TestRemoteControllerCatalogueOwnedTunnelAndIsolation(t *testing.T) {
	b, _, _, auth := controllerFixture(t, nil)
	first := attachController(t, b, "/project-a")
	again := attachController(t, b, "/project-a")
	second := attachController(t, b, "/project-b")
	if first.ID != again.ID || first.ID == second.ID || auth.Load() != 2 {
		t.Fatal("reuse or workspace isolation failed")
	}
	response := controllerCall(b, "GET", "/v1/remote/controllers/"+first.ID+"/sessions", "", true)
	if response.Code != 200 {
		t.Fatalf("list %d: %s", response.Code, response.Body.String())
	}
	var catalogue remoteControllerSessionsResponse
	if json.Unmarshal(response.Body.Bytes(), &catalogue) != nil || len(catalogue.Sessions) != 1 || catalogue.Controller != first || catalogue.Sessions[0].Path != "/remote/session.jsonl" {
		t.Fatal("catalogue mismatch")
	}
	if strings.Contains(response.Body.String(), "not-renderer-safe") || strings.Contains(response.Body.String(), "owned-controller-secret") {
		t.Fatal("unknown private fields forwarded")
	}
	if _, present := b.runtimes.Snapshot(); present {
		t.Fatal("remote catalogue granted local runtime")
	}
	for i := 0; i < 2; i++ {
		if got := controllerCall(b, "DELETE", "/v1/remote/controllers/"+first.ID, "", true); got.Code != 200 {
			t.Fatal("close not idempotent")
		}
	}
	if got := controllerCall(b, "GET", "/v1/remote/controllers/"+first.ID+"/sessions", "", true); got.Code != 409 {
		t.Fatal("closed handle read succeeded")
	}
	if got := controllerCall(b, "GET", "/v1/remote/controllers/"+second.ID+"/sessions", "", true); got.Code != 200 {
		t.Fatal("closing one workspace revoked another")
	}
}

func TestRemoteControllerCatalogueGateAndCapacity(t *testing.T) {
	b, _, _, auth := controllerFixture(t, nil)
	for _, body := range []string{`{"name":"owned","workspace":"/p","url":"http://127.0.0.1:1"}`, `{"name":"owned","workspace":"/p","token":"secret"}`, `{"name":"owned","workspace":"/p","sessionPath":"/local"}`, `{"name":"owned","workspace":""}`, `{"name":"owned","workspace":"/p\nx"}`, `{"name":"owned","workspace":"/p","tailLines":1}`} {
		if got := controllerCall(b, "POST", "/v1/remote/controllers", body, true); got.Code != 400 {
			t.Fatalf("unsafe request status %d", got.Code)
		}
	}
	for _, route := range []struct{ method, path string }{{"POST", "/v1/remote/controllers"}, {"GET", "/v1/remote/controllers/" + strings.Repeat("a", 32) + "/sessions"}, {"DELETE", "/v1/remote/controllers/" + strings.Repeat("a", 32)}} {
		if got := controllerCall(b, route.method, route.path, `{"name":"owned","workspace":"/p"}`, false); got.Code != 401 {
			t.Fatal("unauthenticated route accepted")
		}
	}
	if auth.Load() != 0 {
		t.Fatal("validation/auth failure touched remote")
	}
	for i := 0; i < maxRemoteControllers; i++ {
		attachController(t, b, fmt.Sprintf("/p%d", i))
	}
	if got := controllerCall(b, "POST", "/v1/remote/controllers", `{"name":"owned","workspace":"/overflow"}`, true); got.Code != 409 || auth.Load() != maxRemoteControllers {
		t.Fatal("controller cap did not fail before network")
	}
}

func TestRemoteControllerCatalogueDisconnectCancelsBody(t *testing.T) {
	entered := make(chan struct{})
	released := make(chan struct{})
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		close(released)
	})
	view := attachController(t, b, "/p")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- controllerCall(b, "GET", "/v1/remote/controllers/"+view.ID+"/sessions", "", true) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not enter")
	}
	b.remoteSessions.disconnect("owned")
	select {
	case response := <-done:
		if response.Code != 409 {
			t.Fatalf("late read %d", response.Code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read hung after disconnect")
	}
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP body not released")
	}
	if len(b.remoteSessions.controllers) != 0 {
		t.Fatal("disconnect retained handles")
	}
}

func TestRemoteControllerCatalogueReconnectRevokesSameClientHandle(t *testing.T) {
	b, srv, client, _ := controllerFixture(t, nil)
	view := attachController(t, b, "/p")
	before, _ := client.SSH()
	reconnected := make(chan struct{}, 1)
	unsubscribe := client.Subscribe(func(event remote.StatusEvent) {
		if event.Status == remote.StatusConnected && event.Attempt > 0 {
			select {
			case reconnected <- struct{}{}:
			default:
			}
		}
	})
	defer unsubscribe()
	srv.DropConnections()
	select {
	case <-reconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not reconnect")
	}
	after, _ := client.SSH()
	if before == after {
		t.Fatal("fixture did not change physical SSH")
	}
	if got := controllerCall(b, "GET", "/v1/remote/controllers/"+view.ID+"/sessions", "", true); got.Code != 409 {
		t.Fatal("same client reused old handle after reconnect")
	}
	next := attachController(t, b, "/p")
	if next.ID == view.ID {
		t.Fatal("reconnect did not get new identity")
	}
}

func TestRemoteControllerCataloguePendingCannotResurrect(t *testing.T) {
	for _, mode := range []string{"disconnect", "shutdown", "newer", "stop_alias"} {
		t.Run(mode, func(t *testing.T) {
			b, _, _, _ := controllerFixture(t, nil)
			original := b.remoteControllerFactory
			entered := make(chan struct{})
			release := make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			var calls atomic.Int32
			b.remoteControllerFactory = func(owner, operation context.Context, client *remote.Client, input remoteServeRequest) (*controller.Client, string, error) {
				candidate, workspace, err := original(owner, operation, client, input)
				if calls.Add(1) == 1 {
					close(entered)
					<-release
				}
				return candidate, workspace, err
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- controllerCall(b, "POST", "/v1/remote/controllers", `{"name":"owned","workspace":"/p"}`, true)
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("attach not entered")
			}
			var next remoteControllerView
			switch mode {
			case "stop_alias":
				b.remoteSessions.mu.Lock()
				b.remoteSessions.revokePendingControllersLocked("owned")
				b.remoteSessions.revokeControllersLocked("owned", "/p/resolved")
				b.remoteSessions.mu.Unlock()
			case "disconnect":
				b.remoteSessions.disconnect("owned")
			case "shutdown":
				b.remoteSessions.closeAll()
			case "newer":
				next = attachController(t, b, "/p")
			}
			close(release)
			select {
			case response := <-done:
				if response.Code == 200 {
					t.Fatal("revoked attach published")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("late attach hung")
			}
			if mode == "newer" {
				if b.remoteSessions.getController(next.ID) == nil {
					t.Fatal("old completion revoked new handle")
				}
			} else if len(b.remoteSessions.controllers) != 0 || len(b.remoteSessions.controllerAttempts) != 0 {
				t.Fatal("revocation retained registry")
			}
		})
	}
}

func TestRemoteControllerCatalogueResolvedStopScopeAndShutdown(t *testing.T) {
	b, _, _, _ := controllerFixture(t, nil)
	first := attachController(t, b, "/p")
	other := attachController(t, b, "/q")
	b.remoteSessions.mu.Lock()
	b.remoteSessions.revokeControllersLocked("owned", first.Workspace)
	b.remoteSessions.mu.Unlock()
	if b.remoteSessions.getController(first.ID) != nil || b.remoteSessions.getController(other.ID) == nil {
		t.Fatal("resolved stop crossed workspace ownership")
	}
	remaining := b.remoteSessions.getController(other.ID)
	b.remoteSessions.closeAll()
	if !remaining.client.Closed() || b.remoteSessions.getController(other.ID) != nil {
		t.Fatal("shutdown retained controller")
	}
	if got := controllerCall(b, "POST", "/v1/remote/controllers", `{"name":"owned","workspace":"/q"}`, true); got.Code == 200 {
		t.Fatal("shutdown resurrected controller")
	}
}

func TestRemoteControllerAttachCancelsWhileServeHostIsLocked(t *testing.T) {
	b, _, client, _ := controllerFixture(t, nil)
	unlock := b.remoteServeMu.lock("owned")
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := b.makeRemoteController(context.Background(), ctx, client, remoteServeRequest{Name: "owned", Workspace: "/p"})
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, controller.ErrClosed) {
			t.Fatal("canceled host-lock wait succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled host-lock wait hung")
	}
	other, err := b.remoteServeMu.lockContext(context.Background(), "different-host")
	if err != nil {
		t.Fatal(err)
	}
	other()
}
