package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func promptTestRequest(kind, answer string) SessionPromptRequest {
	return SessionPromptRequest{SessionPromptScope{ownedViewPath, "controller-epoch", "turn", "prompt", "", kind}, json.RawMessage(answer)}
}

func TestSessionPromptStrictDiscriminatedAnswer(t *testing.T) {
	for kind, bodies := range map[string][]string{
		"ask":      {`{"questions":[]}`, `{"questions":[{"questionId":"q","selected":["自定义\n回答"]}]}`},
		"approval": {`{"allow":false}`, `{"allow":true,"session":true}`, `{"allow":true,"persist":true}`},
		"plan":     {`{"action":"start_execution"}`, `{"action":"revise_plan","feedback":"请修改"}`, `{"action":"exit_plan"}`},
		"recovery": {`{"action":"continue"}`, `{"action":"continue_task"}`, `{"action":"revise","feedback":"调整"}`},
		"mcp":      {`{"action":"accept","content":{"answer":true}}`, `{"action":"decline"}`, `{"action":"cancel"}`},
	} {
		for _, body := range bodies {
			if _, err := DecodeSessionPromptAnswer(promptTestRequest(kind, body)); err != nil {
				t.Fatalf("valid %s %s: %v", kind, body, err)
			}
		}
	}
	for _, test := range []struct{ kind, body string }{
		{"ask", `{}`}, {"ask", `{"questions":null}`}, {"ask", `{"questions":[],"allow":true}`},
		{"ask", `{"questions":[{"questionId":"q","selected":[],"private":"x"}]}`},
		{"ask", `{"questions":[{"questionId":"q","selected":[null]}]}`},
		{"ask", `{"questions":[{"questionId":"q","selected":[],"selected":["A"]}]}`},
		{"ask", `{"questions":[{"questionId":"q","questionId":"other","selected":[]}]}`},
		{"ask", `{"questions":[{"questionId":"q","selected":[]},{"questionId":"q","selected":[]}]}`},
		{"approval", `{}`}, {"approval", `{"allow":null}`}, {"approval", `{"allow":"true"}`},
		{"approval", `{"allow":false,"persist":true}`}, {"approval", `{"allow":true,"session":true,"persist":true}`},
		{"approval", `{"allow":true,"content":null}`},
		{"approval", `{"allow":false,"allow":true}`},
		{"plan", `{"action":"continue"}`}, {"plan", `{"action":"start_execution","allow":true}`},
		{"recovery", `{"action":"stop"}`}, {"recovery", `{"action":"revise","feedback":"\u0000"}`},
		{"mcp", `{"action":"accept","content":[]}`}, {"mcp", `{"action":"decline","content":{}}`},
		{"mcp", `{"action":"accept","content":null}`}, {"unknown", `{}`},
	} {
		if _, err := DecodeSessionPromptAnswer(promptTestRequest(test.kind, test.body)); !errors.Is(err, ErrResponse) {
			t.Fatalf("invalid union admitted: %+v / %v", test, err)
		}
	}
	for _, field := range []string{"path", "epoch", "turn", "prompt", "routing"} {
		input := promptTestRequest("approval", `{"allow":false}`)
		switch field {
		case "path":
			input.SessionPath = ""
		case "epoch":
			input.RuntimeEpoch = ""
		case "turn":
			input.TurnID = ""
		case "prompt":
			input.PromptID = ""
		case "routing":
			input.PromptRuntimeEpoch = "bad\nidentity"
		}
		if _, err := DecodeSessionPromptAnswer(input); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid identity admitted", field)
		}
	}
	input := promptTestRequest("plan", `{"action":"revise_plan","feedback":"`+strings.Repeat("x", 4097)+`"}`)
	if _, err := DecodeSessionPromptAnswer(input); !errors.Is(err, ErrResponse) {
		t.Fatal("feedback budget lost")
	}
}

func promptFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "owned", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("reasonix_token"); err != nil || cookie.Value != "owned" || r.Header.Get("Authorization") != "" {
			t.Error("private authentication lost")
		}
		if r.URL.Path == "/sessions" {
			rows := []Session{}
			if listed {
				rows = append(rows, Session{Path: ownedViewPath})
			}
			_ = json.NewEncoder(w).Encode(rows)
			return
		}
		calls.Add(1)
		if r.URL.Path != "/desktop/session-prompt" || r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("decision fallback or identity URL leak")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &calls
}

func TestResolveSessionPromptTypedReceiptAndOneDispatch(t *testing.T) {
	input := promptTestRequest("approval", `{"allow":false}`)
	c, calls := promptFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProtocolVersion int `json:"protocolVersion"`
			SessionPromptRequest
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || body.ProtocolVersion != 1 || body.SessionPromptScope != input.SessionPromptScope || string(body.Answer) != string(input.Answer) {
			t.Error("decision identity changed")
		}
		_ = json.NewEncoder(w).Encode(struct {
			SessionPromptReceipt
			Private string `json:"private"`
		}{SessionPromptReceipt{1, input.SessionPromptScope, true}, "PRIVATE config/key"})
	})
	receipt, err := c.ResolveSessionPrompt(context.Background(), input)
	if err != nil || !receipt.Resolved || receipt.SessionPromptScope != input.SessionPromptScope || calls.Load() != 1 {
		t.Fatal(receipt, err, calls.Load())
	}
	encoded, _ := json.Marshal(receipt)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("private receipt leak")
	}
	input.Answer = json.RawMessage(`{"allow":true,"action":"accept"}`)
	if _, err := c.ResolveSessionPrompt(context.Background(), input); !errors.Is(err, ErrResponse) || calls.Load() != 1 {
		t.Fatal("invalid union dispatched", err)
	}
}

func TestResolveSessionPromptNoRetryAndUnknownOutcome(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		want   error
	}{
		{409, "PRIVATE", ErrPromptChanged}, {404, "", ErrPromptChanged}, {500, "PRIVATE", ErrPromptOutcomeUnknown},
		{200, `{"protocolVersion":1,"resolved":true,"promptId":"other"}`, ErrPromptOutcomeUnknown},
		{200, strings.Repeat("x", (64<<10)+1), ErrPromptOutcomeUnknown}, {401, "PRIVATE", ErrClosed},
	} {
		c, calls := promptFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.status)
			_, _ = w.Write([]byte(test.body))
		})
		_, err := c.ResolveSessionPrompt(context.Background(), promptTestRequest("ask", `{"questions":[]}`))
		if !errors.Is(err, test.want) || calls.Load() != 1 || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unsafe result/retry", err, calls.Load())
		}
	}
	c, calls := promptFixture(t, false, func(w http.ResponseWriter, r *http.Request) { t.Error("unlisted mutation") })
	if _, err := c.ResolveSessionPrompt(context.Background(), promptTestRequest("approval", `{"allow":false}`)); !errors.Is(err, ErrSessionNotListed) || calls.Load() != 0 {
		t.Fatal(err, calls.Load())
	}
}

func TestResolveSessionPromptOwnerRevokedAfterDispatchIsUnknown(t *testing.T) {
	input := promptTestRequest("approval", `{"allow":false}`)
	entered, release := make(chan struct{}), make(chan struct{})
	c, calls := promptFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_ = json.NewEncoder(w).Encode(SessionPromptReceipt{1, input.SessionPromptScope, true})
	})
	defer close(release)
	returned := make(chan error, 1)
	go func() { _, err := c.ResolveSessionPrompt(context.Background(), input); returned <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decision was not dispatched")
	}
	c.Close()
	select {
	case err := <-returned:
		if !errors.Is(err, ErrPromptOutcomeUnknown) || calls.Load() != 1 {
			t.Fatal("revoked dispatch published success or retried", err, calls.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner revocation did not end operation")
	}
	if _, err := c.ResolveSessionPrompt(context.Background(), input); !errors.Is(err, ErrClosed) || calls.Load() != 1 {
		t.Fatal("closed owner dispatched again", err)
	}
}
