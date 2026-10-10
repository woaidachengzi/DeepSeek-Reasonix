package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/remote/controller"
)

type remoteDrivingFixture struct {
	mu                         sync.Mutex
	catalogue                  *previewDesktopCatalogue
	bridge                     *bridgeServer
	driver                     *previewDesktopRemoteDriver
	entry                      previewDesktopCatalogueEntry
	route                      bot.DesktopWatchRoute
	revision, version          uint64
	epoch, holder, fail, block string
	grant                      controller.SessionDrivingScope
	requests                   chan controller.SessionDrivingRequest
}

func remoteDrivingTestFixture(t *testing.T) *remoteDrivingFixture {
	t.Helper()
	f := &remoteDrivingFixture{route: desktopWatchTestRoute(), revision: 1, epoch: "captured-/remote/live.jsonl", requests: make(chan controller.SessionDrivingRequest, 128)}
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("driving scanned history"); w.WriteHeader(400) }, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/desktop/session-observation" {
			var input controller.SessionObservationRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || !controller.ValidSessionObservationRequest(input) || input.Scope.SessionPath != "/remote/live.jsonl" {
				t.Error("invalid fixture observation scope")
				w.WriteHeader(400)
				return
			}
			f.mu.Lock()
			valid := input.Scope.RuntimeEpoch == f.epoch
			f.mu.Unlock()
			if !valid {
				w.WriteHeader(409)
				return
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			_ = json.NewEncoder(w).Encode(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: "ready"})
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/runtime-states" {
			f.mu.Lock()
			states := catalogueFixtureStates([]controller.Session{{Path: "/remote/live.jsonl"}}, strings.TrimSuffix(f.epoch, "-/remote/live.jsonl"))
			states.Sessions[0].State.Revision = f.revision
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(states)
			return
		}
		if r.URL.Path != "/desktop/session-driving" {
			t.Error("legacy or local driving fallback")
			w.WriteHeader(400)
			return
		}
		var input controller.SessionDrivingRequest
		if json.NewDecoder(r.Body).Decode(&input) != nil || !controller.ValidSessionDrivingRequest(input) {
			t.Error("invalid generated driving request")
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		out := controller.SessionDrivingReceipt{ProtocolVersion: 1, Action: input.Action, Scope: input.Scope}
		valid := input.Scope.SessionPath == "/remote/live.jsonl" && input.Scope.RuntimeEpoch == f.epoch
		if valid {
			switch input.Action {
			case "capture":
				out.Scope.Revision = f.revision
				out.Scope.ControlVersion = f.version
			case "acquire":
				valid = f.holder == "" && input.Scope.Revision == f.revision && input.Scope.ControlVersion == f.version
				if valid {
					f.holder = input.Key
					f.grant = input.Scope
					out.Active = true
				}
			case "state":
				out.Active = f.holder == input.Key && f.grant == input.Scope
			case "input":
				valid = f.holder == input.Key && f.grant == input.Scope && input.InputRevision == f.revision && input.Scope.ControlVersion == f.version
				if valid {
					out.Accepted = true
					f.revision++
				}
			case "release":
				valid = input.Scope == f.grant
				if valid {
					f.holder = ""
					f.version++
					out.Released = true
				}
			}
		}
		fail, block := f.fail == input.Action, f.block == input.Action
		f.mu.Unlock()
		f.requests <- input
		if block {
			<-r.Context().Done()
			return
		}
		if !valid {
			w.WriteHeader(409)
			return
		}
		if fail {
			http.Error(w, "private provider path and API key", 502)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	attachController(t, b, "/owned-remote")
	f.bridge = b
	f.catalogue = newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
	t.Cleanup(f.catalogue.Close)
	entries, err := f.catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	f.entry = entries[0]
	f.driver = newPreviewDesktopRemoteDriver(f.catalogue)
	t.Cleanup(f.driver.Close)
	return f
}

func (f *remoteDrivingFixture) command(action string) bot.DesktopCommand {
	c := bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: action}
	if action == "takeover" {
		c.TargetID = f.entry.handle
	}
	if action == "drive" {
		c.AnswerText = "!literal input"
	}
	return c
}

