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

func reclaimObservationInput() DrivingReclaimObservationRequest {
	r := drivingRequest()
	return DrivingReclaimObservationRequest{ProtocolVersion: 1, Scope: r.Scope, Key: r.Key}
}
func reclaimObservationClient(t *testing.T, owner context.Context, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "reclaim-fixture", Path: "/"})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != "reclaim-fixture" || r.Method != "POST" || r.URL.Path != "/desktop/driving-reclaim-observation" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("reclaim observer changed original route/auth")
		}
		var input DrivingReclaimObservationRequest
		if json.NewDecoder(r.Body).Decode(&input) != nil || input != reclaimObservationInput() {
			t.Error("original capture/key changed")
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

func TestDrivingReclaimObservationStrictOneSignalAndNoBody(t *testing.T) {
	for _, raw := range []string{
		`{"protocolVersion":1,"kind":"reclaimed"}`,
		`{"protocolVersion":1,"kind":"reclaimed","key":"private-key"}`,
		`{"protocolVersion":1,"kind":"reclaimed","text":"private-body"}`,
		`{"protocolVersion":1,"kind":"turn_done"}`,
		`{"protocolVersion":1,"kind":"ready"}`,
		`{"protocolVersion":2,"kind":"reclaimed"}`,
		strings.Repeat("x", 1025),
		`{"protocolVersion":1,"kind":"reclaimed"}` + "\n" + `{"protocolVersion":1,"kind":"reclaimed"}`,
	} {
		t.Run(raw[:min(len(raw), 55)], func(t *testing.T) {
			var calls atomic.Int32
			c := reclaimObservationClient(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n"+raw+"\n")
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s, err := c.ObserveDrivingReclaim(ctx, reclaimObservationInput())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			frame, err := s.Next()
			valid := raw == `{"protocolVersion":1,"kind":"reclaimed"}` || strings.Contains(raw, "\n")
			if valid && (err != nil || frame.Kind != "reclaimed") || !valid && err == nil {
				t.Fatal(frame, err)
			}
			if valid {
				if _, err := s.Next(); err == nil {
					t.Fatal("signal repeated or replayed")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("failed stream retried")
			}
		})
	}
}

func TestDrivingReclaimObservationRetirementAfterSignalInterruptsActualHTTP(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	defer cancel()
	ended := make(chan struct{})
	c := reclaimObservationClient(t, owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"protocolVersion\":1,\"kind\":\"ready\"}\n{\"protocolVersion\":1,\"kind\":\"reclaimed\"}\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	})
	s, err := c.ObserveDrivingReclaim(context.Background(), reclaimObservationInput())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Next(); err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	go func() { _, err := s.Next(); read <- err }()
	cancel()
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("retired source produced a signal")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked original source read survived")
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP operation survived original client owner")
	}
}

func TestDrivingReclaimObservationHTTPFailuresDoNotRetryOrLeak(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			c := reclaimObservationClient(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/foreign-private-target")
				http.Error(w, "private-diagnostic", status)
			})
			_, err := c.ObserveDrivingReclaim(context.Background(), reclaimObservationInput())
			if err == nil || strings.Contains(err.Error(), "private") || calls.Load() != 1 {
				t.Fatal("failure retried or exposed body", err)
			}
			if (status == 401 || status == 403) && !errors.Is(err, ErrClosed) {
				t.Fatal("auth failure retained owner", err)
			}
			if (status == 404 || status == 409) && !errors.Is(err, ErrDrivingChanged) {
				t.Fatal("stale grant not reported changed", err)
			}
		})
	}
}

func TestDrivingReclaimObservationInvalidGrantNeverDispatches(t *testing.T) {
	var calls atomic.Int32
	c := reclaimObservationClient(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	valid := reclaimObservationInput()
	for _, change := range []func(*DrivingReclaimObservationRequest){
		func(r *DrivingReclaimObservationRequest) { r.Key = "raw-private-id" },
		func(r *DrivingReclaimObservationRequest) { r.ProtocolVersion = 2 },
		func(r *DrivingReclaimObservationRequest) { r.Scope.Revision = 0 },
		func(r *DrivingReclaimObservationRequest) { r.Scope.ControlVersion = 9_007_199_254_740_991 },
		func(r *DrivingReclaimObservationRequest) { r.Scope.RuntimeEpoch = "" },
	} {
		input := valid
		change(&input)
		if _, err := c.ObserveDrivingReclaim(context.Background(), input); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid grant not rejected", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid grant dispatched")
	}
}
