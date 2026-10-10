package main

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

func TestPreviewBotRuntimeActualDesktopRefreshAndUnknownCleanupGate(t *testing.T) {
	for _, name := range []string{"confirmed", "unknown", "parent_cancel"} {
		unknown := name == "unknown"
		t.Run(name, func(t *testing.T) {
			f := remoteDrivingTestFixture(t)
			t.Setenv("REASONIX_STATE_HOME", t.TempDir())
			t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
			t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
			cfg := appconfig.Default()
			cfg.Bot.Enabled = true
			cfg.Bot.Connections = []appconfig.BotConnectionConfig{{ID: f.route.ConnectionID, Provider: "feishu", Domain: f.route.Domain, Enabled: true, Access: appconfig.BotAccessConfig{Enabled: true, Users: []string{"member"}, Admins: []string{"operator"}}}}
			if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
				t.Fatal(err)
			}
			stream := desktopbridge.NewOwnedEventStream()
			t.Cleanup(stream.Close)
			runtime := newPreviewBotRuntime()
			runtime.configureDesktop(f.catalogue.manager, f.catalogue.remotes, stream)
			t.Cleanup(runtime.stop)
			var created atomic.Int32
			adapters := make(chan *notificationAdapterFixture, 8)
			runtime.bindingFactory = func(*appconfig.Config, map[bot.Platform]bool, *slog.Logger) []bot.AdapterBinding {
				created.Add(1)
				adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
				adapters <- adapter
				return []bot.AdapterBinding{{ID: f.route.ConnectionID, Domain: f.route.Domain, Platform: f.route.Platform, Adapter: adapter}}
			}
			parent, cancelParent := context.WithCancel(context.Background())
			defer cancelParent()
			if err := runtime.refresh(parent); err != nil {
				t.Fatal(err)
			}
			adapter := notificationWait(t, adapters)
			if status := runtime.snapshot(); !status.Running || !status.DesktopBridgeAvailable {
				t.Fatal("production runtime did not bind actual scoped host", status)
			}
			runtime.mu.Lock()
			oldHost, oldGateway := runtime.desktop, runtime.gateway
			runtime.mu.Unlock()
			send := func(text string) string {
				adapter.messages <- bot.InboundMessage{Platform: f.route.Platform, ConnectionID: f.route.ConnectionID, Domain: f.route.Domain, ChatID: f.route.ChatID, ChatType: f.route.ChatType, UserID: "operator", Text: text}
				return notificationWait(t, adapter.sent).Text
			}
			status := send("/desktop status")
			var handle string
			for _, field := range strings.Fields(status) {
				if strings.HasPrefix(field, "s-") && len(field) == 34 {
					handle = field
				}
			}
			if handle == "" || strings.Contains(status, "/remote/live.jsonl") {
				t.Fatal("runtime used legacy/raw status", status)
			}
			if out := send("/desktop watch on"); !strings.Contains(out, "已开启") {
				t.Fatal(out)
			}
			if out := send("/desktop takeover " + handle); !strings.Contains(out, "已接管") {
				t.Fatal(out)
			}
			if capture := notificationWait(t, f.requests); capture.Action != "capture" {
				t.Fatal(capture)
			}
			acquire := notificationWait(t, f.requests)
			if acquire.Action != "acquire" {
				t.Fatal(acquire)
			}
			if name == "parent_cancel" {
				cancelParent()
				// Await actual observer retirement/runtime cleanup, not a sleep.
				runtime.watchDesktopGeneration(oldHost, oldGateway)
				release := notificationWait(t, f.requests)
				if release.Action != "release" || release.Key != acquire.Key || release.Scope != acquire.Scope || len(f.requests) != 0 {
					t.Fatal("parent exit lost original cleanup authority", release)
				}
				if state := runtime.snapshot(); state.Running || state.DesktopBridgeAvailable || created.Load() != 1 {
					t.Fatal("parent cancellation revived or retained gateway", state)
				}
				return
			}
			if unknown {
				f.mu.Lock()
				f.fail = "release"
				f.mu.Unlock()
			}
			err := runtime.refresh(context.Background())
			if unknown != (err != nil) {
				t.Fatal("refresh misreported cleanup", err)
			}
			release := notificationWait(t, f.requests)
			if release.Action != "release" || release.Scope != acquire.Scope || release.Key != acquire.Key || len(f.requests) != 0 {
				t.Fatal("refresh retried or changed original release", release)
			}
			if oldHost.Available() {
				t.Fatal("old host still admitted commands")
			}
			if unknown {
				if status := runtime.snapshot(); status.Running || status.DesktopBridgeAvailable || status.Status != "blocked" {
					t.Fatal("unknown cleanup started a replacement", status)
				}
				if created.Load() != 1 {
					t.Fatal("unknown cleanup constructed a new SDK generation")
				}
				if err := runtime.refresh(context.Background()); err == nil || created.Load() != 1 || len(f.requests) != 0 {
					t.Fatal("unknown cleanup automatically retried")
				}
			} else {
				notificationWait(t, adapters)
				runtime.mu.Lock()
				freshHost, freshGateway := runtime.desktop, runtime.gateway
				runtime.mu.Unlock()
				if freshHost == oldHost || freshGateway == oldGateway || created.Load() != 2 || !runtime.snapshot().DesktopBridgeAvailable || !runtime.watch.Watching(f.route) {
					t.Fatal("refresh did not replace exact generation/preserve watch")
				}
				// A late completion from the retired observer cannot stop the new one.
				runtime.watchDesktopGeneration(oldHost, oldGateway)
				if !runtime.snapshot().Running || !freshHost.Available() {
					t.Fatal("old observer stopped fresh generation")
				}
			}
			runtime.stop()
			if status := runtime.snapshot(); status.Running || status.DesktopBridgeAvailable {
				t.Fatal("runtime survived stop", status)
			}
			if unknown && !strings.Contains(runtime.snapshot().Message, "unconfirmed") {
				t.Fatal("stop concealed unknown grant cleanup")
			}
		})
	}
}
