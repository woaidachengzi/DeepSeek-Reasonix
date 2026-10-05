package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reasonix/internal/desktopbridge"
	"strings"
	"testing"
)

type approvalHTTPRuntime struct {
	bridgeTestRuntime
	mode   string
	writes int
}

func (r *approvalHTTPRuntime) ApprovalMode() string { return r.mode }
func (r *approvalHTTPRuntime) SetApprovalMode(mode string) error {
	r.mode = mode
	r.writes++
	return nil
}
func TestSessionApprovalHTTPAuthenticationOwnershipAndIdempotency(t *testing.T) {
	runtime := &approvalHTTPRuntime{bridgeTestRuntime: bridgeTestRuntime{path: "/tmp/session-approval", state: "idle"}, mode: "ask"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "active"}); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance", manager).handler()
	request := func(method, path, body, id string, auth bool) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	path := "/v1/sessions/active/approval-mode"
	if got := request(http.MethodGet, path, "", "", false); got != http.StatusUnauthorized {
		t.Fatal(got)
	}
	if got := request(http.MethodGet, path, "", "", true); got != http.StatusOK {
		t.Fatal(got)
	}
	if got := request(http.MethodPost, path, `{"mode":"invalid"}`, "invalid", true); got != http.StatusBadRequest {
		t.Fatal(got)
	}
	if got := request(http.MethodPost, "/v1/sessions/other/approval-mode", `{"mode":"yolo"}`, "other", true); got != http.StatusNotFound {
		t.Fatal(got)
	}
	for i := 0; i < 2; i++ {
		if got := request(http.MethodPost, path, `{"mode":"auto"}`, "same", true); got != http.StatusOK {
			t.Fatal(got)
		}
	}
	if runtime.writes != 1 || runtime.mode != "auto" {
		t.Fatal(runtime.writes, runtime.mode)
	}
	runtime.state = "paused"
	if got := request(http.MethodPost, path, `{"mode":"yolo"}`, "paused", true); got != http.StatusConflict {
		t.Fatal(got)
	}
	if runtime.mode != "auto" {
		t.Fatal("pending approval posture changed")
	}
}
