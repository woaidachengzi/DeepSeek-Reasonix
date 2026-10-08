package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/remote/controller"
)

func TestRemoteControllerSessionViewBridgeScopeAndPrivacy(t *testing.T) {
	var reads atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Query().Get("session") != "/remote/session.jsonl" {
			t.Error("wrong selected session")
		}
		_, _ = io.WriteString(w, `{"protocolVersion":1,"sessionPath":"/remote/session.jsonl","readOnly":true,"ownership":"saved","current":false,"modelRef":"","label":"","history":[{"role":"user","content":"remote question"},{"role":"assistant","content":"remote answer","reasoning":"remote thought","config":{"apiKey":"private-extra"}}],"token":"private-extra"}`)
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-view"
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, false); got.Code != 401 {
		t.Fatal("auth missing")
	}
	for _, body := range []string{`{}`, `{"sessionPath":"/remote/session.jsonl","url":"http://private"}`, `{"sessionPath":"bad\npath"}`, `{"sessionPath":1}`} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatalf("invalid body accepted %d", got.Code)
		}
	}
	if reads.Load() != 0 {
		t.Fatal("invalid request reached Serve")
	}
	got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, true)
	var response remoteControllerSessionViewResponse
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || response.View.SessionPath != "/remote/session.jsonl" || len(response.View.History) != 2 || response.View.History[1].Reasoning != "remote thought" {
		t.Fatalf("snapshot %d %s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "private-extra") {
		t.Fatal("unknown remote field forwarded")
	}
	if _, present := b.runtimes.Snapshot(); present {
		t.Fatal("remote history created local runtime")
	}
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/arbitrary/not-listed.jsonl"}`, true); got.Code != 404 || reads.Load() != 1 {
		t.Fatal("unlisted session reached remote history")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, true); got.Code != 409 || reads.Load() != 1 {
		t.Fatal("closed connection read accepted")
	}
}

func TestRemoteControllerSessionViewBridgeRevokesPendingRead(t *testing.T) {
	entered := make(chan struct{})
	abort := make(chan struct{})
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-abort:
		}
		_ = json.NewEncoder(w).Encode(controller.SessionView{ProtocolVersion: 1, SessionPath: "/remote/session.jsonl", ReadOnly: true, Ownership: "saved", History: []controller.HistoryMessage{{Role: "assistant", Content: "late-old-generation"}}})
	})
	defer close(abort)
	view := attachController(t, b, "/project")
	done := make(chan int, 1)
	go func() {
		done <- controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-view", `{"sessionPath":"/remote/session.jsonl"}`, true).Code
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read not started")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case status := <-done:
		if status != 409 {
			t.Fatalf("late snapshot published: %d", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revocation did not cancel read")
	}
}