func (f *remoteDrivingFixture) take(t *testing.T) {
	t.Helper()
	out, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("takeover"))
	if err != nil || !f.driver.DesktopTakeoverActive(f.route, "operator") || strings.Contains(out, "/remote/") {
		t.Fatal(out, err)
	}
	capture := notificationWait(t, f.requests)
	acquire := notificationWait(t, f.requests)
	if capture.Action != "capture" || acquire.Action != "acquire" || len(acquire.Key) != 32 || strings.Contains(out, acquire.Key) {
		t.Fatal("grant identity leaked or changed")
	}
}

func TestPreviewRemoteDrivingExactOwnerRouteActorAndFreshTurnRevision(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	f.take(t)
	for _, mutate := range []func(*bot.DesktopCommand){
		func(c *bot.DesktopCommand) { c.ActorID = "another-admin" },
		func(c *bot.DesktopCommand) { c.Route.ChatID += "-other" },
		func(c *bot.DesktopCommand) { c.Route.ConnectionID += "-other" },
		func(c *bot.DesktopCommand) { c.Route.Domain += "-other" },
		func(c *bot.DesktopCommand) { c.Route.ChatType = bot.ChatThread },
	} {
		wrong := f.command("drive")
		mutate(&wrong)
		if _, err := f.driver.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
			t.Fatal("foreign actor/chat drove")
		}
		if f.driver.DesktopTakeoverActive(wrong.Route, wrong.ActorID) {
			t.Fatal("foreign actor/chat inherited routing")
		}
		wrong.Action = "release"
		wrong.AnswerText = ""
		_, _ = f.driver.ExecuteDesktopCommand(context.Background(), wrong)
		if !f.driver.DesktopTakeoverActive(f.route, "operator") {
			t.Fatal("foreign release revoked original actor")
		}
		wrong.Action = "takeover"
		wrong.TargetID = f.entry.handle
		wrong.AnswerText = ""
		if _, err := f.driver.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
			t.Fatal("foreign route stole holder")
		}
	}
	var originalKey string
	for n := uint64(1); n <= 2; n++ {
		if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("drive")); err != nil {
			t.Fatal(err)
		}
		state, capture, input := notificationWait(t, f.requests), notificationWait(t, f.requests), notificationWait(t, f.requests)
		if n == 1 {
			originalKey = input.Key
		}
		if state.Action != "state" || capture.Action != "capture" || input.Action != "input" || input.Scope.Revision != 1 || input.InputRevision != n || input.Key != originalKey || input.Text != "!literal input" {
			t.Fatal("new turn adopted a different grant", input)
		}
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal(err)
	}
	release := notificationWait(t, f.requests)
	if release.Action != "release" || release.Key != originalKey || f.driver.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("release changed holder")
	}
}

func TestPreviewRemoteDrivingLocalReclaimAndSamePathReplacement(t *testing.T) {
	for _, mode := range []string{"reclaim", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			f.take(t)
			f.mu.Lock()
			if mode == "reclaim" {
				f.holder = ""
				f.version++
			} else {
				f.epoch = "replacement-/remote/live.jsonl"
			}
			f.mu.Unlock()
			if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("drive")); err != errPreviewDesktopBinding {
				t.Fatal(err)
			}
			if request := notificationWait(t, f.requests); request.Action != "state" {
				t.Fatal("revoked input was dispatched")
			}
			if f.driver.DesktopTakeoverActive(f.route, "operator") {
				t.Fatal("old binding survived reclaim")
			}
			select {
			case request := <-f.requests:
				t.Fatal("fallback/reacquire after revoke", request.Action)
			default:
			}
		})
	}
}

