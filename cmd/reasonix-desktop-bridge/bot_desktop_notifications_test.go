package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

// Notification-only fixture: any unexpected Desktop command would panic.
type notificationDesktopFixture struct{ bot.DesktopBridge }

type notificationAdapterFixture struct {
	messages chan bot.InboundMessage
	sent     chan bot.OutboundMessage
	hook     func(context.Context) error
}

func (a *notificationAdapterFixture) Platform() bot.Platform                   { return bot.PlatformFeishu }
func (a *notificationAdapterFixture) Name() string                             { return "isolated-notification-fixture" }
func (a *notificationAdapterFixture) Start(context.Context) error              { return nil }
func (a *notificationAdapterFixture) Stop() error                              { return nil }
func (a *notificationAdapterFixture) Messages() <-chan bot.InboundMessage      { return a.messages }
func (a *notificationAdapterFixture) SendTyping(context.Context, string) error { return nil }
func (a *notificationAdapterFixture) Send(ctx context.Context, msg bot.OutboundMessage) (bot.SendResult, error) {
	a.sent <- msg
	if a.hook != nil {
		return bot.SendResult{}, a.hook(ctx)
	}
	return bot.SendResult{}, nil
}

type notificationDelivery struct {
	route bot.DesktopWatchRoute
	err   error
}
type notificationObservedGateway struct {
	gateway *bot.BotGateway
	calls   chan notificationDelivery
}

func (s *notificationObservedGateway) SendDesktopNotification(ctx context.Context, route bot.DesktopWatchRoute, actor string, note bot.DesktopNotification) (bot.SendResult, error) {
	result, err := s.gateway.SendDesktopNotification(ctx, route, actor, note)
	s.calls <- notificationDelivery{route, err}
	return result, err
}

func notificationWait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("notification fixture timed out")
	}
	var zero T
	return zero
}

func notificationTestSetup(t *testing.T, adapter *notificationAdapterFixture) (*previewDesktopWatchStore, *notificationObservedGateway, *desktopbridge.OwnedEventStream, *desktopbridge.OwnedEventSource, *previewDesktopNotifications) {
	t.Helper()
	store, err := newPreviewDesktopWatchStore(desktopWatchTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	route := desktopWatchTestRoute()
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Desktop: notificationDesktopFixture{}, Enabled: map[bot.Platform]bool{bot.PlatformFeishu: true},
		ConnectionAccess: map[string]bot.AccessConfig{route.ConnectionID: {Enabled: true, Users: []string{"member"}, Admins: []string{"owner"}}},
	}, []bot.AdapterBinding{{ID: route.ConnectionID, Domain: route.Domain, Platform: route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gw.Stop)
	sender := &notificationObservedGateway{gw, make(chan notificationDelivery, 32)}
	stream := desktopbridge.NewOwnedEventStream()
	t.Cleanup(stream.Close)
	source := stream.NewSource()
	scope := desktopbridge.OwnedCommandScope{SessionID: "private-id", OwnerEpoch: 1, SessionPath: "/private/workspace/session", RuntimeEpoch: "private-epoch"}
	if !source.Activate(scope) {
		t.Fatal("source not published")
	}
	consumer, err := newPreviewDesktopNotifications(stream, scope, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumer.Close)
	return store, sender, stream, source, consumer
}

func notificationRun(t *testing.T, consumer *previewDesktopNotifications) chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- consumer.Run(context.Background()) }()
	t.Cleanup(func() { consumer.Close(); notificationWait(t, done) })
	return done
}

func TestPreviewDesktopNotificationsRealGatewayFiltersAndNeverForwardsPayload(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	store, sender, _, source, consumer := notificationTestSetup(t, adapter)
	route := desktopWatchTestRoute()
	route.ChatType = bot.ChatGroup
	if err := store.SetWatch(context.Background(), route, "owner", true); err != nil {
		t.Fatal(err)
	}
	denied := route
	denied.ChatID = "unauthorized-chat"
	if err := store.SetWatch(context.Background(), denied, "member", true); err != nil {
		t.Fatal(err)
	}
	notificationRun(t, consumer)
	for _, kind := range []event.Kind{event.Text, event.Reasoning, event.ToolResult, event.Notice} {
		source.Emit(event.Event{Kind: kind, Text: "private body", Reasoning: "private reasoning", Err: errors.New("private SDK failure")})
	}
	for _, kind := range []event.Kind{event.TurnStarted, event.AskRequest, event.ApprovalRequest, event.MCPInteractionRequest, event.TurnDone} {
		source.Emit(event.Event{Kind: kind, Text: "private body", TurnID: "private-turn", Err: errors.New("private failure")})
		for range 2 {
			call := notificationWait(t, sender.calls)
			if call.route == denied && !errors.Is(call.err, bot.ErrDesktopNotificationDenied) {
				t.Fatal("persisted member gained admin", call.err)
			}
			if call.route == route && call.err != nil {
				t.Fatal(call.err)
			}
		}
		msg := notificationWait(t, adapter.sent)
		if msg.ChatID != route.ChatID || msg.ChatType != bot.ChatGroup || strings.Contains(msg.Text, "private") || msg.Text == "" || msg.SessionWebhook != "" || len(msg.MediaURLs) != 0 {
			t.Fatalf("unsafe delivery: %#v", msg)
		}
	}
	if len(adapter.sent) != 0 || len(sender.calls) != 0 {
		t.Fatal("unapproved events forwarded or delivery retried")
	}
	if err := consumer.Run(context.Background()); !errors.Is(err, desktopbridge.ErrOwnedObservationClosed) {
		t.Fatal("second Run admitted", err)
	}
	select {
	case <-consumer.sub.Done():
		t.Fatal("second Run stopped the active consumer")
	default:
	}
}

