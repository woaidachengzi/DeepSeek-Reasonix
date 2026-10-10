package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/serve"
	"reasonix/internal/stats"
	"reasonix/internal/tool"
)

func remoteReclaimFixture(t *testing.T, adapter *notificationAdapterFixture) (*remoteDrivingFixture, *notificationObservedGateway, *previewDesktopWatchStore) {
	t.Helper()
	store, sender, _, _, _ := notificationTestSetup(t, adapter)
	f := remoteDrivingTestFixture(t)
	if err := f.driver.ConfigureReclaimNotifications(context.Background(), sender); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.driver.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := f.driver.AwaitReclaimNotifications(ctx); err != nil {
			t.Error(err)
		}
	})
	return f, sender, store
}
func remoteReclaimCommand(f *remoteDrivingFixture, action string) bot.DesktopCommand {
	c := f.command(action)
	c.ActorID = "owner"
	return c
}
func remoteReclaimTake(t *testing.T, f *remoteDrivingFixture) {
	t.Helper()
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "takeover")); err != nil {
		t.Fatal(err)
	}
	if c := notificationWait(t, f.requests); c.Action != "capture" {
		t.Fatal("no original capture")
	}
	if c := notificationWait(t, f.requests); c.Action != "acquire" {
		t.Fatal("no original acquire")
	}
}
func signalRemoteReclaim(f *remoteDrivingFixture) {
	f.mu.Lock()
	f.holder = ""
	f.version++
	close(f.reclaimSignal)
	f.mu.Unlock()
}

func TestRemoteReclaimOriginalSSHGatewayNoticeWithoutWatch(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	f, sender, store := remoteReclaimFixture(t, adapter)
	remoteReclaimTake(t, f)
	f.driver.mu.Lock()
	original := f.driver.bindings[f.route]
	f.driver.mu.Unlock()
	signalRemoteReclaim(f)
	msg := notificationWait(t, adapter.sent)
	if msg.Text != previewRemoteReclaimSummary || msg.ChatID != f.route.ChatID || store.Watching(f.route) || len(msg.MediaURLs) != 0 || msg.SessionWebhook != "" {
		t.Fatal("wrong original audience, watch dependency or private metadata", msg)
	}
	if result := notificationWait(t, sender.calls); result.err != nil {
		t.Fatal(result.err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.driver.AwaitReclaimNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.driver.DesktopTakeoverActive(f.route, "owner") {
		t.Fatal("old input fell through into ordinary bot")
	}
	for _, action := range []string{"drive", "takeover"} {
		if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, action)); err == nil {
			t.Fatal("original binding rearmed")
		}
	}
	if len(f.requests) != 0 || len(adapter.sent) != 0 {
		t.Fatal("reclaim retried, auto released or dispatched more input")
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "release")); err != nil {
		t.Fatal(err)
	}
	r := notificationWait(t, f.requests)
	if r.Action != "release" || r.Key != original.key || r.Scope != original.scope {
		t.Fatal("cleanup changed original key/scope")
	}
}