func TestPreviewRemoteDrivingUnknownAcquireAndInputNeverRearm(t *testing.T) {
	for _, action := range []string{"acquire", "input"} {
		t.Run(action, func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			if action == "input" {
				f.take(t)
			}
			f.mu.Lock()
			f.fail = action
			f.mu.Unlock()
			command := f.command("takeover")
			if action == "input" {
				command = f.command("drive")
			}
			if out, err := f.driver.ExecuteDesktopCommand(context.Background(), command); err != errPreviewDesktopBinding || out != "" {
				t.Fatal("unknown leaked diagnostic", out, err)
			}
			for len(f.requests) > 0 {
				<-f.requests
			}
			if !f.driver.DesktopTakeoverActive(f.route, "operator") {
				t.Fatal("unknown silently routed to ordinary bot")
			}
			for _, next := range []string{"takeover", "drive"} {
				if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command(next)); err == nil {
					t.Fatal("unknown mutation rearmed")
				}
			}
			select {
			case request := <-f.requests:
				t.Fatal("unknown retried", request.Action)
			default:
			}
			if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
				t.Fatal("unknown grant could not be explicitly released", err)
			}
			if request := notificationWait(t, f.requests); request.Action != "release" {
				t.Fatal("release recaptured grant")
			}
		})
	}
}

func TestPreviewRemoteDrivingReleaseCancelsReservedInputAndCloseCancelsAcquire(t *testing.T) {
	for _, action := range []string{"input", "acquire"} {
		t.Run(action, func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			if action == "input" {
				f.take(t)
			}
			f.mu.Lock()
			f.block = action
			f.mu.Unlock()
			command := f.command("takeover")
			if action == "input" {
				command = f.command("drive")
			}
			finished := make(chan error, 1)
			go func() { _, err := f.driver.ExecuteDesktopCommand(context.Background(), command); finished <- err }()
			for {
				if notificationWait(t, f.requests).Action == action {
					break
				}
			}
			if _, err := f.driver.ExecuteDesktopCommand(context.Background(), command); err == nil {
				t.Fatal("concurrent duplicate admitted")
			}
			if action == "input" {
				if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
					t.Fatal("release blocked on operation mutex", err)
				}
				if request := notificationWait(t, f.requests); request.Action != "release" {
					t.Fatal("release replayed input")
				}
			} else {
				f.driver.Close()
			}
			if err := notificationWait(t, finished); err != errPreviewDesktopBinding {
				t.Fatal("in-flight operation survived retirement", err)
			}
			if f.driver.DesktopTakeoverActive(f.route, "operator") {
				t.Fatal("retired binding routed input")
			}
		})
	}
}

func TestPreviewRemoteDrivingTTLDoesNotRenewOrRetry(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	f.take(t)
	f.driver.mu.Lock()
	f.driver.bindings[f.route].started = time.Now().Add(-16 * time.Minute)
	f.driver.mu.Unlock()
	if f.driver.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("expired host lease survived")
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("drive")); err == nil {
		t.Fatal("expired binding drove")
	}
	select {
	case request := <-f.requests:
		t.Fatal("expiry renewed or retried", request.Action)
	default:
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal("earlier host expiry lost original release key", err)
	}
	if request := notificationWait(t, f.requests); request.Action != "release" {
		t.Fatal("expired release reacquired authority")
	}
}

func TestPreviewRemoteDrivingCloseDuringReleaseCannotConfirmRevocation(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	f.take(t)
	f.mu.Lock()
	f.block = "release"
	f.mu.Unlock()
	finished := make(chan error, 1)
	go func() {
		out, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release"))
		if out != "" {
			t.Error("close fabricated release confirmation")
		}
		finished <- err
	}()
	if request := notificationWait(t, f.requests); request.Action != "release" {
		t.Fatal("release changed key operation")
	}
	f.driver.Close()
	if err := notificationWait(t, finished); err != errPreviewDesktopBinding {
		t.Fatal("canceled release reported confirmed", err)
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("release")); err != errPreviewDesktopBinding {
		t.Fatal("closed host confirmed remote revocation", err)
	}
}