func TestPreviewDesktopNotificationsWatchOffFailedSaveCancelsFlightAndDoesNotRevive(t *testing.T) {
	entered, returned := make(chan struct{}), make(chan struct{})
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32), hook: func(ctx context.Context) error {
		close(entered)
		defer close(returned)
		<-ctx.Done()
		return ctx.Err()
	}}
	store, sender, _, source, consumer := notificationTestSetup(t, adapter)
	route := desktopWatchTestRoute()
	if err := store.SetWatch(context.Background(), route, "owner", true); err != nil {
		t.Fatal(err)
	}
	notificationRun(t, consumer)
	source.Emit(event.Event{Kind: event.TurnStarted})
	notificationWait(t, entered)
	store.persist = func(previewDesktopWatcher, bool) error { return errors.New("isolated persistence failure") }
	if err := store.SetWatch(context.Background(), route, "owner", false); err == nil {
		t.Fatal("off save unexpectedly succeeded")
	}
	notificationWait(t, returned)
	if call := notificationWait(t, sender.calls); !errors.Is(call.err, bot.ErrDesktopNotificationUnknown) {
		t.Fatal(call.err)
	}
	// A second authorized route supplies an observable barrier after the revoked
	// generation, without wall-clock assertions about absence of a second send.
	barrier := route
	barrier.ChatID = "barrier-chat"
	store.persist = func(previewDesktopWatcher, bool) error { return nil }
	adapter.hook = nil // first Send has returned and consumer receipt was observed
	if err := store.SetWatch(context.Background(), barrier, "owner", true); err != nil {
		t.Fatal(err)
	}
	source.Emit(event.Event{Kind: event.TurnDone})
	call := notificationWait(t, sender.calls)
	if call.route != barrier || call.err != nil {
		t.Fatal("off subscription revived", call)
	}
	first, second := notificationWait(t, adapter.sent), notificationWait(t, adapter.sent)
	if first.ChatID != route.ChatID || second.ChatID != barrier.ChatID || len(adapter.sent) != 0 {
		t.Fatal("unknown delivery retried")
	}
}

func TestPreviewDesktopNotificationsRetirementCancelsBlockedSend(t *testing.T) {
	for _, mode := range []string{"source", "overflow", "stream", "consumer", "gateway", "actor", "watch-close"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{})
			adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32), hook: func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }}
			store, sender, stream, source, consumer := notificationTestSetup(t, adapter)
			if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
				t.Fatal(err)
			}
			notificationRun(t, consumer)
			source.Emit(event.Event{Kind: event.TurnStarted})
			notificationWait(t, entered)
			switch mode {
			case "source":
				source.Close()
			case "overflow":
				for range 40 {
					source.Emit(event.Event{Kind: event.Text, Text: "private buffered"})
				}
			case "stream":
				stream.Close()
			case "consumer":
				consumer.Close()
			case "gateway":
				sender.gateway.Stop()
				consumer.Close()
			case "actor":
				if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "member", true); err != nil {
					t.Fatal(err)
				}
			case "watch-close":
				store.Close()
			}
			if call := notificationWait(t, sender.calls); !errors.Is(call.err, bot.ErrDesktopNotificationUnknown) {
				t.Fatal(call.err)
			}
			if len(adapter.sent) != 1 {
				t.Fatal("blocked send retried")
			}
		})
	}
}

func TestPreviewDesktopNotificationsActualControllerToGateway(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	store, sender, stream, _, initial := notificationTestSetup(t, adapter)
	initial.Close()
	p, endpoint := newOwnedCommandFixtureProvider(t)
	config := fmt.Sprintf(`default_model="local/alpha"
[desktop]
provider_access=["local"]
[[providers]]
name="local"
kind="preview-owned-command-test"
base_url=%q
models=["alpha","beta"]
default="alpha"
`, endpoint)
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	factory := newControllerFactory(nil)
	factory.ownedEvents = stream
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "private-core-session", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	view, ok := manager.CommandSnapshot()
	if !ok {
		t.Fatal("actual owner unavailable")
	}
	consumer, err := newPreviewDesktopNotifications(stream, view.Scope, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	notificationRun(t, consumer)
	if err := store.SetWatch(context.Background(), desktopWatchTestRoute(), "owner", true); err != nil {
		t.Fatal(err)
	}
	if err := manager.SubmitOwned(context.Background(), view, "private user input"); err != nil {
		t.Fatal(err)
	}
	call := awaitOwnedCommand(t, p.calls)
	if !desktopDriverCallContains(call, "private user input") {
		t.Fatal("not an actual provider request")
	}
	start := notificationWait(t, adapter.sent)
	if start.Text != previewDesktopEventSummary("turn_started") {
		t.Fatal(start.Text)
	}
	close(p.finish)
	finish := notificationWait(t, adapter.sent)
	if finish.Text != previewDesktopEventSummary("turn_done") {
		t.Fatal("provider body leaked", finish.Text)
	}
	awaitDesktopDriverIdle(t, manager)
	for range 2 {
		if call := notificationWait(t, sender.calls); call.err != nil {
			t.Fatal(call.err)
		}
	}
	if len(adapter.sent) != 0 {
		t.Fatal("streamed text or reasoning forwarded")
	}
	if _, err := manager.SetSessionModel(context.Background(), view.Scope.SessionID, "local/beta"); err != nil {
		t.Fatal(err)
	}
	// The old consumer cannot silently subscribe to the replacement owner.
	select {
	case <-consumer.sub.Done():
	default:
		t.Fatal("actual replacement retained old observer")
	}
}