func TestRemoteReclaimRetirementCancelsAndAwaitsInFlightSDK(t *testing.T) {
	for _, action := range []string{"source_eof", "source_eof_during_send", "duplicate_frame", "private_frame", "expiry", "release", "wrong_actor_release", "catalogue", "connection", "close"} {
		t.Run(action, func(t *testing.T) {
			cancelled := make(chan struct{})
			adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8), hook: func(ctx context.Context) error { <-ctx.Done(); close(cancelled); return ctx.Err() }}
			f, sender, _ := remoteReclaimFixture(t, adapter)
			remoteReclaimTake(t, f)
			if action == "source_eof" {
				close(f.reclaimRetire)
			} else {
				signalRemoteReclaim(f)
				notificationWait(t, adapter.sent)
				switch action {
				case "source_eof_during_send":
					close(f.reclaimRetire)
				case "duplicate_frame":
					f.reclaimExtra <- `{"protocolVersion":1,"kind":"reclaimed"}`
				case "private_frame":
					f.reclaimExtra <- `{"protocolVersion":1,"kind":"reclaimed","text":"private diagnostic must not be forwarded"}`
				case "expiry":
					f.driver.mu.Lock()
					f.driver.bindings[f.route].started = time.Now().Add(-16 * time.Minute)
					f.driver.mu.Unlock()
					if f.driver.DesktopTakeoverActive(f.route, "owner") {
						t.Fatal("expired binding stayed active")
					}
				case "wrong_actor_release":
					command := remoteReclaimCommand(f, "release")
					command.ActorID = "member"
					if _, err := f.driver.ExecuteDesktopCommand(context.Background(), command); err == nil {
						t.Fatal("another actor released original binding")
					}
					select {
					case <-cancelled:
						t.Fatal("another actor cancelled original notice")
					default:
					}
					fallthrough
				case "release":
					if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "release")); err != nil {
						t.Fatal(err)
					}
				case "catalogue":
					f.catalogue.Close()
				case "connection":
					f.entry.remote.cancel()
				case "close":
					f.driver.Close()
				}
				notificationWait(t, cancelled)
				notificationWait(t, sender.calls)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := f.driver.AwaitReclaimNotifications(ctx); err != nil {
				t.Fatal(err)
			}
			if len(adapter.sent) != 0 {
				t.Fatal("retirement or EOF sent/retried a control notice")
			}
			if action == "expiry" {
				f.driver.mu.Lock()
				original := f.driver.bindings[f.route]
				f.driver.mu.Unlock()
				if original == nil || !original.expired {
					t.Fatal("expiry discarded original cleanup key")
				}
				if _, err := f.driver.ExecuteDesktopCommand(ctx, remoteReclaimCommand(f, "takeover")); err == nil || len(f.requests) != 0 {
					t.Fatal("expiry implicitly reacquired the original grant")
				}
				if _, err := f.driver.ExecuteDesktopCommand(ctx, remoteReclaimCommand(f, "release")); err != nil {
					t.Fatal(err)
				}
				if r := notificationWait(t, f.requests); r.Action != "release" || r.Key != original.key || r.Scope != original.scope {
					t.Fatal("expiry release changed original cleanup scope")
				}
			}
		})
	}
}

func TestRemoteReclaimMissingObservationRetainsUnknownOriginalCleanupKey(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	f, _, _ := remoteReclaimFixture(t, adapter)
	f.mu.Lock()
	f.fail = "observe"
	f.mu.Unlock()
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "takeover")); err == nil {
		t.Fatal("acquire acknowledged without observation readiness")
	}
	if c := notificationWait(t, f.requests); c.Action != "capture" {
		t.Fatal("no capture")
	}
	acquire := notificationWait(t, f.requests)
	if acquire.Action != "acquire" || !f.driver.DesktopTakeoverActive(f.route, "owner") {
		t.Fatal("unknown acquire discarded original cleanup binding")
	}
	for _, action := range []string{"takeover", "drive"} {
		if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, action)); err == nil {
			t.Fatal("unknown observer reacquired or drove")
		}
	}
	if len(f.requests) != 0 || len(adapter.sent) != 0 {
		t.Fatal("observer failure retried or fabricated local input")
	}
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "release")); err != nil {
		t.Fatal(err)
	}
	r := notificationWait(t, f.requests)
	if r.Action != "release" || r.Key != acquire.Key || r.Scope != acquire.Scope {
		t.Fatal("unknown observer cleanup drifted")
	}
}

func TestRemoteReclaimStateReplyBeforeSignalKeepsAudienceAndOriginalKey(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	f, sender, _ := remoteReclaimFixture(t, adapter)
	remoteReclaimTake(t, f)
	f.mu.Lock()
	f.holder = ""
	f.version++
	f.mu.Unlock()
	// Hold the stream frame; the inactive state response arrives first.
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "drive")); err == nil {
		t.Fatal("inactive grant drove")
	}
	if r := notificationWait(t, f.requests); r.Action != "state" {
		t.Fatal("extra input or capture dispatched")
	}
	if !f.driver.DesktopTakeoverActive(f.route, "owner") {
		t.Fatal("state race erased original rejection fence")
	}
	close(f.reclaimSignal)
	if msg := notificationWait(t, adapter.sent); msg.Text != previewRemoteReclaimSummary {
		t.Fatal("state reply discarded original notice")
	}
	notificationWait(t, sender.calls)
	if _, err := f.driver.ExecuteDesktopCommand(context.Background(), remoteReclaimCommand(f, "release")); err != nil {
		t.Fatal(err)
	}
	if r := notificationWait(t, f.requests); r.Action != "release" {
		t.Fatal("original key not explicitly released")
	}
}

