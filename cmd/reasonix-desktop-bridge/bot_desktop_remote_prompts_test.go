package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

type remotePromptFixture struct {
	catalogue  *previewDesktopCatalogue
	service    *previewDesktopRemotePrompts
	entry      previewDesktopCatalogueEntry
	scope      controller.SessionPromptScope
	route      bot.DesktopWatchRoute
	writes     atomic.Int32
	fail       atomic.Bool
	block      atomic.Bool
	release    chan struct{}
	generation atomic.Int32
	requests   chan controller.SessionPromptRequest
}

func remotePromptTestFixture(t *testing.T, kind string) *remotePromptFixture {
	t.Helper()
	f := &remotePromptFixture{route: desktopWatchTestRoute(), requests: make(chan controller.SessionPromptRequest, 8), release: make(chan struct{})}
	f.generation.Store(1)
	path := "/remote/live.jsonl"
	epoch := "captured-" + path
	f.scope = controller.SessionPromptScope{SessionPath: path, RuntimeEpoch: epoch, TurnID: "turn", PromptID: "private-id", PromptRuntimeEpoch: "routing", Kind: kind}
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]controller.Session{{Path: path}})
	}, func(w http.ResponseWriter, r *http.Request) {
		currentEpoch := epoch
		if f.generation.Load() != 1 {
			currentEpoch = "replacement-" + path
		}
		switch r.URL.Path {
		case "/runtime-states":
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: path}}, strings.TrimSuffix(currentEpoch, "-"+path)))
		case "/desktop/session-pending":
			var input struct {
				ProtocolVersion int `json:"protocolVersion"`
				controller.SessionPendingScope
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.RuntimeEpoch != currentEpoch || input.SessionPath != path {
				w.WriteHeader(409)
				return
			}
			prompt := controller.SessionPendingPrompt{Scope: f.scope}
			switch kind {
			case "ask":
				prompt.Ask = &eventwire.Ask{ID: f.scope.PromptID, TurnID: "turn", Questions: []eventwire.AskQuestion{{ID: "q", Prompt: "private question", Options: []eventwire.AskOption{{Label: "A"}, {Label: "B"}}}}}
			case "mcp":
				prompt.MCPInteraction = &eventwire.MCPInteraction{ID: f.scope.PromptID, TurnID: "turn", Mode: "form", RequestedSchema: json.RawMessage("{}")}
			default:
				prompt.Approval = &eventwire.Approval{ID: f.scope.PromptID, TurnID: "turn", Kind: kind, Subject: "private subject"}
			}
			_ = json.NewEncoder(w).Encode(controller.SessionPendingView{ProtocolVersion: 1, SessionPendingScope: input.SessionPendingScope, Revision: 1, TurnID: "turn", Prompts: []controller.SessionPendingPrompt{prompt}})
		case "/desktop/session-prompt":
			f.writes.Add(1)
			var input struct {
				ProtocolVersion int `json:"protocolVersion"`
				controller.SessionPromptRequest
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.SessionPromptScope != f.scope {
				t.Error("decision changed captured scope")
				w.WriteHeader(409)
				return
			}
			f.requests <- input.SessionPromptRequest
			if f.block.Load() {
				select {
				case <-f.release:
				case <-r.Context().Done():
					return
				}
			}
			if f.fail.Load() {
				w.WriteHeader(502)
				return
			}
			_ = json.NewEncoder(w).Encode(controller.SessionPromptReceipt{ProtocolVersion: 1, SessionPromptScope: input.SessionPromptScope, Resolved: true})
		default:
			t.Error("legacy prompt fallback")
			w.WriteHeader(400)
		}
	})
	attachController(t, b, "/owned-remote")
	f.catalogue = newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
	t.Cleanup(f.catalogue.Close)
	entries, err := f.catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	f.entry = entries[0]
	f.service = newPreviewDesktopRemotePrompts(f.catalogue)
	t.Cleanup(f.service.Close)
	return f
}

func TestPreviewRemotePromptConcurrentDecisionAndCloseCancelSingleAttempt(t *testing.T) {
	f := remotePromptTestFixture(t, "ask")
	defer close(f.release)
	id, _, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	f.block.Store(true)
	command := bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: "answer", TargetID: id, AnswerText: "2"}
	finished := make(chan error, 1)
	go func() {
		_, err := f.service.ExecuteDesktopCommand(context.Background(), command)
		finished <- err
	}()
	notificationWait(t, f.requests)
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 1 {
		t.Fatal("concurrent duplicate dispatched")
	}
	f.service.Close()
	if err := notificationWait(t, finished); err != errPreviewDesktopBinding || f.writes.Load() != 1 {
		t.Fatal("close failed to cancel the sole in-flight attempt", err)
	}
	if _, _, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope); err == nil {
		t.Fatal("closed service rearmed ticket")
	}
}

