package bot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func desktopNotificationGateway(t *testing.T, adapter Adapter) *BotGateway {
	t.Helper()
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	gw := NewGatewayWithAdapterBindings(GatewayConfig{
		Desktop: newFakeDesktopBridge(), Enabled: map[Platform]bool{PlatformFeishu: true},
		ConnectionAccess: map[string]AccessConfig{"private-watch": {Enabled: true, Users: []string{"member"}, Admins: []string{"owner"}}},
	}, []AdapterBinding{{ID: "private-watch", Domain: "feishu", Platform: PlatformFeishu, Adapter: adapter}}, discardLogger())
	t.Cleanup(gw.Stop)
	return gw
}

func desktopNotificationRoute() DesktopWatchRoute {
	return DesktopWatchRoute{Platform: PlatformFeishu, ConnectionID: "private-watch", Domain: "feishu", ChatType: ChatDM, ChatID: "fixture-chat"}
}

func TestDesktopNotificationExactStartedAudienceAndSharedChatPrivacy(t *testing.T) {
	adapter := newFakeAdapter(PlatformFeishu, "private-notify-fixture")
	gw := desktopNotificationGateway(t, adapter)
	route := desktopNotificationRoute()
	note := DesktopNotification{Summary: "桌面任务已完成。", PrivateDetail: "private command/error/workspace detail"}
	if _, err := gw.SendDesktopNotification(context.Background(), route, "owner", note); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal("not-started gateway sent", err)
	}
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []ChatType{ChatDM, ChatDirect, ChatGroup, ChatGuild, ChatThread} {
		route.ChatType = kind
		if _, err := gw.SendDesktopNotification(context.Background(), route, "owner", note); err != nil {
			t.Fatal(err)
		}
		got := adapter.sentMessages()[len(adapter.sentMessages())-1]
		if got.ChatID != route.ChatID || got.ChatType != kind || got.ConnectionID != route.ConnectionID || got.Domain != route.Domain || got.SessionWebhook != "" || len(got.MediaURLs) != 0 {
			t.Fatalf("routing = %#v", got)
		}
		private := kind == ChatDM || kind == ChatDirect
		if strings.Contains(got.Text, "private") != private {
			t.Fatalf("audience %q detail=%q", kind, got.Text)
		}
		if !private && got.Text != note.Summary {
			t.Fatal("group received more than safe summary")
		}
	}
	count := len(adapter.sentMessages())
	for _, mutate := range []func(*DesktopWatchRoute){
		func(r *DesktopWatchRoute) { r.Platform = PlatformQQ }, func(r *DesktopWatchRoute) { r.ConnectionID = "other" },
		func(r *DesktopWatchRoute) { r.Domain = "" }, func(r *DesktopWatchRoute) { r.Domain = "FEISHU" },
		func(r *DesktopWatchRoute) { r.ChatType = "unknown" }, func(r *DesktopWatchRoute) { r.ChatID = "" },
	} {
		wrong := route
		mutate(&wrong)
		if _, err := gw.SendDesktopNotification(context.Background(), wrong, "owner", note); !errors.Is(err, ErrDesktopNotificationDenied) {
			t.Fatal("foreign audience admitted", err)
		}
	}
	if _, err := gw.SendDesktopNotification(context.Background(), route, "member", note); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal("non-admin sent", err)
	}
	// Current admission and role are evaluated on each send, not cached when a
	// watch was created. Production refresh replaces/stops the whole gateway.
	gw.cfg.ConnectionAccess["private-watch"] = AccessConfig{Enabled: true, Users: []string{"owner"}, Approvers: []string{"owner"}, Admins: []string{"new-owner"}}
	if _, err := gw.SendDesktopNotification(context.Background(), route, "owner", note); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal("revoked admin retained subscription authority", err)
	}
	if len(adapter.sentMessages()) != count {
		t.Fatal("rejected audience reached adapter")
	}
	gw.Stop()
	if _, err := gw.SendDesktopNotification(context.Background(), route, "new-owner", note); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal("stopped gateway sent", err)
	}
}

