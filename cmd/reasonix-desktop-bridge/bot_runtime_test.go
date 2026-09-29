package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
)

func TestPreviewBotRuntimePlanRequiresExplicitEnablementAndAccessControl(t *testing.T) {
	cfg := appconfig.Default()
	if plan := previewBotRuntimePlan(cfg); plan.status != "stopped" || plan.start {
		t.Fatalf("default config plan = %+v, want stopped", plan)
	}

	cfg.Bot.Enabled = true
	cfg.Bot.Pairing.Enabled = false
	cfg.Bot.Allowlist = appconfig.BotAllowlist{}
	cfg.Bot.Feishu.Enabled = true
	if plan := previewBotRuntimePlan(cfg); plan.status != "blocked" || plan.start {
		t.Fatalf("enabled bot without access control plan = %+v, want blocked", plan)
	}

	cfg.Bot.Allowlist.AllowAll = true
	plan := previewBotRuntimePlan(cfg)
	if !plan.start || plan.status != "running" || !plan.enabled[bot.PlatformFeishu] {
		t.Fatalf("access-controlled bot plan = %+v, want Feishu runtime start", plan)
	}
}

func TestBotSettingsNeverReturnsCredentialValuesAndEnableRequiresAccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("PREVIEW_FEISHU_SECRET", "must-not-leak")
	cfg := appconfig.Default()
	cfg.Bot.Feishu.AppID = "preview-app-id"
	cfg.Bot.Feishu.AppSecretEnv = "PREVIEW_FEISHU_SECRET"
	cfg.Bot.Feishu.Enabled = true
	cfg.Bot.Pairing.Enabled = false
	cfg.Bot.Allowlist = appconfig.BotAllowlist{}
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}

	view, err := readBotSettings()
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if strings.Contains(string(encoded), "must-not-leak") || !view.Channels[1].CredentialsSet {
		t.Fatalf("bot settings expose secret or miss its configured state: %s", encoded)
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "set_enabled", Enabled: true}); err == nil {
		t.Fatal("enabling the bot runtime without access control should fail")
	}
	unchanged, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload isolated profile: %v", err)
	}
	if unchanged.Bot.Enabled {
		t.Fatal("failed enable unexpectedly persisted")
	}
}

func TestBotSettingsCanToggleRuntimeAfterAccessIsConfigured(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Pairing.Enabled = true
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	view, err := changeBotSettings(botSettingsChange{Action: "set_enabled", Enabled: true})
	if err != nil {
		t.Fatalf("enable bot runtime: %v", err)
	}
	if !view.Enabled || !view.AccessControlConfigured {
		t.Fatalf("saved settings = %+v, want enabled with access control", view)
	}
	saved, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload saved profile: %v", err)
	}
	if !saved.Bot.Enabled {
		t.Fatal("bot runtime enablement was not persisted")
	}
}

func TestBotSettingsPairingAndAllowlistCanConfigureAccessControl(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Pairing.Enabled = false
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}

	view, err := changeBotSettings(botSettingsChange{Action: "set_allowlist", Platform: "feishu", List: "users", Values: []string{" user-a ", "user-a", "user-b"}})
	if err != nil {
		t.Fatalf("save Feishu allowlist: %v", err)
	}
	if !view.AccessControlConfigured || !view.AllowlistEnabled || strings.Join(view.Allowlist["feishu"].Users, ",") != "user-a,user-b" {
		t.Fatalf("saved access settings = %+v", view)
	}
	if len(view.Allowlist["qq"].Users) != 0 {
		t.Fatalf("empty allowlist should be a JSON-safe empty list: %+v", view.Allowlist["qq"].Users)
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "set_enabled", Enabled: true}); err != nil {
		t.Fatalf("enable bot after configuring allowlist: %v", err)
	}

	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg = appconfig.Default()
	cfg.Bot.Pairing.Enabled = false
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save second isolated profile: %v", err)
	}
	view, err = changeBotSettings(botSettingsChange{Action: "set_pairing", Enabled: true})
	if err != nil || !view.PairingEnabled || !view.AccessControlConfigured {
		t.Fatalf("pairing policy did not enable guarded access: view=%+v err=%v", view, err)
	}
	view, err = changeBotSettings(botSettingsChange{Action: "set_allow_all", Enabled: true})
	if err != nil || !view.AllowAll || view.AllowlistEnabled || !view.AccessControlConfigured {
		t.Fatalf("allow-everyone mode was not saved: view=%+v err=%v", view, err)
	}
	view, err = changeBotSettings(botSettingsChange{Action: "set_allowlist", Platform: "qq", List: "users", Values: []string{"trusted-user"}})
	if err != nil || view.AllowAll || !view.AllowlistEnabled || !view.AccessControlConfigured {
		t.Fatalf("saving a restricted list did not switch back to allowlist mode: view=%+v err=%v", view, err)
	}
}

