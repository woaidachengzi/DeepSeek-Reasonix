package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
)

func TestPreviewDesktopHostActualGatewayAndOriginalReleaseOnce(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "confirmed"
		if unknown {
			name = "unknown"
		}
		t.Run(name, func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
			store.persist = func(previewDesktopWatcher, bool) error { return nil }
			defer store.Close()
			stream := desktopbridge.NewOwnedEventStream()
			defer stream.Close()
			host, err := newPreviewDesktopHost(context.Background(), f.catalogue.manager, f.catalogue.remotes, stream, store)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = host.Shutdown(context.Background()) })
			adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
			newGateway := func(desktop *previewDesktopHost) *bot.BotGateway {
				return bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
					Desktop: desktop, Enabled: map[bot.Platform]bool{f.route.Platform: true},
					ConnectionAccess: map[string]bot.AccessConfig{f.route.ConnectionID: {Enabled: true, Users: []string{"member"}, Admins: []string{"operator"}}},
				}, []bot.AdapterBinding{{ID: f.route.ConnectionID, Domain: f.route.Domain, Platform: f.route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			}
			gw := newGateway(host)
			if err := gw.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(gw.Stop)
			send := func(actor, text string) string {
				adapter.messages <- bot.InboundMessage{Platform: f.route.Platform, ConnectionID: f.route.ConnectionID, Domain: f.route.Domain, ChatID: f.route.ChatID, ChatType: f.route.ChatType, UserID: actor, Text: text}
				return notificationWait(t, adapter.sent).Text
			}
			if out := send("operator", "/desktop status"); !strings.Contains(out, "未确认") {
				t.Fatal("pre-start scoped commands admitted", out)
			}
			if err := host.Start(context.Background(), gw); err != nil || !host.Available() {
				t.Fatal(err)
			}
			if err := host.Start(context.Background(), gw); err == nil {
				t.Fatal("host adopted another sender generation")
			}
			out := send("operator", "/desktop status")
			var handle string
			for _, field := range strings.Fields(out) {
				if strings.HasPrefix(field, "s-") && len(field) == 34 {
					handle = field
				}
			}
			if handle == "" || strings.Contains(out, "/remote/live.jsonl") {
				t.Fatal("missing opaque status or path leak", out)
			}
			if out := send("member", "/desktop takeover "+handle); !strings.Contains(out, "权限") {
				t.Fatal("member gained host access", out)
			}
			if out := send("operator", "/desktop watch on"); !strings.Contains(out, "已开启") {
				t.Fatal(out)
			}
			if out := send("operator", "/desktop takeover "+handle); !strings.Contains(out, "已接管") {
				t.Fatal(out)
			}
			capture := notificationWait(t, f.requests)
			acquire := notificationWait(t, f.requests)
			if capture.Action != "capture" || acquire.Action != "acquire" || len(acquire.Key) != 32 {
				t.Fatal("not original scoped acquisition")
			}
			if _, err := host.DriveInput(f.route, "unscoped"); err == nil {
				t.Fatal("legacy drive bypassed identity")
			}
			if _, err := host.Approve("private-id", true); err == nil {
				t.Fatal("legacy approval bypassed scope")
			}
			if unknown {
				f.mu.Lock()
				f.fail = "release"
				f.mu.Unlock()
			}
			host.StopIngress()
			if out := send("operator", "must not become an ordinary bot task"); !strings.Contains(out, "未确认") {
				t.Fatal("retired route fell back to ordinary bot processing", out)
			}
			gw.Stop()
			var waits sync.WaitGroup
			results := make(chan error, 3)
			for range 3 {
				waits.Add(1)
				go func() {
					defer waits.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					results <- host.Shutdown(ctx)
				}()
			}
			waits.Wait()
			close(results)
			for err := range results {
				if unknown != (err != nil) {
					t.Fatal("shutdown changed original outcome", err)
				}
			}
			release := notificationWait(t, f.requests)
			if release.Action != "release" || release.Key != acquire.Key || release.Scope != acquire.Scope {
				t.Fatal("shutdown recaptured another owner/key", release)
			}
			if len(f.requests) != 0 {
				t.Fatal("concurrent shutdown retried release")
			}
			if host.Available() || !host.DesktopTakeoverActive(f.route, "operator") || host.DesktopTakeoverActive(f.route, "member") || !store.Watching(f.route) {
				t.Fatal("old ingress survived or shared watch deleted")
			}
			if _, err := host.ExecuteDesktopCommand(context.Background(), bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: "status"}); err == nil {
				t.Fatal("old host revived")
			}
			if !unknown {
				next, err := newPreviewDesktopHost(context.Background(), f.catalogue.manager, f.catalogue.remotes, stream, store)
				if err != nil {
					t.Fatal(err)
				}
				defer next.Shutdown(context.Background())
				nextGateway := newGateway(next)
				if err := nextGateway.Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				defer nextGateway.Stop()
				if err := next.Start(context.Background(), nextGateway); err != nil {
					t.Fatal(err)
				}
				if !store.Watching(f.route) || !next.Available() {
					t.Fatal("fresh host lost persisted watch")
				}
			}
		})
	}
}

func TestPreviewDesktopHostLifecycleWaitHonorsCallerCancellation(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
	defer store.Close()
	stream := desktopbridge.NewOwnedEventStream()
	defer stream.Close()
	host, err := newPreviewDesktopHost(context.Background(), f.catalogue.manager, f.catalogue.remotes, stream, store)
	if err != nil {
		t.Fatal(err)
	}
	host.lifecycle <- struct{}{} // Whitebox contention budget, not owner proof.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Shutdown(ctx); err == nil {
		t.Fatal("cancelled lifecycle wait succeeded")
	}
	<-host.lifecycle
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
