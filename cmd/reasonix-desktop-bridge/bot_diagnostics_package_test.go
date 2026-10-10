package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	appconfig "reasonix/internal/config"
)

// Opt-in exact packaged executable, not an in-process handler or a rebuilt test
// server. The shared harness supplies minimal private HOME/state/cache, file
// credentials, loopback readiness, explicit auth and owned normal shutdown.
func TestBotDiagnosticsActualPackageReadOnly(t *testing.T) {
	binary := os.Getenv("REASONIX_BOT_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	for _, enabled := range []bool{false, true} {
		name := "channel_disabled"
		expected := "disabled"
		if enabled {
			name, expected = "gateway_disabled", "bot_disabled"
		}
		t.Run(name, func(t *testing.T) {
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
			cfg := appconfig.Default()
			cfg.Bot.Enabled = false // Never activate an external adapter.
			cfg.Bot.Feishu.Enabled = enabled
			cfg.Bot.Feishu.AppID = "private-package-identity-canary"
			cfg.Bot.Feishu.AppSecretEnv = "PRIVATE_PACKAGE_SECRET_CANARY"
			configPath := appconfig.UserConfigPath()
			if err := cfg.SaveTo(configPath); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := os.Stat(configPath)
			if err != nil {
				t.Fatal(err)
			}
			for launch := 0; launch < 2; launch++ {
				p := startSQLitePackagedSidecar(t, binary, profile, "")
				for _, probe := range []struct {
					auth   bool
					path   string
					status int
				}{
					{false, "/v1/settings/bots/diagnostics", 401},
					{true, "/v1/settings/bots/diagnostics?account=private-query-canary", 400},
					{true, "/v1/settings/bots/diagnostics", 200},
					{true, "/v1/settings/bots/diagnostics", 200},
				} {
					status, body := p.call(t, "GET", probe.path, nil, probe.auth)
					if status != probe.status || bytes.Contains(body, []byte("private-")) || bytes.Contains(body, []byte("PRIVATE_PACKAGE")) {
						t.Fatal("packaged diagnostic auth/query or privacy contract failed", status)
					}
					if status == 200 {
						var view botDiagnosticsView
						if json.Unmarshal(body, &view) != nil || view.ProtocolVersion != 1 || !view.RuntimeObservationOnly {
							t.Fatal("packaged diagnostics omitted observation contract")
						}
						found := false
						for _, row := range view.Connections {
							if row.ID == "legacy:feishu" {
								found = true
								if row.ConfigStatus != expected {
									t.Fatal("packaged diagnostics conflated configuration state", row.ConfigStatus)
								}
							}
						}
						if !found {
							t.Fatal("packaged diagnostics lost disabled legacy connection")
						}
					}
				}
				status, body := p.call(t, "GET", "/v1/settings/bots/runtime", nil, true)
				var runtime botRuntimeStatusView
				if status != 200 || json.Unmarshal(body, &runtime) != nil || runtime.Running || runtime.Refreshing {
					t.Fatal("read-only diagnostics activated/refreshed a bot runtime")
				}
				p.stop(t)
				after, err := os.ReadFile(configPath)
				afterMetadata, statErr := os.Stat(configPath)
				if err != nil || statErr != nil || !bytes.Equal(original, after) || metadata.Mode() != afterMetadata.Mode() || !metadata.ModTime().Equal(afterMetadata.ModTime()) {
					t.Fatal("packaged diagnostic lifecycle changed its configuration")
				}
			}
		})
	}
}
