package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const ownedEventPath = "/remote/中文 +&.jsonl"

func openBridgeEventStream(t *testing.T, base, id string, ctx context.Context) *http.Response {
	t.Helper()
	body, _ := json.Marshal(remoteControllerSessionViewRequest{SessionPath: ownedEventPath})
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/remote/controllers/"+id+"/session-events", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRemoteControllerSessionEventsActualSSHScopeAndProjection(t *testing.T) {
	ended := make(chan struct{})
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"path": ownedEventPath}})
	}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, ": ready\n\ndata: {\"kind\":\"text\",\"text\":\"FOREIGN\",\"sessionPath\":\"/other\"}\n\n")
		fmt.Fprintf(w, "data: {\"kind\":\"reasoning\",\"text\":\"owned reasoning\",\"turnId\":\"owned-turn\",\"seq\":1,\"sessionPath\":%q,\"token\":\"PRIVATE\"}\n\n", ownedEventPath)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	})
	view := attachController(t, b, "/project")
	server := httptest.NewServer(b.handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp := openBridgeEventStream(t, server.URL, view.ID, ctx)
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("bridge stream did not open")
	}
	scanner := bufio.NewScanner(resp.Body)
	var got remoteControllerSessionEvent
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if strings.Contains(line, "PRIVATE") || strings.Contains(line, "FOREIGN") || strings.Contains(line, "owned-controller-secret") {
			t.Fatal("unowned/private frame reached native channel")
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &got) != nil {
			t.Fatal("invalid bridge event envelope")
		}
		break
	}
	if got.ProtocolVersion != 1 || got.Controller != view || got.SessionPath != ownedEventPath || got.Event.SessionPath != ownedEventPath || got.Event.Kind != "reasoning" || got.Event.Sequence != 1 || got.Event.TurnID != "owned-turn" {
		t.Fatalf("wrong owner envelope: %+v", got)
	}
	b.remoteSessions.closeController(view.ID)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data:") {
			t.Fatal("revoked handle emitted another frame")
		}
	}
	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("revoked SSH owner retained backend stream")
	}
}

