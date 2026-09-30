package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopExternalOpenerPreferenceNarrowWriteRestartAndRejectedInputs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	original := "# preserve this file\nfuture_root = \"keep\"\n[desktop]\nexternal_opener = \"uninstalled-editor\"\nfuture_desktop = \"keep\"\n[agent]\nreasoning_language = \"zh\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "opener-preferences").handler()
	call := func(body, id, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/settings/desktop/external-opener", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(requestIDHeader, id)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	// Reading a copied/unavailable preference must never rewrite the user's file.
	before, err := loadDesktopPreferences()
	if err != nil || before.ExternalOpener != "uninstalled-editor" {
		t.Fatalf("read copied preference: %#v %v", before, err)
	}
	if got := call(`{"id":"finder"}`, "opener-unauthorized", "wrong"); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized: %d", got.Code)
	}
	for i, body := range []string{`{}`, `{"id":""}`, `{"id":"  "}`, `{"id":"/bin/sh"}`, `{"id":"vscode --execute"}`, `{"id":"vscode;touch"}`, `{"id":"finder","program":"/bin/sh"}`} {
		got := call(body, "reject-"+string(rune('a'+i)), testToken)
		if got.Code != http.StatusBadRequest {
			t.Fatalf("invalid input: %d %s", got.Code, got.Body.String())
		}
	}
	if bytes, _ := os.ReadFile(path); string(bytes) != original {
		t.Fatal("read or rejected mutation modified config")
	}
	body := `{"id":" VSCODE "}`
	got := call(body, "opener-success", testToken)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"externalOpener":"vscode"`) {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`external_opener = "vscode"`, `future_root = "keep"`, `future_desktop = "keep"`, `reasoning_language = "zh"`} {
		if !strings.Contains(string(bytes), want) {
			t.Fatalf("lost %q: %s", want, bytes)
		}
	}
	if replay := call(body, "opener-success", testToken); replay.Code != http.StatusOK || replay.Body.String() != got.Body.String() {
		t.Fatal("same request did not replay result")
	}
	if conflict := call(`{"id":"finder"}`, "opener-success", testToken); conflict.Code != http.StatusConflict {
		t.Fatalf("request ID conflict = %d", conflict.Code)
	}
	restarted := newBridgeServer(testToken, "after-restart").handler()
	read := httptest.NewRequest(http.MethodGet, "/v1/settings/desktop", nil)
	read.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	restarted.ServeHTTP(response, read)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"externalOpener":"vscode"`) {
		t.Fatalf("restart preference: %d %s", response.Code, response.Body.String())
	}
	if unchanged, _ := os.ReadFile(path); string(unchanged) != string(bytes) {
		t.Fatal("replay or conflict changed config")
	}
}

func TestDesktopExternalOpenerPreferenceSaveFailureKeepsOriginal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	original := "[desktop]\nexternal_opener = \"finder\"\ninvalid TOML"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/desktop/external-opener", strings.NewReader(`{"id":"vscode"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(requestIDHeader, "failed-save")
	response := httptest.NewRecorder()
	newBridgeServer(testToken, "failed-save").handler().ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "check profile permissions") {
		t.Fatalf("failure = %d %s", response.Code, response.Body.String())
	}
	if bytes, _ := os.ReadFile(path); string(bytes) != original {
		t.Fatal("failed save overwrote unreadable config")
	}
}
