package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewNetworkSettingsRedactSecretsAndPreserveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `future_setting = "keep"

[network]
proxy_mode = "custom"
proxy_url = "socks5://user:secret-url@127.0.0.1:7890"
no_proxy = "localhost"
future_network_setting = "keep"

[network.proxy]
type = "socks5"
server = "127.0.0.1"
port = 7890
username = "user"
password = "secret-password"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/network", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got.Code)
	}
	read := request(http.MethodGet, "", "", true)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"proxyUrlSet":true`) || strings.Contains(read.Body.String(), "secret-url") || strings.Contains(read.Body.String(), "secret-password") {
		t.Fatalf("unsafe network read: %d %s", read.Code, read.Body.String())
	}
	change := `{"proxyMode":"off","noProxy":"localhost,example.com","proxyType":"socks5","proxyServer":"127.0.0.1","proxyPort":7890,"proxyUsername":"user","proxyUrlAction":"keep","proxyPasswordAction":"keep"}`
	if got := request(http.MethodPost, change, "network-change-a", true); got.Code != http.StatusOK || strings.Contains(got.Body.String(), "secret") {
		t.Fatalf("unsafe network change: %d %s", got.Code, got.Body.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_setting", "future_network_setting", "secret-url", "secret-password", "example.com"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config lost %q: %s", want, raw)
		}
	}
	if got := request(http.MethodPost, `{"proxyMode":"custom","proxyType":"socks5","proxyPort":7890,"proxyUrlAction":"clear","proxyPasswordAction":"keep"}`, "network-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid custom proxy status = %d", got.Code)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
		t.Fatalf("invalid request mutated config: %v", err)
	}
	valid := `{"proxyMode":"custom","noProxy":"localhost","proxyType":"socks5","proxyServer":"127.0.0.1","proxyPort":7890,"proxyUsername":"user","proxyUrlAction":"clear","proxyPasswordAction":"clear"}`
	if got := request(http.MethodPost, valid, "network-change-b", true); got.Code != http.StatusOK {
		t.Fatalf("clear secrets status = %d %s", got.Code, got.Body.String())
	}
	viewResponse := request(http.MethodGet, "", "", true)
	var view networkSettingsView
	if err := json.Unmarshal(viewResponse.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ProxyURLSet || view.ProxyPasswordSet || view.ProxyMode != "custom" {
		t.Fatalf("network view after clear = %#v", view)
	}
	replace := `{"proxyMode":"custom","noProxy":"localhost","proxyType":"socks5","proxyServer":"127.0.0.1","proxyPort":7890,"proxyUsername":"user","proxyUrlAction":"replace","proxyUrl":"socks5://user:new-url-secret@127.0.0.1:7890","proxyPasswordAction":"replace","proxyPassword":"new-password-secret"}`
	if got := request(http.MethodPost, replace, "network-change-c", true); got.Code != http.StatusOK || strings.Contains(got.Body.String(), "new-url-secret") || strings.Contains(got.Body.String(), "new-password-secret") {
		t.Fatalf("unsafe secret replacement: %d %s", got.Code, got.Body.String())
	}
	if raw, err := os.ReadFile(path); err != nil || !strings.Contains(string(raw), "new-url-secret") || !strings.Contains(string(raw), "new-password-secret") {
		t.Fatalf("replacement secrets not saved: %v", err)
	}
}
