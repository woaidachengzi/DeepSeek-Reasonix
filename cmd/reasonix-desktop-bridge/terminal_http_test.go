package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/desktopbridge"
)

type terminalHTTPFixture struct {
	*bridgeTestRuntime
	creates atomic.Int32
	writes  atomic.Int32
	last    []byte
}

func (r *terminalHTTPFixture) TerminalWorkspace() (desktopbridge.TerminalWorkspaceView, error) {
	return desktopbridge.TerminalWorkspaceView{Available: true, Sessions: []desktopbridge.TerminalSessionView{},
		Shells: []desktopbridge.TerminalShellView{{ID: "sh", Label: "sh"}}}, nil
}
func (r *terminalHTTPFixture) CreateTerminal(context.Context, string, string) (desktopbridge.TerminalSessionView, error) {
	r.creates.Add(1)
	return desktopbridge.TerminalSessionView{ID: "owned-terminal", Title: "sh", Shell: "sh", Cwd: "/owned", Running: true}, nil
}
func (r *terminalHTTPFixture) WriteTerminal(_ string, data []byte) error {
	r.writes.Add(1)
	r.last = append([]byte(nil), data...)
	return nil
}
func (r *terminalHTTPFixture) ResizeTerminal(string, int, int) error { return nil }
func (r *terminalHTTPFixture) RenameTerminal(string, string) error   { return nil }
func (r *terminalHTTPFixture) CloseTerminal(string) error            { return nil }
func (r *terminalHTTPFixture) TerminalOutput(id string) (desktopbridge.TerminalOutputView, error) {
	if id != "owned-terminal" {
		return desktopbridge.TerminalOutputView{}, desktopbridge.ErrTerminalNotFound
	}
	return desktopbridge.TerminalOutputView{ID: id, Data: "b3duZWQ=", End: 5}, nil
}

func TestTerminalHTTPAuthenticationOwnershipInputBudgetAndIdempotence(t *testing.T) {
	runtime := &terminalHTTPFixture{bridgeTestRuntime: &bridgeTestRuntime{path: "/owned", state: "idle"}}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	bridge := newBridgeServer(testToken, "terminal-http-fixture", manager)
	request := func(method, route, body, requestID string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, route, strings.NewReader(body))
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		if requestID != "" {
			r.Header.Set(requestIDHeader, requestID)
		}
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	root := "/v1/sessions/owned/terminal"
	if request("GET", root, "", "", false).Code != http.StatusUnauthorized {
		t.Fatal("terminal route bypassed auth")
	}
	if request("GET", "/v1/sessions/foreign/terminal", "", "", true).Code != http.StatusNotFound {
		t.Fatal("foreign session reached terminal route")
	}
	if request("POST", root, `{}`, "", true).Code != http.StatusBadRequest {
		t.Fatal("terminal create admitted without request ID")
	}
	if request("POST", root, `{"path":".","executable":"owned-sentinel"}`, "unknown-field", true).Code != http.StatusBadRequest {
		t.Fatal("terminal create accepted renderer executable")
	}
	for i := 0; i < 2; i++ {
		if request("POST", root, `{"path":".","shellId":"sh"}`, "owned-create", true).Code != http.StatusCreated {
			t.Fatal("idempotent create failed")
		}
	}
	if runtime.creates.Load() != 1 {
		t.Fatal("terminal creation was duplicated by replay")
	}
	route := root + "/owned-terminal/input"
	data := []byte{0, 0xff, 0x1b, '[', 'A'}
	body, _ := json.Marshal(terminalInputRequest{Data: base64.StdEncoding.EncodeToString(data)})
	for i := 0; i < 2; i++ {
		if request("POST", route, string(body), "owned-input", true).Code != http.StatusAccepted {
			t.Fatal("terminal input failed")
		}
	}
	if runtime.writes.Load() != 1 || string(runtime.last) != string(data) {
		t.Fatal("input replay duplicated/corrupted terminal bytes")
	}
	if request("POST", route, `{"data":"b3RoZXI="}`, "owned-input", true).Code != http.StatusConflict {
		t.Fatal("same input request ID accepted different bytes")
	}
	for i, bad := range []string{`{"data":"not-base64"}`, `{"data":"YQ==\n"}`, `{"data":""}`, `{"data":"YQ==","command":"owned-sentinel"}`} {
		if request("POST", route, bad, "bad-input-"+string(rune('a'+i)), true).Code != http.StatusBadRequest {
			t.Fatal("invalid terminal input accepted")
		}
	}
	tooLarge, _ := json.Marshal(terminalInputRequest{Data: base64.StdEncoding.EncodeToString(make([]byte, desktopbridge.TerminalInputLimit+1))})
	if request("POST", route, string(tooLarge), "large-input", true).Code != http.StatusBadRequest {
		t.Fatal("input budget bypassed")
	}
	if runtime.writes.Load() != 1 {
		t.Fatal("rejected input reached terminal provider")
	}
	if request("GET", root+"/foreign/output", "", "", true).Code != http.StatusNotFound {
		t.Fatal("foreign terminal exposed output")
	}
	if request("GET", root+"/owned-terminal/output", "", "", true).Code != http.StatusOK {
		t.Fatal("owned terminal output unavailable")
	}
}
