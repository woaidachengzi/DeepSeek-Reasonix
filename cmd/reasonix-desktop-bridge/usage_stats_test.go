package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configpkg "reasonix/internal/config"
)

func TestPreviewUsageStatsReadsOnlySelectedProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	dir := configpkg.StatsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	day := now.Format("2006-01-02")
	rows := `{"ts":"` + now.Format(time.RFC3339Nano) + `","model":"preview/chat","source":"desktop-tauri","prompt":10,"completion":5,"total":15,"requests":1}` + "\n" +
		`{"ts":"` + now.Format(time.RFC3339Nano) + `","model":"other/chat","source":"cli","prompt":3,"completion":2,"total":5,"requests":1}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, day+".jsonl"), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-a")
	request := func(source string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/settings/usage-stats", strings.NewReader(`{"range":"7","source":"`+source+`"}`))
		r.Header.Set("Content-Type", "application/json")
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	if got := request("desktop-tauri", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got.Code)
	}
	if got := request("other-profile", true); got.Code != http.StatusBadRequest {
		t.Fatalf("unsupported source status = %d", got.Code)
	}
	got := request("desktop-tauri", true)
	if got.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body = %s", got.Code, got.Body.String())
	}
	var view previewUsageStatsResponse
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Tokens != 15 || view.Requests != 1 || view.TopModel != "preview/chat" {
		t.Fatalf("Preview stats = %#v", view)
	}
	if all := request("all", true); all.Code != http.StatusOK {
		t.Fatalf("all stats status = %d", all.Code)
	} else if err := json.Unmarshal(all.Body.Bytes(), &view); err != nil || view.Tokens != 20 {
		t.Fatalf("all stats = %#v, error = %v", view, err)
	}
}

func TestPreviewStatsRangeRejectsInvalidDates(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	for _, req := range []previewUsageStatsRequest{
		{Range: "custom", From: "2026-09-28", To: "2026-09-27"},
		{Range: "custom", From: "2026-09-01", To: "2026-09-28"},
		{Range: "custom", From: "2026-09-01", To: "bad"},
		{Range: "custom", From: "2010-01-01", To: "2026-09-27"},
		{Range: "365"},
	} {
		if _, _, err := previewStatsRange(req, now); err == nil {
			t.Fatalf("accepted invalid range %#v", req)
		}
	}
}