func TestBotSettingsRejectsInvalidAllowlistWithoutChangingProfile(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	originalAllowlistEnabled := cfg.Bot.Allowlist.Enabled
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	for _, change := range []botSettingsChange{
		{Action: "set_allowlist", Platform: "unknown", List: "users", Values: []string{"user"}},
		{Action: "set_allowlist", Platform: "qq", List: "tokens", Values: []string{"user"}},
		{Action: "set_allowlist", Platform: "qq", List: "users", Values: []string{"  "}},
	} {
		if _, err := changeBotSettings(change); err == nil {
			t.Fatalf("invalid change %+v unexpectedly succeeded", change)
		}
	}
	saved, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload isolated profile: %v", err)
	}
	if saved.Bot.Allowlist.Enabled != originalAllowlistEnabled || len(saved.Bot.Allowlist.QQUsers) != 0 {
		t.Fatalf("invalid changes mutated access settings: %+v", saved.Bot.Allowlist)
	}
}

func TestBotSettingsPersistsConnectionSpecificAccess(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Connections = []appconfig.BotConnectionConfig{{
		ID: "feishu-lark", Provider: "feishu", Domain: "lark", Enabled: false,
	}}
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}

	view, err := changeBotSettings(botSettingsChange{Action: "set_channel_allowlist", ChannelID: "feishu-lark", List: "users", Values: []string{"feishu-user"}})
	if err != nil {
		t.Fatalf("save connection allowlist: %v", err)
	}
	var channel *botChannelSettings
	for i := range view.Channels {
		if view.Channels[i].ID == "feishu-lark" {
			channel = &view.Channels[i]
		}
	}
	if channel == nil || channel.Access == nil || !channel.Access.Enabled || channel.Access.AllowAll || len(channel.Access.Users) != 1 || channel.Access.Users[0] != "feishu-user" {
		t.Fatalf("connection settings did not expose saved access policy: %+v", channel)
	}
	if !view.AccessControlConfigured {
		t.Fatal("a saved access rule on a disabled connection should allow the user to activate that configured channel")
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "set_channel_access_mode", ChannelID: "feishu-lark", Mode: "everyone"}); err != nil {
		t.Fatalf("enable allow-everyone mode for connection: %v", err)
	}
	view, err = changeBotSettings(botSettingsChange{Action: "set_channel_pairing", ChannelID: "feishu-lark", Enabled: true})
	if err != nil {
		t.Fatalf("enable connection pairing: %v", err)
	}
	for i := range view.Channels {
		if view.Channels[i].ID == "feishu-lark" && (!view.Channels[i].Access.AllowAll || !view.Channels[i].Access.PairingEnabled) {
			t.Fatalf("connection policies did not round-trip: %+v", view.Channels[i].Access)
		}
	}
}