func TestPreviewRemotePromptTicketsFiveDecisionsRemainExactAndSingleShot(t *testing.T) {
	for _, tc := range []struct{ kind, action, text string }{
		{"ask", "answer", "2"}, {"approval", "deny", ""}, {"plan", "plan", "revise_plan private feedback"},
		{"recovery", "recovery", "continue_task"}, {"mcp", "mcp", "accept {\"approved\":true,\"label\":\"two  spaces\"}"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f := remotePromptTestFixture(t, tc.kind)
			id, payload, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
			if err != nil || id == f.scope.PromptID || !strings.HasPrefix(id, "rp-") {
				t.Fatal(id, err)
			}
			payload[0] = '!'
			command := bot.DesktopCommand{Route: f.route, ActorID: "operator", TargetID: id, Action: tc.action, AnswerText: tc.text}
			wrong := command
			wrong.ActorID = "other-admin"
			if _, err := f.service.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
				t.Fatal("actor inherited ticket")
			}
			wrong = command
			wrong.Route.ChatType = bot.ChatGroup
			if _, err := f.service.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
				t.Fatal("chat audience inherited ticket")
			}
			wrong = command
			wrong.TargetID = f.scope.PromptID
			if _, err := f.service.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
				t.Fatal("raw ID granted decision")
			}
			if tc.kind != "approval" {
				wrong = command
				wrong.Action = "approve"
				if _, err := f.service.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
					t.Fatal("generic approval accepted specialized prompt")
				}
			}
			refreshed, fresh, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
			if err != nil || refreshed != id || !json.Valid(fresh) || f.writes.Load() != 0 {
				t.Fatal("display refresh changed/burned ticket", err)
			}
			if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			request := notificationWait(t, f.requests)
			answer, err := controller.DecodeSessionPromptAnswer(request)
			if err != nil || request.SessionPromptScope != f.scope {
				t.Fatal(request, err)
			}
			if tc.kind == "ask" && answer.Questions[0].Selected[0] != "B" {
				t.Fatal("lost actual captured options")
			}
			if tc.kind == "mcp" && answer.Content["label"] != "two  spaces" {
				t.Fatal("MCP content whitespace changed")
			}
			if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 1 {
				t.Fatal("decision retried")
			}
		})
	}
}

func TestPreviewRemotePromptUnknownCannotRearmAndOldOwnerCannotDispatch(t *testing.T) {
	f := remotePromptTestFixture(t, "ask")
	id, _, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	command := bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: "answer", TargetID: id, AnswerText: "unknown=2"}
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 0 {
		t.Fatal("malformed answer dispatched")
	}
	f.fail.Store(true)
	command.AnswerText = "2"
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("unknown reported confirmed")
	}
	f.service.mu.Lock()
	spent := f.service.tickets[id]
	spent.issued = time.Now().Add(-2 * previewDesktopPromptLifetime)
	f.service.tickets[id] = spent
	f.service.mu.Unlock()
	refreshed, _, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
	if err != nil || refreshed != id {
		t.Fatal("unknown refresh changed ticket", err)
	}
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 1 {
		t.Fatal("refresh rearmed unknown")
	}
	// A failed directory cut retires handles, not the uncertain decision.
	f.catalogue.mu.Lock()
	clear(f.catalogue.entries)
	f.catalogue.mu.Unlock()
	entries, err := f.catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 1 || entries[0].handle == f.entry.handle {
		t.Fatal("fixture did not replace retired directory handle", err)
	}
	f.entry = entries[0]
	refreshed, _, err = f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope)
	if err != nil || refreshed != id {
		t.Fatal("directory recovery rearmed unknown decision", err)
	}
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 1 {
		t.Fatal("retired handle rearmed spent decision")
	}
	other, _, err := f.service.Issue(context.Background(), f.route, "second-operator", f.entry, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	f.generation.Store(2)
	command.TargetID, command.ActorID = other, "second-operator"
	if _, err := f.service.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 1 {
		t.Fatal("replacement owner adopted old ticket")
	}
	f.service.Close()
	if _, _, err := f.service.Issue(context.Background(), f.route, "operator", f.entry, f.scope); err == nil {
		t.Fatal("closed service issued")
	}
}
