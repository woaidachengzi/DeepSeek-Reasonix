package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"reasonix/internal/bot"
	"reasonix/internal/remote/controller"
)

func remoteNotificationFixture(t *testing.T, adapter *notificationAdapterFixture) (*previewDesktopRemoteNotifications, *previewDesktopWatchStore, *notificationObservedGateway, chan string, chan struct{}, *bridgeServer, *previewDesktopCatalogue) {
	t.Helper()
	store, sender, _, _, _ := notificationTestSetup(t, adapter)
	frames := make(chan string, 64)
	retire := make(chan struct{})
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("notification scanned saved history")
		w.WriteHeader(400)
	}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: "/remote/live.jsonl"}}, "observed"))
			return
		}
		if r.URL.Path != "/desktop/session-observation" || r.Method != http.MethodPost {
			t.Error("observation used fallback route")
			w.WriteHeader(400)
			return
		}
		var input controller.SessionObservationRequest
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Scope.SessionPath != "/remote/live.jsonl" || input.Scope.RuntimeEpoch != "observed-/remote/live.jsonl" || input.ProtocolVersion != 1 {
			t.Error("observation changed original Controller scope")
			w.WriteHeader(409)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n")
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-retire:
				return
			case kind := <-frames:
				if json.NewEncoder(w).Encode(controller.SessionObservationFrame{ProtocolVersion: 1, Kind: kind}) != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}
	})
	attachController(t, b, "/owned-remote")
	catalogue := newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
	t.Cleanup(catalogue.Close)
	entries, err := catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	n, err := newPreviewDesktopRemoteNotifications(context.Background(), catalogue, entries[0], store, sender)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	return n, store, sender, frames, retire, b, catalogue
}

func TestRemoteDesktopNotificationsOriginalSSHAndGatewayKindsOnly(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	n, store, sender, frames, _, _, _ := remoteNotificationFixture(t, adapter)
	if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- n.Run(context.Background()) }()
	t.Cleanup(func() { n.Close(); notificationWait(t, done) })
	for _, kind := range []string{"turn_started", "ask_request", "approval_request", "mcp_interaction", "turn_done"} {
		frames <- kind
		msg := notificationWait(t, adapter.sent)
		if msg.Text != previewDesktopEventSummary(kind) || strings.Contains(msg.Text, "/remote/") || len(msg.MediaURLs) != 0 || msg.SessionWebhook != "" {
			t.Fatal("private event metadata forwarded", msg)
		}
		if call := notificationWait(t, sender.calls); call.err != nil {
			t.Fatal(call.err)
		}
	}
	if err := n.Run(context.Background()); err == nil {
		t.Fatal("consumer admitted a second Run")
	}
}

func TestRemoteDesktopNotificationsCancelSDKOnRetirementAndWatchOff(t *testing.T) {
	for _, action := range []string{"source_eof", "watch_off", "actor_changed", "overflow", "connection", "catalogue", "close"} {
		t.Run(action, func(t *testing.T) {
			cancelled := make(chan struct{})
			adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 4), hook: func(ctx context.Context) error { <-ctx.Done(); close(cancelled); return ctx.Err() }}
			n, store, sender, frames, retire, _, catalogue := remoteNotificationFixture(t, adapter)
			if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- n.Run(context.Background()) }()
			frames <- "turn_started"
			notificationWait(t, adapter.sent)
			switch action {
			case "source_eof":
				close(retire)
			case "watch_off":
				if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", false); err != nil {
					t.Fatal(err)
				}
			case "actor_changed":
				if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "member", true); err != nil {
					t.Fatal(err)
				}
			case "overflow":
				for range 40 {
					frames <- "turn_done"
				}
			case "connection":
				n.entry.remote.cancel()
			case "catalogue":
				catalogue.Close()
			case "close":
				n.Close()
			}
			notificationWait(t, cancelled)
			notificationWait(t, sender.calls)
			n.Close()
			if err := notificationWait(t, done); err == nil {
				t.Fatal("retired consumer reported normal completion")
			}
			if len(adapter.sent) != 0 {
				t.Fatal("unknown send retried")
			}
		})
	}
}
