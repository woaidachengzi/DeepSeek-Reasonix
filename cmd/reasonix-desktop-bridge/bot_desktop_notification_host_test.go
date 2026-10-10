package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

func TestDesktopNotificationHostAllActualLocalAndRemoteOwners(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	store, sender, stream, _, initial := notificationTestSetup(t, adapter)
	initial.Close()
	p, endpoint := newOwnedCommandFixtureProvider(t)
	config := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\",\"beta\"]\ndefault=\"alpha\"\n", endpoint)
	factory := newControllerFactory(nil)
	factory.ownedEvents = stream
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	channels := map[string]chan string{"/remote/live.jsonl": make(chan string, 8), "/remote/detached.jsonl": make(chan string, 8)}
	opened := make(chan string, 8)
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("host scanned history"); w.WriteHeader(400) }, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: "/remote/live.jsonl"}, {Path: "/remote/detached.jsonl"}, {Path: "/remote/saved.jsonl"}, {Path: "/remote/external.jsonl"}}, "host"))
			return
		}
		var input controller.SessionObservationRequest
		if r.URL.Path != "/desktop/session-observation" || json.NewDecoder(r.Body).Decode(&input) != nil || input.ProtocolVersion != 1 || input.Scope.RuntimeEpoch != "host-"+input.Scope.SessionPath || channels[input.Scope.SessionPath] == nil {
			t.Error("host adopted an unowned or changed source")
			w.WriteHeader(409)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n")
		w.(http.Flusher).Flush()
		opened <- input.Scope.SessionPath
		for {
			select {
			case <-r.Context().Done():
				return
			case kind := <-channels[input.Scope.SessionPath]:
				if json.NewEncoder(w).Encode(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: kind}) != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}
	})
	attachController(t, b, "/owned-remote")
	// SSH fixture selects a new isolated profile; create the local runtime
	// only after that selection, so a real settings-stale fence is not bypassed.
	if err := os.MkdirAll(filepath.Dir(appconfig.UserConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "host-local", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	catalogue := newPreviewDesktopCatalogue(manager, b.remoteSessions)
	t.Cleanup(catalogue.Close)
	if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
		t.Fatal(err)
	}
	host, err := newPreviewDesktopNotificationHost(context.Background(), catalogue, stream, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()
	t.Cleanup(func() { host.Close(); notificationWait(t, done) })
	if err := notificationWait(t, host.Ready()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		seen[notificationWait(t, opened)] = true
	}
	if len(seen) != 2 {
		t.Fatal("not all remote owners observed", seen)
	}
	for _, path := range []string{"/remote/live.jsonl", "/remote/detached.jsonl"} {
		channels[path] <- "mcp_interaction"
		if msg := notificationWait(t, adapter.sent); msg.Text != previewDesktopEventSummary("mcp_interaction") {
			t.Fatal(msg)
		}
		notificationWait(t, sender.calls)
	}
	view, ok := manager.CommandSnapshot()
	if !ok {
		t.Fatal("local owner unavailable")
	}
	if err := manager.SubmitOwned(context.Background(), view, "private local input"); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, p.calls)
	if msg := notificationWait(t, adapter.sent); msg.Text != previewDesktopEventSummary("turn_started") {
		t.Fatal(msg)
	}
	notificationWait(t, sender.calls)
	close(p.finish)
	if msg := notificationWait(t, adapter.sent); msg.Text != previewDesktopEventSummary("turn_done") {
		t.Fatal(msg)
	}
	notificationWait(t, sender.calls)
	awaitDesktopDriverIdle(t, manager)
	if _, err := manager.SetSessionModel(context.Background(), view.Scope.SessionID, "local/beta"); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := host.Refresh(deadline); err != nil {
		t.Fatal(err)
	}
	replacement, ok := manager.CommandSnapshot()
	if !ok || replacement.Scope == view.Scope {
		t.Fatal("model replacement did not publish a fresh owner")
	}
	if err := manager.SubmitOwned(deadline, replacement, "private replacement input"); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, p.calls)
	for _, kind := range []string{"turn_started", "turn_done"} {
		if msg := notificationWait(t, adapter.sent); msg.Text != previewDesktopEventSummary(kind) {
			t.Fatal("new real owner not observed", msg)
		}
		notificationWait(t, sender.calls)
	}
	awaitDesktopDriverIdle(t, manager)
	// Polling unchanged remotes must not reopen them.
	select {
	case <-opened:
		t.Fatal("unchanged remote owners reopened")
	default:
	}
	if err := host.Shutdown(deadline); err != nil {
		t.Fatal(err)
	}
	select {
	case <-host.Done():
	default:
		t.Fatal("Shutdown did not await consumers")
	}
	if err := host.Run(context.Background()); err == nil {
		t.Fatal("second host Run admitted")
	}
}

