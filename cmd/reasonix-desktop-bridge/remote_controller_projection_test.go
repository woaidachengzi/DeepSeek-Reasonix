package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

const bridgeProjectionFixture = `{"protocolVersion":1,"sessionPath":"/remote/session.jsonl","readOnly":true,"initial":true,"history":[{"id":"old-user","role":"user","content":"old question","config":"private-extra"}],"userSuffix":[{"id":"new-user","role":"user","content":"new question"}],"activeTurnId":"owned-turn","turnStatus":"in_progress","replayAfterSeq":0,"replay":{"events":[{"kind":"steer","text":"guidance","itemId":"inbox-id","messageId":"guidance-id","turnId":"owned-turn","seq":1,"status":"in_progress","sessionPath":"/remote/session.jsonl","apiKey":"private-extra"}],"floorSeq":1,"latestSeq":1,"nextAfterSeq":1,"hasMore":false,"runtimeEpoch":"owned-epoch"},"token":"private-extra"}`

func TestRemoteControllerProjectionHostContinuation(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	var reads atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		var projection controller.SessionProjection
		json.Unmarshal([]byte(bridgeProjectionFixture), &projection)
		projection.Replay.LatestSequence = 2
		if r.URL.Query().Get("page") == "" {
			projection.PageToken = secret
			projection.Replay.HasMore = true
		} else {
			if r.URL.Query().Get("page") != secret || r.URL.Query().Get("after") != "1" {
				t.Error("not host-issued next cursor")
			}
			projection.Initial = false
			projection.History = []controller.HistoryMessage{}
			projection.UserSuffix = []controller.HistoryMessage{}
			projection.Replay.NextAfterSequence = 2
			projection.Replay.Events = []eventwire.Event{{Kind: "text", Text: "captured tail", Sequence: 2, TurnID: "owned-turn", Status: "in_progress", SessionPath: "/remote/session.jsonl"}}
		}
		json.NewEncoder(w).Encode(projection)
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-projection"
	initial := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, true)
	var first remoteControllerSessionProjectionResponse
	if initial.Code != 200 || json.Unmarshal(initial.Body.Bytes(), &first) != nil || !controllerHandle(first.NextPage) || first.Projection.PageToken != "" || strings.Contains(initial.Body.String(), secret) {
		t.Fatal("Serve token leaked or host page missing", initial.Body.String())
	}
	body, _ := json.Marshal(remoteControllerProjectionRequest{SessionPath: "/remote/session.jsonl", Continuation: first.NextPage})
	for range 2 {
		got := controllerCall(b, "POST", target, string(body), true)
		var next remoteControllerSessionProjectionResponse
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &next) != nil || next.NextPage != "" || next.Projection.Initial || next.Projection.Replay.NextAfterSequence != 2 {
			t.Fatal("final page/retry failed", got.Body.String())
		}
	}
	before := reads.Load()
	foreign := attachController(t, b, "/other")
	if got := controllerCall(b, "POST", "/v1/remote/controllers/"+foreign.ID+"/session-projection", string(body), true); got.Code != 409 || reads.Load() != before {
		t.Fatal("foreign owner page dispatched")
	}
	if got := controllerCall(b, "POST", target, strings.Replace(string(body), "/remote/session.jsonl", "/other", 1), true); got.Code != 409 || reads.Load() != before {
		t.Fatal("foreign path page dispatched")
	}
	connection := b.remoteSessions.getController(view.ID)
	connection.projectionMu.Lock()
	page := connection.projectionPages[first.NextPage]
	page.deadline = time.Now().Add(-time.Second)
	connection.projectionPages[first.NextPage] = page
	connection.projectionMu.Unlock()
	if got := controllerCall(b, "POST", target, string(body), true); got.Code != 409 || reads.Load() != before {
		t.Fatal("expired page dispatched")
	}
	connection.projectionMu.Lock()
	remaining := len(connection.projectionPages)
	connection.projectionMu.Unlock()
	if remaining != 0 {
		t.Fatal("expired metadata retained")
	}
}

func TestRemoteProjectionPageCapacityAndOriginalDeadline(t *testing.T) {
	c := &remoteControllerConnection{}
	deadline := time.Now().Add(time.Minute)
	for range maxRemoteProjectionPages {
		handle, err := c.retainProjectionPage(remoteProjectionPage{path: "owned", deadline: deadline}, "")
		if err != nil {
			t.Fatal(err)
		}
		page, ok := c.projectionPage(handle, "owned", time.Now())
		if !ok || !page.deadline.Equal(deadline) {
			t.Fatal("deadline extended")
		}
	}
	if _, err := c.retainProjectionPage(remoteProjectionPage{path: "owned", deadline: deadline}, ""); err == nil {
		t.Fatal("unbounded cache")
	}
	if _, ok := c.projectionPage(strings.Repeat("a", 32), "owned", time.Now().Add(2*time.Minute)); ok {
		t.Fatal("unknown/expired page")
	}
	if _, err := c.retainProjectionPage(remoteProjectionPage{path: "owned", deadline: deadline}, ""); err != nil {
		t.Fatal("expired slots not reclaimed", err)
	}
}

