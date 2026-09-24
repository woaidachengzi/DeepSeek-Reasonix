package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

func TestBridgeOpenAndSwitchExposeSessionLifecycleConflictCodes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	sessionDir := appconfig.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	missingPath, err := bridgeSessionPath(sessionDir, "known-missing")
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.ImportLegacyCatalog(ctx, sessionDir, []sessionidentity.Candidate{{ID: "known-missing", Path: missingPath}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"known-deleting", "known-deleted"} {
		path, err := bridgeSessionPath(sessionDir, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := identities.Reserve(ctx, sessionDir, sessionidentity.Candidate{ID: id, Path: path}); err != nil {
			t.Fatal(err)
		}
		if err := identities.BeginDelete(ctx, id, path); err != nil {
			t.Fatal(err)
		}
		if id == "known-deleted" {
			if err := identities.FinishDelete(ctx, id, path); err != nil {
				t.Fatal(err)
			}
		}
	}
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	server := newBridgeServer(testToken, "lifecycle-codes", manager)
	for _, route := range []string{"/v1/sessions:open", "/v1/sessions:switch"} {
		for _, test := range []struct{ id, code string }{
			{"known-missing", "session_missing"},
			{"known-deleting", "session_deleting"},
			{"known-deleted", "session_deleted"},
		} {
			request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"sessionId":"`+test.id+`"}`))
			request.Header.Set("Authorization", "Bearer "+testToken)
			response := httptest.NewRecorder()
			server.handler().ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("%s %s: HTTP %d, body %s", route, test.id, response.Code, response.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != test.code {
				t.Fatalf("%s %s: error code %q, decode error %v", route, test.id, body.Error.Code, err)
			}
		}
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("opening missing session created transcript: %v", err)
	}
}
