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

const bridgeCancelBody = `{"sessionPath":"/remote/session.jsonl","runtimeEpoch":"remote-instance","turnId":"remote-turn"}`

func TestRemoteControllerSessionCancelScopePrivacyAndNoLocalRuntime(t *testing.T) {
	var calls atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			controller.SessionCancelScope
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionPath != "/remote/session.jsonl" || input.RuntimeEpoch != "remote-instance" || input.TurnID != "remote-turn" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("bridge changed scope or sent arbitrary fields")
		}
		_ = json.NewEncoder(w).Encode(struct {
			controller.SessionCancelReceipt
			Token string `json:"token"`
		}{controller.SessionCancelReceipt{ProtocolVersion: 1, SessionPath: input.SessionPath, RuntimeEpoch: input.RuntimeEpoch, TurnID: input.TurnID, Cancelled: true}, "PRIVATE config/key"})
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-cancel"
	if got := controllerCall(b, "POST", target, bridgeCancelBody, false); got.Code != 401 {
		t.Fatal("stop bypassed bridge authentication")
	}
	for _, body := range []string{`{}`, bridgeCancelBody + " {}", strings.TrimSuffix(bridgeCancelBody, "}") + `,"url":"http://127.0.0.1/private"}`, `{"sessionPath":"/remote/session.jsonl","runtimeEpoch":"","turnId":"remote-turn"}`, `{"sessionPath":"/remote/session.jsonl","runtimeEpoch":"x","turnId":"bad\nturn"}`} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatalf("malformed stop accepted: %d", got.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid stop reached remote")
	}
	got := controllerCall(b, "POST", target, bridgeCancelBody, true)
	var response remoteControllerSessionCancelResponse
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || !response.Controller.ReadOnly || !response.Receipt.Cancelled || response.Receipt.RuntimeEpoch != "remote-instance" || response.Receipt.TurnID != "remote-turn" || strings.Contains(got.Body.String(), "PRIVATE") || calls.Load() != 1 {
		t.Fatalf("bad scoped stop receipt: %d %s", got.Code, got.Body.String())
	}
	if _, exists := b.runtimes.Snapshot(); exists {
		t.Fatal("remote stop created or cancelled a local foreground runtime")
	}
	if got := controllerCall(b, "POST", target, strings.Replace(bridgeCancelBody, "/remote/session.jsonl", "/not-listed.jsonl", 1), true); got.Code != 404 || calls.Load() != 1 {
		t.Fatal("unlisted path dispatched stop")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, bridgeCancelBody, true); got.Code != 409 || calls.Load() != 1 {
		t.Fatal("closed controller dispatched stop")
	}
}

func TestRemoteControllerSessionCancelRefusalAndUnknownOutcome(t *testing.T) {
	for _, status := range []int{409, 500, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"private":"PRIVATE key/endpoint"}`)
			})
			view := attachController(t, b, "/project")
			got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-cancel", bridgeCancelBody, true)
			want := 502
			if status == 409 {
				want = 409
			}
			if got.Code != want || calls.Load() != 1 || strings.Contains(got.Body.String(), "PRIVATE") {
				t.Fatalf("bad stop failure: %d %s", got.Code, got.Body.String())
			}
			if status != 409 && !strings.Contains(got.Body.String(), "remote_cancel_unknown") {
				t.Fatal("indeterminate mutation lost its outcome classification")
			}
		})
	}
}

func TestRemoteControllerSessionCancelRevokedDispatchStaysUnknown(t *testing.T) {
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
		got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-cancel", bridgeCancelBody, true)
		done <- &httpResult{status: got.Code, body: got.Body.String()}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not dispatch")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case got := <-done:
		if got.status != 502 || !strings.Contains(got.body, "remote_cancel_unknown") || strings.Contains(got.body, "PRIVATE") || calls.Load() != 1 {
			t.Fatal("revoked dispatch retried or published stale receipt", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked stop did not return")
	}
}

type httpResult struct {
	status int
	body   string
}
