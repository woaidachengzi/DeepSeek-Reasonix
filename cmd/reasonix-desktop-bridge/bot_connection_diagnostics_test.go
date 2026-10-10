package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
)

func TestBotDiagnosticsKeepsConfigurationAndRuntimeFactsSeparate(t *testing.T) {
	settings := botSettingsView{Enabled: true, AccessControlConfigured: true, Channels: []botChannelSettings{{ID: "account-one", Platform: "feishu", Domain: "lark", Enabled: true, CredentialsSet: true, Label: "private-label", CredentialIdentity: "private-identity", WorkspaceRoot: "private-workspace"}}}
	health := bot.AdapterHealthSnapshot{ID: "account-one", Platform: bot.PlatformFeishu, Domain: "lark", Status: "running", LastError: "private-sdk-error", Name: "private-sdk-name"}
	for _, test := range []struct{ mode, config, runtime string }{
		{"valid", "configured", "running"},
		{"disabled", "disabled", "running"},
		{"bot_disabled", "bot_disabled", "running"},
		{"credential", "missing_credentials", "running"},
		{"credential_boolean", "missing_credentials", "running"},
		{"access", "access_blocked", "running"},
		{"refreshing", "configured", "refreshing"},
		{"wrong_id", "configured", "not_observed"},
		{"wrong_domain", "configured", "not_observed"},
		{"wrong_platform", "configured", "not_observed"},
		{"unknown_status", "configured", "unknown"},
		{"duplicate", "configured", "unknown"},
		{"prefix_alias", "configured", "not_observed"},
		{"legacy_weixin", "configured", "running"},
		{"legacy_dingtalk", "configured", "running"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			s := settings
			s.Channels = append([]botChannelSettings(nil), settings.Channels...)
			r := botRuntimeStatusView{AdapterHealth: []bot.AdapterHealthSnapshot{health}}
			switch test.mode {
			case "disabled":
				s.Channels[0].Enabled = false
			case "bot_disabled":
				s.Enabled = false
			case "credential":
				s.Channels[0].CredentialMissing = true
			case "credential_boolean":
				s.Channels[0].CredentialsSet = false
			case "access":
				s.AccessControlConfigured = false
			case "refreshing":
				r.Refreshing = true
			case "wrong_id":
				r.AdapterHealth[0].ID = "account-two"
			case "wrong_domain":
				r.AdapterHealth[0].Domain = "feishu"
			case "wrong_platform":
				r.AdapterHealth[0].Platform = bot.PlatformQQ
			case "unknown_status":
				r.AdapterHealth[0].Status = "private-provider-status"
			case "duplicate":
				r.AdapterHealth = append(r.AdapterHealth, health)
			case "prefix_alias":
				s.Channels[0].ID = "legacy:account-one"
			case "legacy_weixin", "legacy_dingtalk":
				platform := strings.TrimPrefix(test.mode, "legacy_")
				s.Channels[0].ID, s.Channels[0].Platform, s.Channels[0].Domain = "legacy:"+platform, platform, ""
				r.AdapterHealth[0].ID, r.AdapterHealth[0].Platform, r.AdapterHealth[0].Domain = platform, bot.Platform(platform), platform
			}
			view := projectBotConnectionDiagnostics(s, r)
			if !view.RuntimeObservationOnly || len(view.Connections) != 1 || view.Connections[0].ConfigStatus != test.config || view.Connections[0].RuntimeStatus != test.runtime {
				t.Fatal("diagnostics conflated saved configuration with adapter observation", view)
			}
			data, err := json.Marshal(view)
			if err != nil || bytes.Contains(data, []byte("private-")) {
				t.Fatal("diagnostics exposed private metadata", err)
			}
		})
	}
}

func TestBotDiagnosticsReadOnlyHTTPAndUnavailableConfiguration(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Setenv("REASONIX_DIAGNOSTIC_PRIVATE_SECRET", "private-value")
	cfg := appconfig.Default()
	cfg.Bot.Feishu.AppID = "private-identity"
	cfg.Bot.Feishu.AppSecretEnv = "REASONIX_DIAGNOSTIC_PRIVATE_SECRET"
	if err := cfg.SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(appconfig.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	b := &bridgeServer{token: testToken, botRuntime: newPreviewBotRuntime()}
	for _, test := range []struct {
		auth   bool
		query  string
		status int
	}{{false, "", 401}, {true, "", 200}, {true, "?target=private-recipient", 400}} {
		r := httptest.NewRequest("GET", "/v1/settings/bots/diagnostics"+test.query, nil)
		if test.auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		b.handler().ServeHTTP(w, r)
		if w.Code != test.status || strings.Contains(w.Body.String(), "private-") || strings.Contains(w.Body.String(), "REASONIX_DIAGNOSTIC_PRIVATE_SECRET") {
			t.Fatal("unsafe diagnostic response", w.Code)
		}
	}
	if after, err := os.ReadFile(appconfig.UserConfigPath()); err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only diagnostics rewrote configuration", err)
	}
	if b.botRuntime.snapshot().Running || b.botRuntime.snapshot().Refreshing {
		t.Fatal("diagnostics started or refreshed adapters")
	}
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte("broken = [ private-config-error"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	b.botConnectionDiagnostics(w, httptest.NewRequest("GET", "/v1/settings/bots/diagnostics", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private-config-error") {
		t.Fatal("invalid config diagnostic leaked parser details", w.Code)
	}
}

func TestBotDiagnosticsRejectsLegacyRuntimeIDAliases(t *testing.T) {
	settings := botSettingsView{Enabled: true, AccessControlConfigured: true, Channels: []botChannelSettings{
		{ID: "legacy:weixin", Platform: "weixin", Enabled: true, CredentialsSet: true},
		{ID: "weixin", Platform: "weixin", Domain: "weixin", Enabled: true, CredentialsSet: true},
	}}
	runtime := botRuntimeStatusView{AdapterHealth: []bot.AdapterHealthSnapshot{{ID: "weixin", Platform: bot.PlatformWeixin, Domain: "weixin", Status: "running"}}}
	view := projectBotConnectionDiagnostics(settings, runtime)
	for _, diagnostic := range view.Connections {
		if diagnostic.RuntimeStatus != "unknown" {
			t.Fatal("ambiguous legacy/custom adapter ID claimed another connection's runtime", diagnostic)
		}
	}
}
