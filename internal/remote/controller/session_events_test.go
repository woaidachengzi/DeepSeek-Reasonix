package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const eventSession = "/remote/中文 +&.jsonl"

func eventStreamFixture(t *testing.T, serve func(http.ResponseWriter, *http.Request)) (*Client, *atomic.Int32) {
	t.Helper()
	var opens atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != "owned" || r.Header.Get("Authorization") != "" {
			t.Error("host-only authentication missing")
		}
		switch r.URL.Path {
		case "/sessions":
			json.NewEncoder(w).Encode([]Session{{Path: eventSession}})
		case "/events":
			opens.Add(1)
			if r.URL.RawQuery != "all=1" || r.Method != "GET" || r.Header.Get("Accept") != "text/event-stream" {
				t.Error("unexpected stream route")
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			serve(w, r)
		default:
			t.Error("unexpected route")
		}
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "owned")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &opens
}

func TestSessionEventsSelectedTypedFramesAndNoImplicitForeground(t *testing.T) {
	c, opens := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ": connected\r\n\r\ndata: {\"kind\":\"text\",\"text\":\"FOREIGN\",\"sessionPath\":\"/other\"}\n\n")
		io.WriteString(w, "data: {\"kind\":\"text\",\"text\":\"UNTAGGED\"}\n\n")
		fmt.Fprintf(w, "id: private-replay-id\nretry: 1\ndata: {\"kind\":\"text\",\ndata: \"text\":\"owned delta\",\"turnId\":\"turn\",\"seq\":7,\"sessionPath\":%q,\"token\":\"PRIVATE\",\"tool\":{\"id\":\"tool\",\"config\":\"PRIVATE\"}}\n\n", eventSession)
	})
	stream, err := c.SessionEvents(context.Background(), eventSession)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	frame, err := stream.Next()
	if err != nil || frame.Text != "owned delta" || frame.Sequence != 7 || frame.SessionPath != eventSession {
		t.Fatalf("selected frame: %+v %v", frame, err)
	}
	data, _ := json.Marshal(frame)
	if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "replay") {
		t.Fatal("untyped private fields forwarded")
	}
	if _, err := stream.Next(); !errors.Is(err, ErrStreamInterrupted) {
		t.Fatalf("EOF must require reconciliation: %v", err)
	}
	if opens.Load() != 1 {
		t.Fatal("stream silently retried")
	}
}

func TestSessionEventsRejectsMalformedBoundedFrames(t *testing.T) {
	for _, body := range []string{
		"data: null\n\n", "data: {broken}\n\n", "data: {\"kind\":\"text\",\"text\":\"\xff\"}\n\n",
		fmt.Sprintf("data: {\"kind\":\"new-unknown\",\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"text\",\"seq\":9007199254740992,\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"text\",\"turnId\":\"bad\\u0000\",\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"steer\",\"messageId\":\"bad\\u0000\",\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"text\",\"messageId\":\"owned-message\",\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"user_message_admitted\",\"sessionPath\":%q}\n\n", eventSession),
		fmt.Sprintf("data: {\"kind\":\"host_input_admitted\",\"messageId\":\"bad\\u0000\",\"sessionPath\":%q}\n\n", eventSession),
		"data: {\"kind\":\"text\"}\n", ":" + strings.Repeat("x", sessionEventFrameMax) + "\n\n",
		strings.Repeat(":"+strings.Repeat("x", 1024)+"\n", 8192) + "\n",
	} {
		t.Run("invalid", func(t *testing.T) {
			c, _ := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
			stream, err := c.SessionEvents(context.Background(), eventSession)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if _, err := stream.Next(); !errors.Is(err, ErrResponse) {
				t.Fatalf("bad frame accepted: %v", err)
			}
		})
	}
}

