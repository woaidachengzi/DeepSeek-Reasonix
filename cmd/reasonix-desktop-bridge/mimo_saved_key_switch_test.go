package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
)

func TestMiMoSavedCredentialProbeSwitchAndRetryShareCatalog(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	const name = "mimo-token-plan-cn-fixture"
	const ref = name + "/mimo-v2.6-flash"
	raw := fmt.Sprintf(`default_model = "existing/chat"
[desktop]
provider_access = ["existing", "%s"]
[[providers]]
name = "existing"
kind = "openai"
base_url = "http://127.0.0.1:9/v1"
models = ["chat"]
[[providers]]
name = "%s"
preset_id = "mimo-token-plan-cn"
kind = "openai"
base_url = "https://token-plan-cn.xiaomimimo.com/v1"
models = ["mimo-v2.5-pro", "mimo-v2.5"]
default = "mimo-v2.5-pro"
api_key_env = "REASONIX_MIMO_SAVED_FIXTURE_KEY"
`, name, name)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := persistProviderAPIKey(setProviderKeyRequest{ProviderName: name, APIKey: "fixture-saved-only"}); err != nil {
		t.Fatal(err)
	}
	var calls int
	mock := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-saved-only" {
			t.Error("probe lost saved credential")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer mock.Close()
	probe := func() {
		t.Helper()
		cfg, err := configpkg.LoadUserConfigReadOnly()
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := cfg.Provider(name)
		if !ok {
			t.Fatal("service missing")
		}
		copy := *entry
		copy.BaseURL = mock.URL + "/v1"
		copy.NoProxy = true
		// No transient key: use exactly the persisted key as the reopened settings page does.
		if err := runProviderProbeWithProxy(context.Background(), &copy, "mimo-v2.6-flash", "", netclient.ProxySpec{}); err != nil {
			t.Fatal(err)
		}
	}
	probe()
	id := "mimo-saved-switch"
	path := filepath.Join(home, "sessions", "tauri-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	transcript := agent.NewSession("base")
	transcript.Messages = append(transcript.Messages, provider.Message{Role: provider.RoleUser, Content: "existing question"}, provider.Message{Role: provider.RoleAssistant, Content: "existing answer"})
	if err := transcript.SaveIfAbsent(path); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	defer manager.Shutdown()
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: id, WorkspaceRoot: workspace, ModelRef: "existing/chat"}); err != nil {
		t.Fatal(err)
	}
	before, err := manager.History(id)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.SetSessionModel(context.Background(), id, ref)
	if err != nil || selected.ModelRef != ref {
		t.Fatalf("settings-listed MiMo cannot be selected: %v", err)
	}
	after, err := manager.History(id)
	if err != nil || !reflect.DeepEqual(before.Messages, after.Messages) {
		t.Fatal("model switch changed history")
	}
	probe()
	if _, err := manager.SetSessionModel(context.Background(), id, name+"/unknown-model"); !errors.Is(err, boot.ErrUnknownModel) {
		t.Fatalf("invalid model selection not rejected: %v", err)
	}
	recovered, ok := manager.Snapshot()
	if !ok || recovered.ModelRef != ref {
		t.Fatal("failed switch lost previous model")
	}
	probe()
	if calls != 3 {
		t.Fatalf("saved credential probe calls: %d", calls)
	}
}