func TestRemoteControllerSessionEventsNarrowAdmission(t *testing.T) {
	var opens atomic.Int32
	b, _, _, auth := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"path": ownedEventPath}})
	}, func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		t.Error("invalid request opened backend events")
	})
	view := attachController(t, b, "/project")
	target := "/v1/remote/controllers/" + view.ID + "/session-events"
	for _, body := range []string{`{}`, `{"sessionPath":"bad\u0000"}`, `{"sessionPath":"/selected","url":"http://private"}`, `{"sessionPath":"/selected","token":"private"}`, `{"sessionPath":"/selected","workspace":"/local"}`, `{"sessionPath":"/selected","afterSequence":1}`} {
		if got := controllerCall(b, "POST", target, body, true); got.Code != 400 {
			t.Fatalf("wide request admitted: %d", got.Code)
		}
	}
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/missing"}`, true); got.Code != 404 {
		t.Fatal("unlisted session admitted")
	}
	if got := controllerCall(b, "POST", target+"?afterSequence=1", `{"sessionPath":"/selected"}`, true); got.Code != 400 {
		t.Fatal("query replay admitted")
	}
	if got := controllerCall(b, "POST", target, `{"sessionPath":"/selected"}`, false); got.Code != 401 {
		t.Fatal("auth bypassed")
	}
	if got := controllerCall(b, "GET", target, "", true); got.Code != 405 {
		t.Fatal("wide method admitted")
	}
	req := httptest.NewRequest("POST", target, strings.NewReader(`{"sessionPath":"/selected"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Last-Event-ID", "1")
	recorder := httptest.NewRecorder()
	b.handler().ServeHTTP(recorder, req)
	if recorder.Code != 400 || opens.Load() != 0 || auth.Load() != 1 {
		t.Fatal("replay parameters/invalid requests reached backend stream")
	}
}

func TestRemoteControllerSessionEventsSubscriptionLimitAndCancellation(t *testing.T) {
	var opens, ended atomic.Int32
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"path": ownedEventPath}})
	}, func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, ": ready\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		ended.Add(1)
	})
	view := attachController(t, b, "/project")
	server := httptest.NewServer(b.handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := openBridgeEventStream(t, server.URL, view.ID, ctx)
	defer first.Body.Close()
	second := openBridgeEventStream(t, server.URL, view.ID, ctx)
	defer second.Body.Close()
	if first.StatusCode != 200 || second.StatusCode != 200 {
		t.Fatal("owned stream slots failed")
	}
	third := openBridgeEventStream(t, server.URL, view.ID, ctx)
	third.Body.Close()
	if third.StatusCode != 429 || opens.Load() != 2 {
		t.Fatal("subscription cap did not bound backend streams")
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for ended.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ended.Load() != 2 {
		t.Fatal("cancelled native subscribers retained remote readers")
	}
	connection := b.remoteSessions.getController(view.ID)
	if connection == nil {
		t.Fatal("one subscription cancellation revoked shared controller")
	}
	for len(connection.eventSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(connection.eventSlots) != 0 {
		t.Fatal("cancelled subscribers retained admission slots")
	}
}

type failedEventWriter struct {
	*httptest.ResponseRecorder
	initial bool
}

func (w *failedEventWriter) Write(data []byte) (int, error) {
	if w.initial || strings.HasPrefix(string(data), "data:") {
		return 0, errors.New("owned native reader disconnected")
	}
	return w.ResponseRecorder.Write(data)
}

func TestRemoteControllerSessionEventsOutputFailureCancelsUpstream(t *testing.T) {
	for _, initial := range []bool{true, false} {
		t.Run(fmt.Sprintf("initial=%t", initial), func(t *testing.T) {
			ended := make(chan struct{})
			b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode([]map[string]any{{"path": ownedEventPath}})
			}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"kind\":\"text\",\"text\":\"owned\",\"sessionPath\":%q}\n\n", ownedEventPath)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(ended)
			})
			view := attachController(t, b, "/project")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			body, _ := json.Marshal(remoteControllerSessionViewRequest{SessionPath: ownedEventPath})
			req := httptest.NewRequest("POST", "/v1/remote/controllers/"+view.ID+"/session-events", strings.NewReader(string(body))).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+testToken)
			writer := &failedEventWriter{httptest.NewRecorder(), initial}
			b.handler().ServeHTTP(writer, req)
			select {
			case <-ended:
			case <-ctx.Done():
				t.Fatal("output failure retained upstream reader")
			}
			connection := b.remoteSessions.getController(view.ID)
			if ctx.Err() != nil || connection == nil || len(connection.eventSlots) != 0 {
				t.Fatal("output failure relied on request timeout or retained admission slot")
			}
		})
	}
}

func TestRemoteControllerSessionEventsNativeHTTP10Framing(t *testing.T) {
	ended := make(chan struct{})
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"path": ownedEventPath}})
	}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"kind\":\"text\",\"text\":\"owned\",\"sessionPath\":%q}\n\n", ownedEventPath)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	})
	view := attachController(t, b, "/project")
	server := httptest.NewServer(b.handler())
	defer server.Close()
	connection, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(3 * time.Second))
	body, _ := json.Marshal(remoteControllerSessionViewRequest{SessionPath: ownedEventPath})
	_, err = fmt.Fprintf(connection, "POST /v1/remote/controllers/%s/session-events HTTP/1.0\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", view.ID, server.Listener.Addr(), testToken, len(body), body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: "POST"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Proto != "HTTP/1.0" || len(response.TransferEncoding) != 0 || response.ContentLength != -1 || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("native HTTP/1.0 request was not close-delimited SSE")
	}
	scanner := bufio.NewScanner(response.Body)
	found := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			var frame remoteControllerSessionEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &frame) != nil || frame.Event.Text != "owned" || frame.SessionPath != ownedEventPath {
				t.Fatal("invalid native close-delimited frame")
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("native stream missing selected event")
	}
	connection.Close()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("native socket close retained upstream body")
	}
}