func TestSessionEventsPreservesSteerMessageAndInboxIdentities(t *testing.T) {
	c, _ := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "data: {\"kind\":\"steer\",\"text\":\"owned guidance\",\"messageId\":\"owned-message\",\"itemId\":\"owned-inbox\",\"sessionPath\":%q}\n\n", eventSession)
		fmt.Fprintf(w, "data: {\"kind\":\"steer\",\"text\":\"legacy\",\"sessionPath\":%q}\n\n", eventSession)
	})
	stream, err := c.SessionEvents(context.Background(), eventSession)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	frame, err := stream.Next()
	if err != nil || frame.MessageID != "owned-message" || frame.ItemID != "owned-inbox" {
		t.Fatalf("associated: %+v %v", frame, err)
	}
	legacy, err := stream.Next()
	if err != nil || legacy.MessageID != "" || legacy.ItemID != "" || legacy.Text != "legacy" {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
}

func TestSessionEventsCanonicalAdmissionsAreIdentityOnly(t *testing.T) {
	for _, kind := range []string{"user_message_admitted", "host_input_admitted"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "data: {\"kind\":%q,\"messageId\":\"canonical-input\",\"turnId\":\"owned\",\"seq\":7,\"sessionPath\":%q,\"sessionCurrent\":true,\"text\":\"PRIVATE\",\"itemId\":\"action\"}\n\n", kind, eventSession)
			})
			stream, err := c.SessionEvents(context.Background(), eventSession)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			frame, err := stream.Next()
			if err != nil || frame.Kind != kind || frame.MessageID != "canonical-input" || frame.TurnID != "owned" || frame.Sequence != 7 || !frame.SessionCurrent || frame.Text != "" || frame.ItemID != "" {
				t.Fatalf("canonical readiness rejected or exposed body/authority: %+v %v", frame, err)
			}
		})
	}
}

func TestSessionEventsAdmissionAndAuthFailure(t *testing.T) {
	c, opens := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); io.WriteString(w, "PRIVATE") })
	for _, path := range []string{"", "bad\x00", "/not-listed"} {
		if stream, err := c.SessionEvents(context.Background(), path); err == nil || stream != nil {
			t.Fatal("invalid selection admitted")
		}
	}
	if _, err := c.SessionEvents(nil, eventSession); !errors.Is(err, ErrClient) {
		t.Fatal("nil operation accepted")
	}
	if opens.Load() != 0 {
		t.Fatal("invalid selection opened stream")
	}
	if _, err := c.SessionEvents(context.Background(), eventSession); !errors.Is(err, ErrUnavailable) || !c.Closed() {
		t.Fatal("expired auth did not revoke controller")
	}
	if opens.Load() != 1 {
		t.Fatal("auth failure retried")
	}
}

func TestSessionEventsCancellationClosesActualHTTPBody(t *testing.T) {
	for _, mode := range []string{"operation", "owner", "stream"} {
		t.Run(mode, func(t *testing.T) {
			ended := make(chan struct{})
			c, _ := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, ": connected\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(ended)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := c.SessionEvents(ctx, eventSession)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			done := make(chan error, 1)
			go func() { _, err := stream.Next(); done <- err }()
			switch mode {
			case "operation":
				cancel()
			case "owner":
				c.Close()
			case "stream":
				stream.Close()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled read published")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("reader retained cancelled stream")
			}
			select {
			case <-ended:
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP body survived cancellation")
			}
			stream.Close()
			if mode != "owner" && c.Closed() {
				t.Fatal("one stream closed shared controller owner")
			}
		})
	}
}

func TestSessionEventsBufferedPublicationAndMediaBoundary(t *testing.T) {
	for _, mode := range []string{"owner", "operation"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "data: {\"kind\":\"text\",\"text\":\"must not publish\",\"sessionPath\":%q}\n\n", eventSession)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := c.SessionEvents(ctx, eventSession)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if mode == "owner" {
				c.Close()
			} else {
				cancel()
			}
			frame, err := stream.Next()
			if err == nil || frame.Text != "" {
				t.Fatal("revoked scope published buffered frame")
			}
		})
	}
	for _, media := range []string{"", "application/json", "text/event-stream;broken"} {
		t.Run("media", func(t *testing.T) {
			c, opens := eventStreamFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", media)
				io.WriteString(w, "PRIVATE")
			})
			if _, err := c.SessionEvents(context.Background(), eventSession); !errors.Is(err, ErrUnavailable) {
				t.Fatal("non-SSE response accepted")
			}
			if opens.Load() != 1 {
				t.Fatal("bad media retried")
			}
		})
	}
}
