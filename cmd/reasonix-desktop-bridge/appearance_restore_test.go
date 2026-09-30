package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDesktopAppearanceRollbackRestoresUnsetExplicitAndLegacyState(t *testing.T) {
	for _, seed := range []struct{ name, desktop string }{
		{"unset", ""}, {"explicit-auto", "theme = \"auto\"\n"}, {"legacy-style", "theme = \"dark\"\ntheme_style = \"glacier\"\n"},
	} {
		t.Run(seed.name, func(t *testing.T) {
			t.Setenv("REASONIX_HOME", t.TempDir())
			path := filepath.Join(os.Getenv("REASONIX_HOME"), "config.toml")
			initial := "future_root_option = \"keep-root\"\n[desktop]\nlanguage = \"zh\"\nterminal_theme = \"dark\"\nfuture_desktop_option = \"keep-desktop\"\n" + seed.desktop
			if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := loadDesktopPreferences()
			if err != nil {
				t.Fatal(err)
			}
			handler := newBridgeServer(testToken, "appearance-rollback").handler()
			sequence := 0
			call := func(endpoint string, body []byte, authenticated bool, id string) *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
				if authenticated {
					request.Header.Set("Authorization", "Bearer "+testToken)
				}
				request.Header.Set(requestIDHeader, id)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				return response
			}
			sequence++
			if got := call("/v1/settings/desktop/appearance", []byte(`{"theme":"light","style":"aurora"}`), true, "write-"+strconv.Itoa(sequence)); got.Code != http.StatusOK {
				t.Fatalf("save status: %d", got.Code)
			}
			request := restoreDesktopAppearanceRequest{
				Expected: desktopAppearanceSnapshot{Theme: "light", Style: "aurora", Configured: true},
				Previous: desktopAppearanceSnapshot{Theme: before.Theme, Style: before.ThemeStyle, Configured: before.AppearanceConfigured},
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := call("/v1/settings/desktop/appearance/restore", body, false, "unauthorized"); got.Code != http.StatusUnauthorized {
				t.Fatalf("unauthorized status: %d", got.Code)
			}
			if actual, _ := os.ReadFile(path); string(actual) != string(written) {
				t.Fatal("unauthenticated rollback changed config")
			}
			if got := call("/v1/settings/desktop/appearance/restore", body, true, "restore"); got.Code != http.StatusOK {
				t.Fatalf("restore status: %d", got.Code)
			}
			after, err := loadDesktopPreferences()
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("rollback changed preferences: before=%+v after=%+v", before, after)
			}
			restored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, preserved := range []string{`future_root_option = "keep-root"`, `future_desktop_option = "keep-desktop"`} {
				if !strings.Contains(string(restored), preserved) {
					t.Fatal("rollback lost unrelated/future config")
				}
			}
			if got := call("/v1/settings/desktop/appearance/restore", body, true, "restore"); got.Code != http.StatusOK {
				t.Fatalf("duplicate rollback status: %d", got.Code)
			}
			if actual, _ := os.ReadFile(path); string(actual) != string(restored) {
				t.Fatal("idempotent rollback changed config")
			}
		})
	}
}

func TestDesktopAppearanceRollbackRejectsStaleInvalidAndUnreadableConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	if err := os.WriteFile(path, []byte("[desktop]\ntheme = \"dark\"\ntheme_style = \"graphite\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "appearance-rollback-conflict").handler()
	request := restoreDesktopAppearanceRequest{
		Expected: desktopAppearanceSnapshot{Theme: "light", Style: "aurora", Configured: true},
		Previous: desktopAppearanceSnapshot{Theme: "auto", Configured: false},
	}
	call := func(value restoreDesktopAppearanceRequest, id string) *httptest.ResponseRecorder {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/settings/desktop/appearance/restore", strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set(requestIDHeader, id)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	original, _ := os.ReadFile(path)
	if got := call(request, "stale"); got.Code != http.StatusConflict {
		t.Fatalf("stale status: %d", got.Code)
	}
	for index, invalid := range []func(*restoreDesktopAppearanceRequest){
		func(r *restoreDesktopAppearanceRequest) { r.Expected.Theme = "sepia" },
		func(r *restoreDesktopAppearanceRequest) { r.Expected.Configured = false },
		func(r *restoreDesktopAppearanceRequest) { r.Previous.Style = "aurora" },
		func(r *restoreDesktopAppearanceRequest) { r.Previous.Configured = true; r.Previous.Style = "unknown" },
	} {
		value := request
		invalid(&value)
		if got := call(value, "invalid-"+strconv.Itoa(index)); got.Code != http.StatusBadRequest {
			t.Fatalf("invalid rollback status: %d", got.Code)
		}
	}
	if actual, _ := os.ReadFile(path); string(actual) != string(original) {
		t.Fatal("rejected rollback changed config")
	}
	corrupt := []byte("private-fixture-secret = [broken\n")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	got := call(request, "corrupt")
	if got.Code != http.StatusInternalServerError || strings.Contains(got.Body.String(), home) || strings.Contains(got.Body.String(), "private-fixture-secret") {
		t.Fatal("read failure was not redacted")
	}
	if actual, _ := os.ReadFile(path); string(actual) != string(corrupt) {
		t.Fatal("failed rollback replaced unreadable config")
	}
}