func TestRemoteProjectionIntermediateRetryReusesSlot(t *testing.T) {
	c := &remoteControllerConnection{}
	page := remoteProjectionPage{path: "owned", deadline: time.Now().Add(time.Minute)}
	parent, err := c.retainProjectionPage(page, "")
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.retainProjectionPage(page, parent)
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		retry, err := c.retainProjectionPage(page, parent)
		if err != nil || retry != next {
			t.Fatal("retry allocated a different slot", err)
		}
	}
	if len(c.projectionPages) != 2 {
		t.Fatal("unbounded retry metadata")
	}
	changed := page
	changed.deadline = changed.deadline.Add(time.Minute)
	if _, err := c.retainProjectionPage(changed, parent); err == nil {
		t.Fatal("original deadline extended")
	}
}

func TestRemoteControllerSessionProjectionBridgeScopeAndPrivacy(t *testing.T) {
	var reads atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Query().Get("session") != "/remote/session.jsonl" {
			t.Error("wrong selected session")
		}
		_, _ = io.WriteString(w, bridgeProjectionFixture)
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-projection"
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, false); got.Code != 401 {
		t.Fatal("missing bridge auth accepted")
	}
	for _, body := range []string{`{}`, `{"sessionPath":1}`, `{"sessionPath":"bad\npath"}`, `{"sessionPath":"/remote/session.jsonl","after":0}`, `{"sessionPath":"/remote/session.jsonl","page":"private"}`, `{"sessionPath":"/remote/session.jsonl","url":"http://private"}`, `{"sessionPath":"/remote/session.jsonl"} {}`} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatalf("invalid request accepted: %d", got.Code)
		}
	}
	if reads.Load() != 0 {
		t.Fatal("invalid request dispatched")
	}
	if got := controllerCall(b, "POST", target+"?after=0", `{"sessionPath":"/remote/session.jsonl"}`, true); got.Code != 400 || reads.Load() != 0 {
		t.Fatal("query replay accepted")
	}
	req := httptest.NewRequest("POST", target, strings.NewReader(`{"sessionPath":"/remote/session.jsonl"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Last-Event-ID", "1")
	recorder := httptest.NewRecorder()
	b.handler().ServeHTTP(recorder, req)
	if recorder.Code != 400 || reads.Load() != 0 {
		t.Fatal("header replay accepted")
	}
	got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, true)
	var response remoteControllerSessionProjectionResponse
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || len(response.Projection.Replay.Events) != 1 {
		t.Fatalf("initial cut: %d %s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "private-extra") || response.Projection.History[0].ID != "old-user" || response.Projection.UserSuffix[0].ID != "new-user" || response.Projection.Replay.Events[0].MessageID != "guidance-id" || response.Projection.Replay.Events[0].ItemID != "inbox-id" {
		t.Fatal("identity or privacy boundary lost")
	}
	if _, present := b.runtimes.Snapshot(); present {
		t.Fatal("display read created a local runtime")
	}
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/not-listed"}`, true); got.Code != 404 || reads.Load() != 1 {
		t.Fatal("unlisted session dispatched")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/remote/session.jsonl"}`, true); got.Code != 409 || reads.Load() != 1 {
		t.Fatal("closed controller dispatched")
	}
}

func TestRemoteControllerSessionProjectionBridgeRejectsMalformedRemote(t *testing.T) {
	for name, body := range map[string]string{
		"system":        strings.Replace(bridgeProjectionFixture, `"role":"user"`, `"role":"system"`, 1),
		"foreign-event": strings.Replace(bridgeProjectionFixture, `"turnId":"owned-turn"`, `"turnId":"other-turn"`, 1),
		"sequence-gap":  strings.Replace(bridgeProjectionFixture, `"seq":1`, `"seq":2`, 1),
		"false-ready":   strings.Replace(bridgeProjectionFixture, `"turnStatus":"in_progress"`, `"turnStatus":"completed"`, 1),
		"broken":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
			view := attachController(t, b, "/project")
			got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-projection", `{"sessionPath":"/remote/session.jsonl"}`, true)
			if got.Code != 502 || strings.Contains(got.Body.String(), "guidance") || strings.Contains(got.Body.String(), "private-extra") {
				t.Fatalf("invalid remote cut published: %d %s", got.Code, got.Body.String())
			}
		})
	}
}

func TestRemoteControllerSessionProjectionBridgeSafeErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 429, 502} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, "private-extra")
			})
			view := attachController(t, b, "/project")
			got := controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-projection", `{"sessionPath":"/remote/session.jsonl"}`, true)
			want := 502
			if status == 409 || status == 401 || status == 403 {
				want = 409
			}
			if got.Code != want || strings.Contains(got.Body.String(), "private-extra") {
				t.Fatalf("unsafe error: %d %s", got.Code, got.Body.String())
			}
		})
	}
}

func TestRemoteControllerSessionProjectionBridgeRevokesPendingRead(t *testing.T) {
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
		io.WriteString(w, bridgeProjectionFixture)
	})
	defer close(abort)
	view := attachController(t, b, "/project")
	done := make(chan int, 1)
	go func() {
		done <- controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-projection", `{"sessionPath":"/remote/session.jsonl"}`, true).Code
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not start")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case status := <-done:
		if status != 409 {
			t.Fatalf("late cut published: %d", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending read not cancelled")
	}
}
