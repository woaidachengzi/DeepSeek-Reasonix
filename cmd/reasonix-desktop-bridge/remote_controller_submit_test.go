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

const bridgeSubmitBody = `{"sessionPath":"/remote/session.jsonl","runtimeEpoch":"remote-instance","revision":7,"text":"用户问题\n/new"}`

func TestRemoteControllerSessionSubmitScopePrivacyAndNoLocalRuntime(t *testing.T) {
	var calls atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			ProtocolVersion int    `json:"protocolVersion"`
			SessionPath     string `json:"sessionPath"`
			RuntimeEpoch    string `json:"runtimeEpoch"`
			Revision        uint64 `json:"revision"`
			Text            string `json:"text"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionPath != "/remote/session.jsonl" || input.RuntimeEpoch != "remote-instance" || input.Revision != 7 || input.Text != "用户问题\n/new" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("bridge changed message or scope")
		}
		_ = json.NewEncoder(w).Encode(struct {
			controller.SessionSubmitReceipt
			Token string `json:"token"`
		}{controller.SessionSubmitReceipt{ProtocolVersion: 1, SessionPath: input.SessionPath, RuntimeEpoch: input.RuntimeEpoch, Revision: input.Revision, Accepted: true}, "PRIVATE config/key"})
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-submit"
	if got := controllerCall(b, "POST", target, bridgeSubmitBody, false); got.Code != 401 {
		t.Fatal("message bypassed bridge auth")
	}
	for _, body := range []string{`{}`, bridgeSubmitBody + " {}", strings.TrimSuffix(bridgeSubmitBody, "}") + `,"url":"http://127.0.0.1/private"}`, strings.Replace(bridgeSubmitBody, `"revision":7`, `"revision":0`, 1), strings.Replace(bridgeSubmitBody, `"revision":7`, `"revision":9007199254740992`, 1), strings.Repeat("x", (1<<20)+1)} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatal("malformed message accepted", got.Code)
		}
	}
	if got := controllerCall(b, "POST", target+"?session=other", bridgeSubmitBody, true); got.Code != 400 {
		t.Fatal("query changed selection")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid message dispatched")
	}
	got := controllerCall(b, "POST", target, bridgeSubmitBody, true)
	var response remoteControllerSessionSubmitResponse
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || !response.Controller.ReadOnly || !response.Receipt.Accepted || response.Receipt.RuntimeEpoch != "remote-instance" || response.Receipt.Revision != 7 || strings.Contains(got.Body.String(), "PRIVATE") || strings.Contains(got.Body.String(), "用户问题") || calls.Load() != 1 {
		t.Fatalf("bad message receipt: %d %s", got.Code, got.Body.String())
	}
	if _, exists := b.runtimes.Snapshot(); exists {
		t.Fatal("remote message created local foreground runtime")
	}
	if got := controllerCall(b, "POST", target, strings.Replace(bridgeSubmitBody, "/remote/session.jsonl", "/not-listed.jsonl", 1), true); got.Code != 404 || calls.Load() != 1 {
		t.Fatal("unlisted path dispatched message")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, bridgeSubmitBody, true); got.Code != 409 || calls.Load() != 1 {
		t.Fatal("closed owner dispatched message")
	}
}

func TestRemoteControllerSessionSubmitRefusalAndUnknownOutcome(t *testing.T) {
	for _, status := range []int{409, 500, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"private":"PRIVATE key/endpoint"}`)
			})
			view := attachController(t, b, "/project")
			got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-submit", bridgeSubmitBody, true)
			want := 502
			if status == 409 {
				want = 409
			}
			if got.Code != want || calls.Load() != 1 || strings.Contains(got.Body.String(), "PRIVATE") {
				t.Fatalf("bad message failure: %d %s", got.Code, got.Body.String())
			}
			if status != 409 && !strings.Contains(got.Body.String(), "remote_submit_unknown") {
				t.Fatal("unknown outcome lost classification")
			}
		})
	}
}

func TestRemoteControllerSessionSubmitRevokedDispatchStaysUnknown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
		_, _ = io.WriteString(w, `{"private":"PRIVATE old receipt"}`)
	})
	defer close(release)
	view := attachController(t, b, "/project")
	done := make(chan *httpResult, 1)
	go func() {
		got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-submit", bridgeSubmitBody, true)
		done <- &httpResult{status: got.Code, body: got.Body.String()}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("message did not dispatch")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case got := <-done:
		if got.status != 502 || !strings.Contains(got.body, "remote_submit_unknown") || strings.Contains(got.body, "PRIVATE") || calls.Load() != 1 {
			t.Fatal("revoked dispatch retried or published old receipt", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked dispatch did not finish")
	}
}