func TestPreviewRemoteDrivingActualTunnelReplacementAndDisconnect(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "replacement"}[replace], func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			f.take(t)
			if replace {
				// Same-scope attach intentionally reuses a live tunnel. Retire it
				// first so this assertion covers an actual replacement object.
				f.catalogue.remotes.closeController(f.entry.remote.view.ID)
				attachController(t, f.bridge, "/owned-remote")
				if f.catalogue.remotes.getController(f.entry.remote.view.ID) == f.entry.remote {
					t.Fatal("fixture did not replace tunnel")
				}
			} else {
				f.catalogue.remotes.closeController(f.entry.remote.view.ID)
			}
			if f.driver.DesktopTakeoverActive(f.route, "operator") {
				t.Fatal("old binding adopted replacement tunnel")
			}
			if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("drive")); err == nil {
				t.Fatal("retired original tunnel drove")
			}
			if _, err := f.driver.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err == nil {
				t.Fatal("old opaque handle captured replacement")
			}
			select {
			case request := <-f.requests:
				t.Fatal("retirement recaptured or dispatched", request.Action)
			default:
			}
		})
	}
}

type remoteDrivingGatewayFixture struct {
	bot.DesktopBridge // Any legacy fallback panics; production registration off.
	driver            *previewDesktopRemoteDriver
}

func (f *remoteDrivingGatewayFixture) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	return f.driver.ExecuteDesktopCommand(ctx, command)
}
func (f *remoteDrivingGatewayFixture) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	return f.driver.DesktopTakeoverActive(route, actor)
}

func TestPreviewRemoteDrivingActualGatewayIngressAndContinuingAdminLoss(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	start := func(admin bool) *bot.BotGateway {
		admins := []string{"second-admin"}
		if admin {
			admins = append(admins, "operator")
		}
		gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
			Desktop: &remoteDrivingGatewayFixture{driver: f.driver}, Enabled: map[bot.Platform]bool{f.route.Platform: true},
			ConnectionAccess: map[string]bot.AccessConfig{f.route.ConnectionID: {Enabled: true, Users: []string{"operator", "member"}, Admins: admins}},
		}, []bot.AdapterBinding{{ID: f.route.ConnectionID, Domain: f.route.Domain, Platform: f.route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err := gw.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		return gw
	}
	send := func(actor, text string) string {
		t.Helper()
		adapter.messages <- bot.InboundMessage{Platform: f.route.Platform, ConnectionID: f.route.ConnectionID, Domain: f.route.Domain, ChatID: f.route.ChatID, ChatType: f.route.ChatType, UserID: actor, Text: text}
		return notificationWait(t, adapter.sent).Text
	}
	gw := start(true)
	t.Cleanup(gw.Stop)
	if out := send("member", "/desktop takeover "+f.entry.handle); !strings.Contains(out, "权限") {
		t.Fatal("member acquired through gateway", out)
	}
	select {
	case request := <-f.requests:
		t.Fatal("member reached remote", request.Action)
	default:
	}
	if out := send("operator", "/desktop takeover "+f.entry.handle); !strings.Contains(out, "已接管") {
		t.Fatal(out)
	}
	notificationWait(t, f.requests)
	acquire := notificationWait(t, f.requests)
	if out := send("second-admin", "/desktop takeover "+f.entry.handle); !strings.Contains(out, "未确认") {
		t.Fatal("second admin stole holder", out)
	}
	text := "ordinary  input\nwith exact spaces"
	if out := send("operator", text); !strings.Contains(out, "已接纳") {
		t.Fatal(out)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	input := notificationWait(t, f.requests)
	if input.Text != text || input.Key != acquire.Key {
		t.Fatal("continuing ingress changed original grant/text")
	}
	gw.Stop()
	gw = start(false)
	t.Cleanup(gw.Stop)
	if out := send("operator", "must not reach provider after admin loss"); !strings.Contains(out, "已解除") || !strings.Contains(out, "管理员权限") {
		t.Fatal(out)
	}
	if request := notificationWait(t, f.requests); request.Action != "release" || request.Key != acquire.Key {
		t.Fatal("admin loss dispatched input instead of original release")
	}
	if f.driver.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("continuing permission loss retained binding")
	}
}
