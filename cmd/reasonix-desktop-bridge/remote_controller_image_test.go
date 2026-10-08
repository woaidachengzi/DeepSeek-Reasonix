package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func bridgeImagePixels(t *testing.T) string {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}
func TestRemoteControllerSessionImageBackendWorkspaceAndPrivacy(t *testing.T) {
	pixels := bridgeImagePixels(t)
	var reads atomic.Int32
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		var input map[string]string
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input) != 3 || input["sessionPath"] != "/remote/session.jsonl" || input["workspace"] != "/project/resolved" || input["source"] != "image.png" {
			t.Error("image did not use backend workspace")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"protocolVersion": 1, "sessionPath": "/remote/session.jsonl", "workspace": "/project/resolved", "image": map[string]any{"url": pixels, "mime": "image/png", "size": 1, "openHref": "file:///private-extra", "config": "private-extra"}, "token": "private-extra"})
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-image"
	body := `{"sessionPath":"/remote/session.jsonl","source":"image.png"}`
	if got := controllerCall(b, "POST", target, body, false); got.Code != 401 {
		t.Fatal("image bypassed bridge token")
	}
	for _, bad := range []string{`{}`, `{"sessionPath":"/remote/session.jsonl","source":"image.png","workspace":"/local/private"}`, `{"sessionPath":"bad\npath","source":"image.png"}`, `{"sessionPath":"/remote/session.jsonl","source":1}`, body + " {}"} {
		if got := controllerCall(b, "POST", target, bad, true); got.Code != 400 {
			t.Fatal("invalid image request accepted", got.Code)
		}
	}
	if reads.Load() != 0 {
		t.Fatal("invalid body reached remote")
	}
	got := controllerCall(b, "POST", target, body, true)
	var response remoteControllerSessionImageResponse
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &response) != nil || response.Controller != view || response.View.Image.URL != pixels || response.View.Workspace != view.Workspace || strings.Contains(got.Body.String(), "private-extra") || strings.Contains(got.Body.String(), "openHref") {
		t.Fatalf("image response scope/privacy: %d %s", got.Code, got.Body.String())
	}
	if _, present := b.runtimes.Snapshot(); present {
		t.Fatal("remote image created local runtime")
	}
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/not-listed.jsonl","source":"image.png"}`, true); got.Code != 404 || reads.Load() != 1 {
		t.Fatal("unlisted image was read")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	if got := controllerCall(b, "POST", target, body, true); got.Code != 409 || reads.Load() != 1 {
		t.Fatal("closed image handle revived")
	}
}

func TestRemoteControllerSessionImageRevokesPendingRead(t *testing.T) {
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
		_, _ = io.WriteString(w, `{"private":"late old pixels"}`)
	})
	defer close(abort)
	view := attachController(t, b, "/project")
	done := make(chan int, 1)
	go func() {
		done <- controllerCall(b, "POST", "/v1/remote/controllers/"+view.ID+"/session-image", `{"sessionPath":"/remote/session.jsonl","source":"image.png"}`, true).Code
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("image read not started")
	}
	controllerCall(b, "DELETE", "/v1/remote/controllers/"+view.ID, "", true)
	select {
	case status := <-done:
		if status != 409 {
			t.Fatal("late image published", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("image read did not cancel")
	}
}
