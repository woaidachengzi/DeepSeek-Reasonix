package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/remote"
)

func TestRemoteAttemptOwnershipRejectsLateDialAfterDisconnectReplaceAndShutdown(t *testing.T) {
	manager := newPreviewRemoteSessions()
	create := func() *remote.Client {
		client, err := remote.New(remote.Options{Host: remote.ResolvedHost{HostName: "127.0.0.1"}})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	firstCtx, first, err := manager.begin("host", context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, next, _ := manager.begin("host", context.Background())
	if firstCtx.Err() == nil {
		t.Fatal("new request did not cancel prior dial")
	}
	if manager.putCurrent("host", first, create(), remote.HostKeyQuestion{}) {
		t.Fatal("stale dial published")
	}
	manager.finish("host", first)
	if !manager.putCurrent("host", next, create(), remote.HostKeyQuestion{}) {
		t.Fatal("current request was erased by old completion")
	}
	manager.disconnect("host")
	if manager.putCurrent("host", next, create(), remote.HostKeyQuestion{}) {
		t.Fatal("disconnect resurrected a connection")
	}
	ctx, last, _ := manager.begin("host", context.Background())
	manager.closeAll()
	if ctx.Err() == nil || manager.putCurrent("host", last, create(), remote.HostKeyQuestion{}) {
		t.Fatal("shutdown retained or resurrected a dial")
	}
	if _, _, err := manager.begin("host", context.Background()); err == nil {
		t.Fatal("manager accepted a dial after shutdown")
	}
}

func TestBotConnectionCRUDPersistsRedactsAndPreservesSharedCredential(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	input := &botConnectionInput{ID: "lark-work", Platform: "feishu", Domain: "lark", Label: "Work", Identity: "fixture-id", Secret: "fixture-only-secret"}
	view, err := changeBotSettings(botSettingsChange{Action: "create_connection", Connection: input})
	if err != nil {
		t.Fatal(err)
	}
	var found *botChannelSettings
	for i := range view.Channels {
		if view.Channels[i].ID == input.ID {
			found = &view.Channels[i]
		}
	}
	if found == nil || found.Enabled || !found.CredentialsSet || found.Access == nil || !found.Access.PairingEnabled || found.Access.AllowAll {
		t.Fatalf("unsafe initial connection: %+v", found)
	}
	encoded, _ := json.Marshal(view)
	data, _ := os.ReadFile(appconfig.UserConfigPath())
	if strings.Contains(string(encoded), input.Secret) || strings.Contains(string(data), input.Secret) {
		t.Fatal("secret exposed in configuration or response")
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "create_connection", Connection: input}); err == nil {
		t.Fatal("duplicate connection overwritten")
	}
	loaded, err := appconfig.LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	loaded.Bot.Routes = append(loaded.Bot.Routes, appconfig.BotRouteConfig{ConnectionID: input.ID}, appconfig.BotRouteConfig{ConnectionID: "other"})
	loaded.Bot.Control.TokenEnv = botCredentialKey(input.ID)
	if err := loaded.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "remove_connection", ChannelID: input.ID}); err != nil {
		t.Fatal(err)
	}
	if !appconfig.ResolveCredentialForRootGlobalFirst(".", botCredentialKey(input.ID)).Set {
		t.Fatal("shared credential deleted")
	}
	loaded, _ = appconfig.LoadUserConfigReadOnly()
	if len(loaded.Bot.Connections) != 0 || len(loaded.Bot.Routes) != 1 || loaded.Bot.Routes[0].ConnectionID != "other" {
		t.Fatal("removal retained owned route or deleted unrelated route")
	}
	loaded.Bot.Control.TokenEnv = ""
	if err := loaded.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "create_connection", Connection: input}); err != nil {
		t.Fatal(err)
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "remove_connection", ChannelID: input.ID}); err != nil {
		t.Fatal(err)
	}
	if appconfig.ResolveCredentialForRootGlobalFirst(".", botCredentialKey(input.ID)).Set {
		t.Fatal("unused owned credential retained")
	}
	input.ID = "qq"
	if _, err := changeBotSettings(botSettingsChange{Action: "create_connection", Connection: input}); err == nil {
		t.Fatal("legacy runtime ID collision accepted")
	}
}