func TestDesktopNotificationInvalidPayloadCanceledAndDisabledDesktopDoNotSend(t *testing.T) {
	adapter := newFakeAdapter(PlatformFeishu, "private-notify-fixture")
	gw := desktopNotificationGateway(t, adapter)
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	route := desktopNotificationRoute()
	for _, note := range []DesktopNotification{
		{}, {Summary: strings.Repeat("x", 513)}, {Summary: "safe", PrivateDetail: strings.Repeat("x", 8193)},
		{Summary: "safe\x00"}, {Summary: "safe", PrivateDetail: string([]byte{255})},
	} {
		if _, err := gw.SendDesktopNotification(context.Background(), route, "owner", note); !errors.Is(err, ErrDesktopNotificationDenied) {
			t.Fatal("invalid payload dispatched", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gw.SendDesktopNotification(ctx, route, "owner", DesktopNotification{Summary: "safe"}); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal(err)
	}
	gw.cfg.Desktop = nil
	if _, err := gw.SendDesktopNotification(context.Background(), route, "owner", DesktopNotification{Summary: "safe"}); !errors.Is(err, ErrDesktopNotificationDenied) {
		t.Fatal("disabled desktop sent", err)
	}
	if len(adapter.sentMessages()) != 0 {
		t.Fatal("invalid admission reached adapter")
	}
}

type desktopNotificationBlockingAdapter struct {
	*fakeAdapter
	entered  chan struct{}
	returned chan struct{}
}

func (a *desktopNotificationBlockingAdapter) Send(ctx context.Context, _ OutboundMessage) (SendResult, error) {
	close(a.entered)
	defer close(a.returned)
	<-ctx.Done()
	return SendResult{}, errors.New("private SDK credential/workspace error")
}

func TestDesktopNotificationStopCancelsAndAwaitsInFlightWithoutRetry(t *testing.T) {
	adapter := &desktopNotificationBlockingAdapter{fakeAdapter: newFakeAdapter(PlatformFeishu, "blocked-notify"), entered: make(chan struct{}), returned: make(chan struct{})}
	gw := desktopNotificationGateway(t, adapter)
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := gw.SendDesktopNotification(context.Background(), desktopNotificationRoute(), "owner", DesktopNotification{Summary: "safe", PrivateDetail: "private"})
		result <- err
	}()
	select {
	case <-adapter.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("send not entered")
	}
	stopped := make(chan struct{})
	go func() { gw.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not await cancellation")
	}
	select {
	case <-adapter.returned:
	default:
		t.Fatal("Stop returned before outbound SDK returned")
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrDesktopNotificationUnknown) || strings.Contains(err.Error(), "private") {
			t.Fatal("unknown result leaked or reported success", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send result missing")
	}
}

func TestDesktopNotificationSendFailureNeverRetriesOrReturnsPrivateError(t *testing.T) {
	adapter := &resultAdapter{fakeAdapter: newFakeAdapter(PlatformFeishu, "failed-notify"), result: SendResult{MessageID: "partial-delivery-fixture"}, err: errors.New("private SDK token")}
	gw := desktopNotificationGateway(t, adapter)
	gw.cfg.IgnoreSelfMessages = true
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.SendDesktopNotification(context.Background(), desktopNotificationRoute(), "owner", DesktopNotification{Summary: "safe"}); !errors.Is(err, ErrDesktopNotificationUnknown) || strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
	if len(adapter.sentMessages()) != 1 {
		t.Fatal("send retried")
	}
	for _, health := range gw.AdapterHealth() {
		if strings.Contains(health.LastError, "private") {
			t.Fatal("SDK secret leaked through health status")
		}
	}
	route := desktopNotificationRoute()
	if !gw.isSelfMessage(InboundMessage{Platform: route.Platform, ConnectionID: route.ConnectionID, Domain: route.Domain, ChatID: route.ChatID, ChatType: route.ChatType, MessageID: "partial-delivery-fixture", UserID: "fixture-self"}) {
		t.Fatal("partial/unknown send lost echo suppression")
	}
}
