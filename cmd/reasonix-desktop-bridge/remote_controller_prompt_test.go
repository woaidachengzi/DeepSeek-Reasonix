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

const bridgePromptBody = `{"sessionPath":"/remote/session.jsonl","runtimeEpoch":"remote-instance","turnId":"turn","promptId":"prompt","promptRuntimeEpoch":"","kind":"approval","answer":{"allow":false}}`

func TestRemoteControllerSessionPromptScopePrivacyAndNoLocalRuntime(t *testing.T) {
	var calls atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			controller.SessionPromptRequest
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionPath != "/remote/session.jsonl" || input.RuntimeEpoch != "remote-instance" || input.TurnID != "turn" || input.PromptID != "prompt" || input.PromptRuntimeEpoch != "" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("decision identity changed")
		}
		if _, err := controller.DecodeSessionPromptAnswer(input.SessionPromptRequest); err != nil {
			t.Error("answer changed", err)
		}
		_ = json.NewEncoder(w).Encode(struct {
			controller.SessionPromptReceipt
			Private string `json:"private"`
		}{controller.SessionPromptReceipt{ProtocolVersion: 1, SessionPromptScope: input.SessionPromptScope, Resolved: true}, "PRIVATE key/answer"})
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-prompt"
	if got := controllerCall(b, "POST", target, bridgePromptBody, false); got.Code != 401 {
		t.Fatal("decision bypassed bridge auth")
	}
	for _, body := range []string{`{}`, bridgePromptBody + ` {}`, strings.TrimSuffix(bridgePromptBody, "}") + `,"url":"http://127.0.0.1/private"}`, strings.Replace(bridgePromptBody, `"allow":false`, `"allow":false,"action":"accept"`, 1), strings.Replace(bridgePromptBody, `"allow":false`, `"allow":false,"allow":true`, 1), strings.Replace(bridgePromptBody, `"runtimeEpoch":"remote-instance"`, `"runtimeEpoch":""`, 1), strings.Repeat("x", (256<<10)+1)} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatal("malformed decision admitted", got.Code)
		}
	}
	if got := controllerCall(b, "POST", target+"?session=other", bridgePromptBody, true); got.Code != 400 {
		t.Fatal("query changed selection")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid decision dispatched")
	}
	for kind, answer := range map[string]string{"ask": `{"questions":[]}`, "approval": `{"allow":false}`, "plan": `{"action":"exit_plan"}`, "recovery": `{"action":"revise","feedback":"修改"}`, "mcp": `{"action":"decline"}`} {
		body := strings.Replace(bridgePromptBody, `"kind":"approval"`, `"kind":"`+kind+`"`, 1)
		body = strings.Replace(body, `{"allow":false}`, answer, 1)
		got := controllerCall(b, "POST", target, body, true)
		var response remoteControllerSessionPromptResponse
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || !response.Controller.ReadOnly || !response.Receipt.Resolved || response.Receipt.Kind != kind || response.Receipt.RuntimeEpoch != "remote-instance" || response.Receipt.PromptID != "prompt" || strings.Contains(got.Body.String(), "PRIVATE") || strings.Contains(got.Body.String(), "answer") {
			t.Fatalf("bad decision receipt: %d %s", got.Code, got.Body.String())
		}
	}
	if calls.Load() != 5 {
		t.Fatal("decision retried", calls.Load())
	}
	if _, exists := b.runtimes.Snapshot(); exists {
		t.Fatal("remote decision created local runtime")
	}
	if got := controllerCall(b, "POST", target, strings.Replace(bridgePromptBody, "/remote/session.jsonl", "/not-listed.jsonl", 1), true); got.Code != 404 || calls.Load() != 5 {
		t.Fatal("unlisted decision dispatched")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, bridgePromptBody, true); got.Code != 409 || calls.Load() != 5 {
		t.Fatal("closed owner dispatched decision")
	}
}

func TestRemoteControllerSessionPromptRefusalAndUnknownOutcome(t *testing.T) {
	for _, status := range []int{409, 500, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"private":"PRIVATE endpoint/key"}`)
			})
			view := attachController(t, b, "/project")
			got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-prompt", bridgePromptBody, true)
			want := 502
			if status == 409 {
				want = 409
			}
			if got.Code != want || calls.Load() != 1 || strings.Contains(got.Body.String(), "PRIVATE") {
				t.Fatal("unsafe decision failure", got.Code, got.Body.String())
			}
			if status != 409 && !strings.Contains(got.Body.String(), "remote_prompt_unknown") {
				t.Fatal("unknown classification lost")
			}
		})
	}
}

func TestRemoteControllerSessionPromptRevokedDispatchStaysUnknown(t *testing.T) {
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
		got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-prompt", bridgePromptBody, true)
		done <- &httpResult{status: got.Code, body: got.Body.String()}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decision did not dispatch")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case got := <-done:
		if got.status != 502 || !strings.Contains(got.body, "remote_prompt_unknown") || strings.Contains(got.body, "PRIVATE") || calls.Load() != 1 {
			t.Fatal("revoked decision retried/published", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoked dispatch did not finish")
	}
}
