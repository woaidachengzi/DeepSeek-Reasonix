package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "reasonix/internal/config"
)

func TestPreviewRemoteSettingsReadWriteAndSSHImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host gpu\n  HostName gpu.example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := configpkg.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	credential := configpkg.RemotePasswordCredentialEnvName("old")
	if _, err := configpkg.SetCredential(credential, "secret-value"); err != nil {
		t.Fatal(err)
	}
	initial := "default_model = \"deepseek-flash\"\n\n[remote]\nimport_ssh_config = true\n\n[[remote.hosts]]\nname = \"old\"\nhost = \"old.example.test\"\npassword_env = \"" + credential + "\"\n"
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, path, body, id string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if id != "" {
			req.Header.Set(requestIDHeader, id)
		}
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		response := httptest.NewRecorder()
		server.handler().ServeHTTP(response, req)
		return response
	}
	if got := request(http.MethodGet, "/v1/settings/remote", "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read status = %d", got.Code)
	}
	var scan remoteSSHConfigScanView
	if got := request(http.MethodPost, "/v1/settings/remote/scan", "{}", "", true); got.Code != http.StatusOK {
		t.Fatalf("scan SSH config: %d %s", got.Code, got.Body.String())
	} else if err := json.Unmarshal(got.Body.Bytes(), &scan); err != nil {
		t.Fatal(err)
	}
	if len(scan.Aliases) != 1 || scan.Aliases[0].Alias != "gpu" {
		t.Fatalf("SSH aliases = %#v", scan.Aliases)
	}
	var view remoteSettingsView
	if got := request(http.MethodGet, "/v1/settings/remote", "", "", true); got.Code != http.StatusOK {
		t.Fatalf("read remote settings: %d %s", got.Code, got.Body.String())
	} else if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Hosts) != 1 || !view.Hosts[0].PasswordSet || view.Hosts[0].Name != "old" {
		t.Fatalf("initial hosts = %#v", view.Hosts)
	}
	updated, err := changeRemoteSettings(remoteSettingsChange{Action: "upsert", Host: remoteSettingsHostInput{Name: "old", Host: "old.example.test", Workspace: "/srv/updated"}})
	if err != nil || len(updated.Hosts) != 1 || !updated.Hosts[0].PasswordSet || updated.Hosts[0].Workspace != "/srv/updated" {
		t.Fatalf("editing host without touching credentials = %#v, err=%v", updated.Hosts, err)
	}
	if result := configpkg.ResolveCredential(credential); !result.Set || result.Value != "secret-value" {
		t.Fatal("editing a host without replacing its password lost the saved credential")
	}
	body := `{"action":"upsert","host":{"name":"gpu","host":"gpu","port":0,"user":"ops","identityFile":"~/.ssh/id_ed25519","proxyJump":"","workspace":"/srv/app","serveInstall":"auto","credentialMode":"remote","useSSHConfig":true,"passwordAction":"replace","password":"user-secret","passphraseAction":"replace","passphrase":"key-secret"}}`
	saved := request(http.MethodPost, "/v1/settings/remote/hosts", body, "remote-upsert-1", true)
	if got := saved; got.Code != http.StatusOK {
		t.Fatalf("save remote host: %d %s", got.Code, got.Body.String())
	}
	if strings.Contains(saved.Body.String(), "user-secret") || strings.Contains(saved.Body.String(), "key-secret") {
		t.Fatal("saved remote credential was returned to the caller")
	}
	if got := request(http.MethodGet, "/v1/settings/remote", "", "", true); got.Code != http.StatusOK {
		t.Fatalf("read after save: %d %s", got.Code, got.Body.String())
	} else if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Hosts) != 2 || view.Hosts[1].Name != "gpu" || view.Hosts[1].Workspace != "/srv/app" || !view.Hosts[1].UseSSHConfig || !view.Hosts[1].PasswordSet || !view.Hosts[1].PassphraseSet {
		t.Fatalf("saved hosts = %#v", view.Hosts)
	}
	if got := request(http.MethodPost, "/v1/settings/remote/hosts", `{"action":"remove","name":"old"}`, "remote-remove-1", true); got.Code != http.StatusOK {
		t.Fatalf("remove remote host: %d %s", got.Code, got.Body.String())
	}
	if result := configpkg.ResolveCredential(credential); result.Set {
		t.Fatal("removing a host retained its Reasonix-managed password")
	}
	for _, key := range []string{configpkg.RemotePasswordCredentialEnvName("gpu"), configpkg.RemotePassphraseCredentialEnvName("gpu")} {
		if result := configpkg.ResolveCredential(key); !result.Set {
			t.Fatalf("editing one host removed another host's credential %s", key)
		}
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `import_ssh_config = true`) || !strings.Contains(string(raw), `workspace = "/srv/app"`) || !strings.Contains(string(raw), `default_model = "deepseek-flash"`) {
		t.Fatalf("remote edit lost unrelated or saved settings: %s", raw)
	}
	if got := request(http.MethodPost, "/v1/settings/remote/hosts", `{"action":"remove","name":"gpu"}`, "remote-remove-2", true); got.Code != http.StatusOK {
		t.Fatalf("remove host with generated credentials: %d %s", got.Code, got.Body.String())
	}
	for _, key := range []string{configpkg.RemotePasswordCredentialEnvName("gpu"), configpkg.RemotePassphraseCredentialEnvName("gpu")} {
		if result := configpkg.ResolveCredential(key); result.Set {
			t.Fatalf("removing a host retained generated credential %s", key)
		}
	}
}

func TestPreviewRemoteSettingsRejectsMalformedConfigWithoutOverwriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	malformed := []byte("[remote\nhosts = [\n")
	if err := os.WriteFile(path, malformed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := changeRemoteSettings(remoteSettingsChange{Action: "upsert", Host: remoteSettingsHostInput{Name: "gpu", Host: "gpu"}}); err == nil {
		t.Fatal("malformed config unexpectedly accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(malformed) {
		t.Fatalf("malformed config changed: %q, err=%v", after, err)
	}
}
