package main

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestProviderWailsEnvAuthenticatedBoundedReadOnly(t *testing.T) {
	home := t.TempDir()
	core := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", core)
	source := filepath.Join(home, ".reasonix")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	config := []byte("[[providers]]\nname = \"demo\"\nkind = \"openai\"\nbase_url = \"https://example.invalid/v1\"\napi_key_env = \"REASONIX_CONNECTION_ABC_KEY\"\nmodels = [\"m\"]\ndefault = \"m\"\n")
	env := []byte("REASONIX_CONNECTION_ABC_KEY='owned-fake-only'\nUNRELATED_KEY=never-export-this\n")
	for path, data := range map[string][]byte{filepath.Join(core, "config.toml"): config, filepath.Join(source, "config.toml"): config, filepath.Join(source, ".env"): env} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Ambient and project sources cannot win over the selected original file.
	t.Setenv("REASONIX_CONNECTION_ABC_KEY", "ambient-not-exported")
	handler := newBridgeServer(testToken, "wails-env-test").handler()
	for _, tc := range []struct {
		query  string
		auth   bool
		status int
	}{
		{"demo", false, 401}, {"demo&account=UNRELATED_KEY&sourcePath=/outside", true, 200}, {"unknown", true, 400}, {"UNRELATED_KEY", true, 400},
	} {
		r := httptest.NewRequest(http.MethodGet, "/v1/settings/wails-env-credential?provider="+tc.query, nil)
		if tc.auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("unexpected status %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, home) || strings.Contains(body, core) || strings.Contains(body, "never-export-this") || strings.Contains(body, "ambient-not-exported") {
			t.Fatal("source boundary leaked unrelated value/path")
		}
		if tc.status == 200 {
			var result struct {
				Value *string `json:"value"`
			}
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Value == nil || *result.Value != "owned-fake-only" {
				t.Fatal("selected original value was not resolved")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("secret response is cacheable")
			}
		} else if strings.Contains(body, "owned-fake-only") {
			t.Fatal("failure leaked credential")
		}
	}
	before, err := os.Stat(filepath.Join(source, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := readWailsEnvCredential("demo")
	if err != nil || value == nil || *value != "owned-fake-only" {
		t.Fatal("native source lookup failed")
	}
	after, _ := os.Stat(filepath.Join(source, ".env"))
	if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.ModTime() != after.ModTime() {
		t.Fatal("source metadata changed")
	}
	for path, data := range map[string][]byte{filepath.Join(core, "config.toml"): config, filepath.Join(source, "config.toml"): config, filepath.Join(source, ".env"): env} {
		raw, e := os.ReadFile(path)
		if e != nil || string(raw) != string(data) {
			t.Fatal("source or target changed")
		}
	}

	// The selected name/env alone cannot move an original key to another server.
	if err := os.WriteFile(filepath.Join(core, "config.toml"), []byte(strings.ReplaceAll(string(config), "example.invalid", "other.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWailsEnvCredential("demo"); err == nil {
		t.Fatal("changed credential destination accepted")
	}
	if err := os.WriteFile(filepath.Join(core, "config.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	// A current provider cannot nominate an unrelated old env account.
	if err := os.WriteFile(filepath.Join(core, "config.toml"), []byte(strings.ReplaceAll(string(config), "REASONIX_CONNECTION_ABC_KEY", "UNRELATED_KEY")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWailsEnvCredential("demo"); err == nil {
		t.Fatal("mismatched original provider account accepted")
	}
	if err := os.WriteFile(filepath.Join(core, "config.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(source, ".env")
	if err := os.Remove(envPath); err != nil {
		t.Fatal(err)
	}
	if value, err := readWailsEnvCredential("demo"); err != nil || value != nil {
		t.Fatal("missing original must be explicit absence")
	}
	if err := os.Symlink(filepath.Join(core, "config.toml"), envPath); err != nil {
		t.Fatal(err)
	}
	if _, err := readWailsEnvCredential("demo"); err == nil {
		t.Fatal("symlink secret source accepted")
	}
	if err := os.Remove(envPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWailsEnvCredential("demo"); err == nil {
		t.Fatal("oversized source accepted")
	}
}

func TestProviderWailsEnvNormalizesLegacyOfficialSnapshot(t *testing.T) {
	home, core := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", core)
	source := filepath.Join(home, ".reasonix")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	// Official normalization adds wallet/network metadata to this old config.
	config := []byte("[[providers]]\nname = \"deepseek\"\nkind = \"openai\"\nbase_url = \"https://api.deepseek.com/v1\"\napi_key_env = \"DEEPSEEK_API_KEY\"\nmodels = [\"deepseek-chat\"]\ndefault = \"deepseek-chat\"\n")
	for _, path := range []string{filepath.Join(core, "config.toml"), filepath.Join(source, "config.toml")} {
		if err := os.WriteFile(path, config, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Wails also accepts BOM-prefixed UTF-16 global env files.
	units := utf16.Encode([]rune("DEEPSEEK_API_KEY='owned-unicode-假值'\n"))
	env := make([]byte, 2+2*len(units))
	env[0], env[1] = 0xff, 0xfe
	for i, unit := range units {
		binary.LittleEndian.PutUint16(env[2+2*i:], unit)
	}
	envPath := filepath.Join(source, ".env")
	if err := os.WriteFile(envPath, env, 0600); err != nil {
		t.Fatal(err)
	}
	value, err := readWailsEnvCredential("deepseek")
	if err != nil || value == nil || *value != "owned-unicode-假值" {
		t.Fatal("normalized official UTF16 credential unavailable")
	}
	for path, want := range map[string][]byte{filepath.Join(source, "config.toml"): config, envPath: env} {
		got, e := os.ReadFile(path)
		if e != nil || string(got) != string(want) {
			t.Fatal("original snapshot modified")
		}
	}
	if err := os.WriteFile(filepath.Join(source, "config.toml"), []byte("[[providers]]\nname = [broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWailsEnvCredential("deepseek"); err == nil {
		t.Fatal("malformed source substituted defaults")
	}
}