func TestBotPairingAuthOrphanProtectionAndExactConnectionGrant(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Connections = []appconfig.BotConnectionConfig{{ID: "work", Provider: "feishu", Domain: "lark", Access: appconfig.BotAccessConfig{PairingEnabled: true}}}
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	request, _, err := bot.CreateOrRefreshPairingRequest(bot.InboundMessage{Platform: bot.PlatformFeishu, ConnectionID: "work", Domain: "lark", ChatType: bot.ChatDM, ChatID: "chat", UserID: "user"}, bot.PairingConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer("fixture-token", "fixture-instance")
	defer bridge.botRuntime.stop()
	decide := func(auth, action, id string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(botPairingChange{Action: action, Code: request.Code})
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/bots/pairing", strings.NewReader(string(body)))
		req.Header.Set("Authorization", auth)
		req.Header.Set(requestIDHeader, id)
		result := httptest.NewRecorder()
		bridge.handler().ServeHTTP(result, req)
		return result
	}
	if got := decide("", "approve", "unauthorized"); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized=%d", got.Code)
	}
	if got := decide("Bearer fixture-token", "approve", "approved"); got.Code != http.StatusOK {
		t.Fatalf("approval=%d %s", got.Code, got.Body)
	}
	loaded, _ := appconfig.LoadUserConfigReadOnly()
	if len(loaded.Bot.Connections[0].Access.Users) != 1 || loaded.Bot.Connections[0].Access.Users[0] != "user" || len(loaded.Bot.Allowlist.FeishuUsers) > 0 {
		t.Fatal("grant was not scoped to exact connection")
	}
	// Idempotent replay must not repeat a consumed pairing grant.
	if got := decide("Bearer fixture-token", "approve", "approved"); got.Code != http.StatusOK {
		t.Fatalf("replay=%d", got.Code)
	}
	request, _, err = bot.CreateOrRefreshPairingRequest(bot.InboundMessage{Platform: bot.PlatformFeishu, ConnectionID: "removed", Domain: "lark", ChatType: bot.ChatDM, ChatID: "orphan-chat", UserID: "orphan"}, bot.PairingConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := decide("Bearer fixture-token", "approve", "orphan"); got.Code != http.StatusConflict {
		t.Fatalf("orphan=%d", got.Code)
	}
	pending, _ := bot.ListPairingRequests()
	if len(pending) != 1 {
		t.Fatal("refused orphan request was consumed")
	}
	if got := decide("Bearer fixture-token", "reject", "reject"); got.Code != http.StatusOK {
		t.Fatalf("reject=%d", got.Code)
	}
}

type lifecycleBotAdapter struct {
	messages   chan bot.InboundMessage
	stopped    atomic.Bool
	startError bool
}

func (a *lifecycleBotAdapter) Platform() bot.Platform              { return bot.PlatformFeishu }
func (a *lifecycleBotAdapter) Name() string                        { return "fixture" }
func (a *lifecycleBotAdapter) Messages() <-chan bot.InboundMessage { return a.messages }
func (a *lifecycleBotAdapter) Start(context.Context) error {
	if a.startError {
		return errors.New("fixture-only-secret")
	}
	return nil
}
func (a *lifecycleBotAdapter) Stop() error {
	if a.stopped.CompareAndSwap(false, true) {
		close(a.messages)
	}
	return nil
}
func (a *lifecycleBotAdapter) Send(context.Context, bot.OutboundMessage) (bot.SendResult, error) {
	return bot.SendResult{}, nil
}
func (a *lifecycleBotAdapter) SendTyping(context.Context, string) error { return nil }

func TestBotRuntimeReplacementStopsPreviousAdaptersAndRedactsErrors(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Enabled = true
	cfg.Bot.Feishu.Enabled = true
	cfg.Bot.Pairing.Enabled = true
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	runtime := newPreviewBotRuntime()
	defer runtime.stop()
	var adapters []*lifecycleBotAdapter
	runtime.bindingFactory = func(_ *appconfig.Config, _ map[bot.Platform]bool, _ *slog.Logger) []bot.AdapterBinding {
		a := &lifecycleBotAdapter{messages: make(chan bot.InboundMessage)}
		bad := &lifecycleBotAdapter{messages: make(chan bot.InboundMessage), startError: true}
		adapters = append(adapters, a, bad)
		return []bot.AdapterBinding{{ID: "good", Adapter: a}, {ID: "bad", Adapter: bad}}
	}
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := runtime.snapshot()
	encoded, _ := json.Marshal(status)
	if !status.Running || status.Status != "degraded" || strings.Contains(string(encoded), "fixture-only-secret") {
		t.Fatalf("runtime status=%s", encoded)
	}
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !adapters[0].stopped.Load() {
		t.Fatal("replacement left old adapter alive")
	}
	runtime.stop()
	if runtime.snapshot().Running || !adapters[2].stopped.Load() {
		t.Fatal("shutdown left running adapter")
	}
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 4 {
		t.Fatal("closed runtime restarted")
	}
}