func TestRemoteReclaimActualScopedHostGatewayRejectsContinuingInput(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
	store.persist = func(previewDesktopWatcher, bool) error { return nil }
	t.Cleanup(store.Close)
	stream := desktopbridge.NewOwnedEventStream()
	t.Cleanup(stream.Close)
	host, err := newPreviewDesktopHost(context.Background(), f.catalogue.manager, f.catalogue.remotes, stream, store)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Desktop: host, Enabled: map[bot.Platform]bool{f.route.Platform: true},
		ConnectionAccess: map[string]bot.AccessConfig{f.route.ConnectionID: {Enabled: true, Users: []string{"member"}, Admins: []string{"owner"}}},
	}, []bot.AdapterBinding{{ID: f.route.ConnectionID, Domain: f.route.Domain, Platform: f.route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gw.Stop)
	t.Cleanup(func() {
		if err := host.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := host.Start(context.Background(), gw); err != nil {
		t.Fatal(err)
	}
	send := func(actor, text string) string {
		adapter.messages <- bot.InboundMessage{Platform: f.route.Platform, ConnectionID: f.route.ConnectionID, Domain: f.route.Domain, ChatID: f.route.ChatID, ChatType: f.route.ChatType, UserID: actor, Text: text}
		return notificationWait(t, adapter.sent).Text
	}
	status := send("owner", "/desktop status")
	var handle string
	for _, field := range strings.Fields(status) {
		if strings.HasPrefix(field, "s-") && len(field) == 34 {
			handle = field
		}
	}
	if handle == "" {
		t.Fatal("no opaque owner")
	}
	if reply := send("owner", "/desktop takeover "+handle); !strings.Contains(reply, "已接管") {
		t.Fatal(reply)
	}
	if r := notificationWait(t, f.requests); r.Action != "capture" {
		t.Fatal("wrong capture")
	}
	acquire := notificationWait(t, f.requests)
	if acquire.Action != "acquire" {
		t.Fatal("wrong acquire")
	}
	signalRemoteReclaim(f)
	if msg := notificationWait(t, adapter.sent); msg.Text != previewRemoteReclaimSummary || store.Watching(f.route) {
		t.Fatal("scoped host did not deliver watchless notice")
	}
	if reply := send("owner", "private continuing input"); !strings.Contains(reply, "未确认") {
		t.Fatal("reclaimed input fell through to ordinary task", reply)
	}
	if len(f.requests) != 0 {
		t.Fatal("reclaimed ordinary input dispatched remotely")
	}
	if reply := send("owner", "/desktop release"); !strings.Contains(reply, "已解除") {
		t.Fatal(reply)
	}
	release := notificationWait(t, f.requests)
	if release.Action != "release" || release.Key != acquire.Key || release.Scope != acquire.Scope {
		t.Fatal("gateway cleanup changed original holder")
	}
}

func TestRemoteReclaimActualSSHServeAgentAndScopedGateway(t *testing.T) {
	// Delay handler installation until the SSH fixture has selected a fresh
	// private profile; no real accounts, provider endpoint or user history.
	var actual http.Handler
	forward := func(w http.ResponseWriter, r *http.Request) {
		if actual == nil {
			t.Error("source used before private setup")
			w.WriteHeader(503)
			return
		}
		actual.ServeHTTP(w, r)
	}
	b, _, _, _ := controllerFixture(t, forward, forward)
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	if err := os.MkdirAll(filepath.Dir(appconfig.UserConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte("default_model=\"owned-unconfigured\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	closeUsage := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := stats.CloseUsageCatalogs(ctx); err != nil {
			t.Error(err)
		}
	}
	closeUsage()
	t.Cleanup(closeUsage)
	p := &ownedCommandProvider{calls: make(chan ownedCommandCall, 4), finish: make(chan struct{})}
	dir := t.TempDir()
	path := filepath.Join(dir, "actual-remote-reclaim.jsonl")
	session := agent.NewSession("isolated remote source")
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	exec := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Runner: exec, Executor: exec, SessionDir: dir, SessionPath: path, Sink: event.Discard})
	awaitIdle := func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			state := ctrl.RuntimeStateSnapshot()
			if !ctrl.Running() && !state.Running && state.Phase == "idle" {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("actual source did not commit idle")
	}
	t.Cleanup(func() {
		select {
		case <-p.finish:
		default:
			close(p.finish)
		}
		awaitIdle()
		ctrl.Close()
	})
	actual = serve.New(ctrl, serve.NewBroadcaster(), appconfig.ServeConfig{AuthMode: "token", Token: "owned-controller-secret"}).Handler()
	attachController(t, b, "/owned-remote")
	store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
	store.persist = func(previewDesktopWatcher, bool) error { return nil }
	t.Cleanup(store.Close)
	stream := desktopbridge.NewOwnedEventStream()
	t.Cleanup(stream.Close)
	host, err := newPreviewDesktopHost(context.Background(), b.runtimes, b.remoteSessions, stream, store)
	if err != nil {
		t.Fatal(err)
	}
	route := desktopWatchTestRoute()
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Desktop: host, Enabled: map[bot.Platform]bool{route.Platform: true},
		ConnectionAccess: map[string]bot.AccessConfig{route.ConnectionID: {Enabled: true, Admins: []string{"owner"}}},
	}, []bot.AdapterBinding{{ID: route.ConnectionID, Domain: route.Domain, Platform: route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gw.Stop)
	t.Cleanup(func() {
		if err := host.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := host.Start(context.Background(), gw); err != nil {
		t.Fatal(err)
	}
	send := func(text string) string {
		adapter.messages <- bot.InboundMessage{Platform: route.Platform, ConnectionID: route.ConnectionID, Domain: route.Domain, ChatID: route.ChatID, ChatType: route.ChatType, UserID: "owner", Text: text}
		return notificationWait(t, adapter.sent).Text
	}
	status := send("/desktop status")
	var handle string
	for _, field := range strings.Fields(status) {
		if strings.HasPrefix(field, "s-") && len(field) == 34 {
			handle = field
		}
	}
	if handle == "" || strings.Contains(status, dir) {
		t.Fatal("source catalogue missing opaque owner")
	}
	if reply := send("/desktop takeover " + handle); !strings.Contains(reply, "已接管") {
		t.Fatal(reply)
	}
	if reply := send("private literal remote question"); !strings.Contains(reply, "已接纳") {
		t.Fatal(reply)
	}
	call := notificationWait(t, p.calls)
	if !ctrl.TrySteer("private local steering") {
		t.Fatal("actual running Agent did not accept local guidance")
	}
	if msg := notificationWait(t, adapter.sent); msg.Text != previewRemoteReclaimSummary || store.Watching(route) {
		t.Fatal("actual SSH source did not deliver watchless fixed notice")
	}
	if call.ctx.Err() != nil {
		t.Fatal("source reclaim cancelled previously accepted Agent")
	}
	if reply := send("must not dispatch after reclaim"); !strings.Contains(reply, "未确认") {
		t.Fatal("source reclaim rerouted continuing input", reply)
	}
	if reply := send("/desktop release"); !strings.Contains(reply, "已解除") || call.ctx.Err() != nil {
		t.Fatal("release cancelled accepted task or failed", reply)
	}
	close(p.finish)
	awaitIdle()
	// Local steering may legitimately continue the accepted Agent turn with
	// another provider request. Reject the later remote input, not that work.
	for len(p.calls) != 0 {
		continued := <-p.calls
		payload, err := json.Marshal(continued.request)
		if err != nil || strings.Contains(string(payload), "must not dispatch after reclaim") {
			t.Fatal("reclaimed remote input reached provider")
		}
	}
	if agent.CanonicalSessionPath(ctrl.SessionPath()) != agent.CanonicalSessionPath(path) {
		t.Fatal("reclaim changed source owner")
	}
}
