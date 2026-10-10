package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func drivingRequest() SessionDrivingRequest {
	return SessionDrivingRequest{ProtocolVersion: 1, Action: "acquire", Scope: SessionDrivingScope{SessionPendingScope: pendingFixtureScope(), Revision: 1}, Key: "0123456789abcdef0123456789abcdef"}
}

func drivingClientFixture(t *testing.T, owner context.Context, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "driving-fixture", Path: "/"})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != "driving-fixture" || r.Method != "POST" || r.URL.Path != "/desktop/session-driving" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("driving used another route/owner/fallback")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(owner, context.Background(), s.URL, "isolated")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestClientSessionDrivingFiveActions(t *testing.T) {
	for _, action := range []string{"capture", "acquire", "state", "release", "input"} {
		t.Run(action, func(t *testing.T) {
			input := drivingRequest()
			input.Action = action
			if action == "capture" {
				input.Scope.Revision = 0
				input.Key = ""
			}
			if action == "input" {
				input.InputRevision = 7
				input.Text = "!literal question"
			}
			var calls atomic.Int32
			c := drivingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var got SessionDrivingRequest
				if json.NewDecoder(r.Body).Decode(&got) != nil || got != input {
					t.Error("original holder/input scope changed")
				}
				out := SessionDrivingReceipt{ProtocolVersion: 1, Action: action, Scope: input.Scope, Active: action == "acquire" || action == "state", Accepted: action == "input", Released: action == "release"}
				if action == "capture" {
					out.Scope.Revision = 3
					out.Scope.ControlVersion = 8
				}
				_ = json.NewEncoder(w).Encode(out)
			})
			got, err := c.SessionDriving(context.Background(), input)
			if err != nil || got.Action != action || calls.Load() != 1 {
				t.Fatal(got, err, calls.Load())
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), input.Key) && input.Key != "" {
				t.Fatal("private holder key echoed")
			}
		})
	}
}

func TestClientSessionDrivingUnknownNeverRetriesOrLeaks(t *testing.T) {
	for _, scenario := range []string{"500", "redirect", "mixed", "flags", "key", "missing-version", "oversize", "diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			input := drivingRequest()
			var calls atomic.Int32
			c := drivingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				out := SessionDrivingReceipt{ProtocolVersion: 1, Action: input.Action, Scope: input.Scope, Active: true}
				switch scenario {
				case "500":
					http.Error(w, "private-api-key", 500)
					return
				case "redirect":
					http.Redirect(w, r, "/private-key-destination", 307)
					return
				case "mixed":
					out.Scope.ControlVersion++
				case "flags":
					out.Accepted = true
				case "key":
					raw, _ := json.Marshal(out)
					_, _ = io.WriteString(w, strings.TrimSuffix(string(raw), "}")+`,"key":"private-api-key"}`)
					return
				case "missing-version":
					_, _ = io.WriteString(w, `{"protocolVersion":1,"action":"acquire","scope":{"sessionPath":"/private/owned.jsonl","runtimeEpoch":"core-instance","revision":1},"active":true}`)
					return
				case "oversize":
					_, _ = io.WriteString(w, strings.Repeat("x", (256<<10)+1))
					return
				case "diagnostic":
					_, _ = io.WriteString(w, "private-api-key")
					return
				}
				_ = json.NewEncoder(w).Encode(out)
			})
			_, err := c.SessionDriving(context.Background(), input)
			if !errors.Is(err, ErrDrivingUnknown) || strings.Contains(err.Error(), "private-api-key") || calls.Load() != 1 {
				t.Fatal("unknown outcome retried or leaked", err, calls.Load())
			}
		})
	}
}

func TestClientSessionDrivingCancellationAndAuthentication(t *testing.T) {
	for _, mode := range []string{"owner", "operation", "401", "403", "409", "404"} {
		t.Run(mode, func(t *testing.T) {
			owner, stopOwner := context.WithCancel(context.Background())
			defer stopOwner()
			op, stopOp := context.WithCancel(context.Background())
			defer stopOp()
			started := make(chan struct{})
			var calls atomic.Int32
			c := drivingClientFixture(t, owner, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch mode {
				case "401":
					w.WriteHeader(401)
				case "403":
					w.WriteHeader(403)
				case "409":
					w.WriteHeader(409)
				case "404":
					w.WriteHeader(404)
				default:
					_, _ = io.Copy(io.Discard, r.Body)
					close(started)
					<-r.Context().Done()
				}
			})
			result := make(chan error, 1)
			go func() { _, err := c.SessionDriving(op, drivingRequest()); result <- err }()
			if mode == "owner" || mode == "operation" {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("call not dispatched")
				}
				if mode == "owner" {
					stopOwner()
				} else {
					stopOp()
				}
			}
			var err error
			select {
			case err = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("owned call did not cancel")
			}
			want := ErrDrivingUnknown
			if mode == "401" || mode == "403" {
				want = ErrClosed
				if !c.Closed() {
					t.Fatal("auth loss did not close owner")
				}
			}
			if mode == "409" || mode == "404" {
				want = ErrDrivingChanged
			}
			if !errors.Is(err, want) || calls.Load() != 1 {
				t.Fatal(err, calls.Load())
			}
		})
	}
}

func TestClientSessionDrivingInvalidInputDoesNotDispatch(t *testing.T) {
	var calls atomic.Int32
	c := drivingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, mutate := range []func(*SessionDrivingRequest){
		func(r *SessionDrivingRequest) { r.Action = "unknown" },
		func(r *SessionDrivingRequest) { r.Key = strings.ToUpper(r.Key) },
		func(r *SessionDrivingRequest) { r.Key = "raw-session-id" },
		func(r *SessionDrivingRequest) { r.Scope.Revision = 0 },
		func(r *SessionDrivingRequest) { r.Scope.ControlVersion = 9_007_199_254_740_991 },
		func(r *SessionDrivingRequest) { r.Text = "unexpected acquire text" },
		func(r *SessionDrivingRequest) {
			r.Action = "input"
			r.InputRevision = 1
			r.Text = strings.Repeat("x", (64<<10)+1)
		},
		func(r *SessionDrivingRequest) { r.Action = "input"; r.InputRevision = 1; r.Text = "\x00" },
		func(r *SessionDrivingRequest) { r.Action = "capture"; r.Key = "" },
	} {
		input := drivingRequest()
		mutate(&input)
		if _, err := c.SessionDriving(context.Background(), input); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid call accepted", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input dispatched", calls.Load())
	}
}