func TestDesktopNotificationHostFailedGenerationNeverReconnects(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	n, store, sender, _, retire, _, catalogue := remoteNotificationFixture(t, adapter)
	n.Close()
	stream := desktopbridge.NewOwnedEventStream()
	defer stream.Close()
	host, err := newPreviewDesktopNotificationHost(context.Background(), catalogue, stream, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()
	t.Cleanup(func() { host.Close(); notificationWait(t, host.Done()) })
	if err := notificationWait(t, host.Ready()); err != nil {
		t.Fatal(err)
	}
	close(retire)
	if err := notificationWait(t, done); err == nil {
		t.Fatal("failed observer reported coverage")
	}
	if len(host.tasks) != 0 || len(host.seen) != 1 {
		t.Fatal("failed generation revived or consumer not awaited")
	}
	if err := host.reconcile(); err == nil {
		t.Fatal("stopped host reopened original generation")
	}
}

func TestDesktopNotificationHostShutdownCancelsAndAwaitsSDK(t *testing.T) {
	var cancelled atomic.Bool
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8), hook: func(ctx context.Context) error { <-ctx.Done(); cancelled.Store(true); return ctx.Err() }}
	n, store, sender, frames, _, _, catalogue := remoteNotificationFixture(t, adapter)
	n.Close()
	if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
		t.Fatal(err)
	}
	stream := desktopbridge.NewOwnedEventStream()
	defer stream.Close()
	host, err := newPreviewDesktopNotificationHost(context.Background(), catalogue, stream, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()
	t.Cleanup(func() { host.Close(); notificationWait(t, host.Done()) })
	if err := notificationWait(t, host.Ready()); err != nil {
		t.Fatal(err)
	}
	frames <- "turn_started"
	notificationWait(t, adapter.sent)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := host.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !cancelled.Load() || len(host.tasks) != 0 {
		t.Fatal("SDK or consumer survived Shutdown")
	}
	notificationWait(t, done)
	if len(adapter.sent) != 0 {
		t.Fatal("unknown send retried")
	}
}

func TestDesktopNotificationHostStartupFailureDrainsWholeCutBeforeDelivery(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	store, sender, stream, _, initial := notificationTestSetup(t, adapter)
	initial.Close()
	closed := make(chan struct{})
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("host scanned history"); w.WriteHeader(400) }, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: "/remote/live.jsonl"}, {Path: "/remote/detached.jsonl"}}, "partial"))
			return
		}
		var input controller.SessionObservationRequest
		if r.URL.Path != "/desktop/session-observation" || json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("unexpected route")
			w.WriteHeader(400)
			return
		}
		if input.Scope.SessionPath == "/remote/detached.jsonl" {
			w.WriteHeader(409)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n{\"protocolVersion\":1,\"kind\":\"turn_started\"}\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	})
	attachController(t, b, "/owned-remote")
	catalogue := newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
	defer catalogue.Close()
	if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
		t.Fatal(err)
	}
	host, err := newPreviewDesktopNotificationHost(context.Background(), catalogue, stream, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()
	t.Cleanup(func() { host.Close(); notificationWait(t, host.Done()) })
	if err := notificationWait(t, host.Ready()); err == nil {
		t.Fatal("partial startup reported complete coverage")
	}
	if err := notificationWait(t, done); err == nil {
		t.Fatal("partial startup succeeded")
	}
	notificationWait(t, closed)
	if len(host.tasks) != 0 || len(adapter.sent) != 0 || len(sender.calls) != 0 {
		t.Fatal("partial cut delivered or retained consumers")
	}
}