func TestBotSettingsPersistConnectionRuntimePreferences(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	cfg.Bot.Connections = []appconfig.BotConnectionConfig{{ID: "feishu-lark", Provider: "feishu", Domain: "lark"}}
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	model, approval, workspace := "openai/gpt-5", "yolo", "/tmp/bot-workspace"
	view, err := changeBotSettings(botSettingsChange{Action: "set_channel_runtime", ChannelID: "feishu-lark", Model: &model, ToolApprovalMode: &approval, WorkspaceRoot: &workspace})
	if err != nil {
		t.Fatalf("save connection runtime preferences: %v", err)
	}
	var channel *botChannelSettings
	for i := range view.Channels {
		if view.Channels[i].ID == "feishu-lark" {
			channel = &view.Channels[i]
		}
	}
	if channel == nil || !channel.RuntimeSettings || channel.Model != model || channel.ToolApprovalMode != approval || channel.WorkspaceRoot != workspace {
		t.Fatalf("runtime preferences did not round-trip through settings view: %+v", channel)
	}
	saved, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload saved profile: %v", err)
	}
	if saved.Bot.Connections[0].Model != model || saved.Bot.Connections[0].ToolApprovalMode != approval || saved.Bot.Connections[0].WorkspaceRoot != workspace {
		t.Fatalf("runtime preferences not persisted: %+v", saved.Bot.Connections[0])
	}
	inherit := ""
	if _, err := changeBotSettings(botSettingsChange{Action: "set_channel_runtime", ChannelID: "feishu-lark", ToolApprovalMode: &inherit}); err != nil {
		t.Fatalf("clear approval override to inherit: %v", err)
	}
	for _, mode := range []string{"unsafe", "YOLO"} {
		if _, err := changeBotSettings(botSettingsChange{Action: "set_channel_runtime", ChannelID: "feishu-lark", ToolApprovalMode: &mode}); err == nil {
			t.Fatalf("invalid approval mode %q unexpectedly accepted", mode)
		}
	}
}

func TestBotGatewayRuntimeSettingsRoundTrip(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	maxSteps, debounce, queueCap, ttl, pending := 42, 2500, 12, 90, 7
	queueMode, queueDrop := "collect", "old"
	ignoreSelf := false
	view, err := changeBotSettings(botSettingsChange{
		Action: "set_gateway_runtime", MaxSteps: &maxSteps, DebounceMs: &debounce,
		QueueMode: &queueMode, QueueCap: &queueCap, QueueDrop: &queueDrop,
		IgnoreSelfMessages: &ignoreSelf, PairingRequestTTLMinutes: &ttl, PairingMaxPending: &pending,
	})
	if err != nil {
		t.Fatalf("save gateway runtime settings: %v", err)
	}
	if view.MaxSteps != maxSteps || view.DebounceMs != debounce || view.QueueMode != queueMode || view.QueueCap != queueCap || view.QueueDrop != queueDrop || view.IgnoreSelfMessages || view.PairingRequestTTLMinutes != ttl || view.PairingMaxPending != pending {
		t.Fatalf("settings view did not round-trip gateway runtime values: %+v", view)
	}
	ids, err := changeBotSettings(botSettingsChange{Action: "set_self_user_ids", Platform: "qq", Values: []string{" self-a ", "self-a", "self-b"}})
	if err != nil {
		t.Fatalf("save self user IDs: %v", err)
	}
	if got := strings.Join(ids.SelfUserIDs["qq"], ","); got != "self-a,self-b" {
		t.Fatalf("QQ self IDs = %q", got)
	}
	saved, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload saved profile: %v", err)
	}
	if saved.Bot.MaxSteps != maxSteps || saved.Bot.DebounceMs != debounce || saved.Bot.QueueMode != queueMode || saved.Bot.QueueCap != queueCap || saved.Bot.QueueDrop != queueDrop || saved.Bot.IgnoreSelfMessages || saved.Bot.Pairing.RequestTTLMinutes != ttl || saved.Bot.Pairing.MaxPendingPerPlatform != pending || strings.Join(saved.Bot.SelfUserIDs.QQ, ",") != "self-a,self-b" {
		t.Fatalf("gateway runtime values were not persisted: %+v", saved.Bot)
	}
	invalidMode := "unsafe"
	if _, err := changeBotSettings(botSettingsChange{Action: "set_gateway_runtime", QueueMode: &invalidMode}); err == nil {
		t.Fatal("invalid queue mode unexpectedly accepted")
	}
}

