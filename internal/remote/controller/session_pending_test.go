package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reasonix/internal/eventwire"
)

func pendingFixtureScope() SessionPendingScope {
	return SessionPendingScope{SessionPath: "/private/owned.jsonl", RuntimeEpoch: "core-instance"}
}
func pendingFixtureView(kind string) SessionPendingView {
	scope := pendingFixtureScope()
	prompt := SessionPendingPrompt{Scope: SessionPromptScope{SessionPath: scope.SessionPath, RuntimeEpoch: scope.RuntimeEpoch, TurnID: "turn", PromptID: "prompt", PromptRuntimeEpoch: "routing", Kind: kind}}
	switch kind {
	case "ask":
		prompt.Ask = &eventwire.Ask{ID: "prompt", TurnID: "turn", Questions: []eventwire.AskQuestion{{ID: "q", Prompt: "private question"}}}
	case "mcp":
		prompt.MCPInteraction = &eventwire.MCPInteraction{ID: "prompt", TurnID: "turn", Mode: "form", RequestedSchema: json.RawMessage("{}")}
	default:
		prompt.Approval = &eventwire.Approval{ID: "prompt", TurnID: "turn", Kind: kind, Subject: "private subject"}
	}
	return SessionPendingView{ProtocolVersion: 1, SessionPendingScope: scope, Revision: 1, TurnID: "turn", Prompts: []SessionPendingPrompt{prompt}}
}

func pendingClientFixture(t *testing.T, owner context.Context, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			http.SetCookie(w, &http.Cookie{Name: "reasonix_token", Value: "pending-fixture", Path: "/"})
			w.WriteHeader(204)
			return
		}
		cookie, err := r.Cookie("reasonix_token")
		if err != nil || cookie.Value != "pending-fixture" || r.Method != "POST" || r.URL.Path != "/desktop/session-pending" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("pending reader used another owner/route/fallback")
		}
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			SessionPendingScope
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.ProtocolVersion != 1 || input.SessionPendingScope != pendingFixtureScope() {
			t.Error("pending scope changed")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := Connect(owner, context.Background(), server.URL, "isolated")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestClientSessionPendingFiveKindsAndPrivateUnknownFields(t *testing.T) {
	for _, kind := range []string{"ask", "approval", "plan", "recovery", "mcp"} {
		t.Run(kind, func(t *testing.T) {
			expected := pendingFixtureView(kind)
			client := pendingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) {
				raw, _ := json.Marshal(expected)
				_, _ = io.WriteString(w, strings.TrimSuffix(string(raw), "}")+",\"apiKey\":\"hidden-key\",\"reasoning\":\"hidden-thought\"}")
			})
			got, err := client.ReadSessionPending(context.Background(), pendingFixtureScope())
			if err != nil || len(got.Prompts) != 1 || got.Prompts[0].Scope.Kind != kind {
				t.Fatal(got, err)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), "hidden") {
				t.Fatal("unrelated private data forwarded")
			}
			if _, err := client.ReadSessionPending(nil, pendingFixtureScope()); !errors.Is(err, ErrClient) {
				t.Fatal(err)
			}
		})
	}
}

func TestClientSessionPendingRejectsMixedScopeAndPayloads(t *testing.T) {
	for _, mutate := range []func(*SessionPendingView){
		func(v *SessionPendingView) { v.ProtocolVersion = 0 }, func(v *SessionPendingView) { v.RuntimeEpoch = "other" },
		func(v *SessionPendingView) { v.Prompts = nil }, func(v *SessionPendingView) { v.Revision = 0 },
		func(v *SessionPendingView) { v.Prompts[0].Scope.RuntimeEpoch = "other" },
		func(v *SessionPendingView) { v.Prompts[0].Scope.TurnID = "other" },
		func(v *SessionPendingView) { v.Prompts[0].Scope.Kind = "plan" },
		func(v *SessionPendingView) { v.Prompts[0].Ask.ID = "other" },
		func(v *SessionPendingView) { v.Prompts[0].Ask.TurnID = "other" },
		func(v *SessionPendingView) { v.Prompts[0].Approval = &eventwire.Approval{} },
		func(v *SessionPendingView) { v.Prompts = append(v.Prompts, v.Prompts[0]) },
		func(v *SessionPendingView) { v.Prompts = make([]SessionPendingPrompt, 33) },
		func(v *SessionPendingView) { v.Prompts[0].Ask.Questions[0].Prompt = strings.Repeat("x", 64<<10) },
	} {
		value := pendingFixtureView("ask")
		mutate(&value)
		client := pendingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(value) })
		if _, err := client.ReadSessionPending(context.Background(), pendingFixtureScope()); !errors.Is(err, ErrResponse) {
			t.Fatal("mixed/oversized view accepted", err)
		}
	}
}

func TestClientSessionPendingOwnerCancellationAndNoFallback(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, abort := make(chan struct{}), make(chan struct{})
	defer close(abort)
	client := pendingClientFixture(t, owner, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-abort:
		}
	})
	done := make(chan error, 1)
	go func() { _, err := client.ReadSessionPending(context.Background(), pendingFixtureScope()); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("read not started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read not canceled")
	}
	for _, code := range []int{404, 409, 401} {
		c := pendingClientFixture(t, context.Background(), func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		_, err := c.ReadSessionPending(context.Background(), pendingFixtureScope())
		if code == 401 {
			if !errors.Is(err, ErrClosed) || !c.Closed() {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrPromptChanged) {
			t.Fatal(err)
		}
	}
}