func TestBotRoutesRoundTripAndRejectInvalidRules(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	routes := []botRouteSettings{{
		ConnectionID: "feishu-lark", Platform: "feishu", ChatType: "group", ChatID: "oc-group",
		WorkspaceRoot: "/tmp/project", Model: "openai/gpt-5", ToolApprovalMode: "auto",
	}}
	view, err := changeBotSettings(botSettingsChange{Action: "set_routes", Routes: routes})
	if err != nil {
		t.Fatalf("save bot routes: %v", err)
	}
	if len(view.Routes) != 1 || view.Routes[0] != routes[0] {
		t.Fatalf("routes did not round-trip through settings view: %+v", view.Routes)
	}
	saved, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("reload saved profile: %v", err)
	}
	if len(saved.Bot.Routes) != 1 || saved.Bot.Routes[0].WorkspaceRoot != "/tmp/project" || saved.Bot.Routes[0].Model != "openai/gpt-5" || saved.Bot.Routes[0].ToolApprovalMode != "auto" {
		t.Fatalf("route configuration not persisted: %+v", saved.Bot.Routes)
	}
	invalid := []botRouteSettings{{Platform: "unknown"}}
	if _, err := changeBotSettings(botSettingsChange{Action: "set_routes", Routes: invalid}); err == nil {
		t.Fatal("invalid route platform unexpectedly accepted")
	}
	if _, err := changeBotSettings(botSettingsChange{Action: "set_routes"}); err != nil {
		t.Fatalf("clear all routes: %v", err)
	}
	cleared, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil || len(cleared.Bot.Routes) != 0 {
		t.Fatalf("route clear left stale rules: routes=%+v err=%v", cleared.Bot.Routes, err)
	}
}

func TestBotCredentialSaveStoresSecretOutsideProfileAndNeverReturnsIt(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := appconfig.Default()
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatalf("save isolated profile: %v", err)
	}
	view, err := changeBotSettings(botSettingsChange{Action: "set_credentials", ChannelID: "legacy:feishu", Identity: "app-id", Secret: "secret-value"})
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}
	if !view.Channels[1].CredentialsSet || view.Channels[1].CredentialIdentity != "app-id" {
		t.Fatalf("saved channel settings = %+v", view.Channels[1])
	}
	configData, err := os.ReadFile(appconfig.UserConfigPath())
	if err != nil {
		t.Fatalf("read profile config: %v", err)
	}
	if strings.Contains(string(configData), "secret-value") {
		t.Fatal("bot secret was written into config.toml")
	}
	resolved := appconfig.ResolveCredentialForRootGlobalFirst(".", botCredentialKey("legacy:feishu"))
	if !resolved.Set || resolved.Value != "secret-value" {
		t.Fatal("bot secret was not written to the isolated credential store")
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "secret-value") {
		t.Fatal("bot settings response included the submitted secret")
	}
}

func TestBotRuntimeStatusHandlerReturnsProtocolAndSafeStatus(t *testing.T) {
	bridge := newBridgeServer("token", "instance")
	bridge.botRuntime.setStatus(botRuntimeStatusView{
		ProtocolVersion: 1, Status: "blocked", Message: "Bot access control is required before starting",
		DesktopBridgeAvailable: false,
	})
	response := httptest.NewRecorder()
	bridge.botRuntimeStatus(response, httptest.NewRequest("GET", "/v1/settings/bots/runtime", nil))
	if response.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body botRuntimeStatusView
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.ProtocolVersion != 1 || body.Status != "blocked" || body.DesktopBridgeAvailable {
		t.Fatalf("status response = %+v", body)
	}
	if strings.Contains(response.Body.String(), "token") {
		t.Fatalf("status response leaked a credential: %s", response.Body.String())
	}
}
