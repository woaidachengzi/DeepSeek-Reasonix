package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/eventwire"
	"reasonix/internal/mcpinteraction"
)

func TestPreviewDesktopPromptTicketsActualAskNoReplayOrRetarget(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	provider, endpoint := newOwnedCommandFixtureProvider(t)
	config := fmt.Sprintf(`default_model="local/alpha"
[desktop]
provider_access=["local"]
[[providers]]
name="local"
kind="preview-owned-command-test"
base_url=%q
models=["alpha","beta"]
default="alpha"
`, endpoint)
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	requests, decisions := make(chan event.Event, 4), make(chan event.Event, 4)
	factory := &settingsControllerFactory{controllerFactory: newControllerFactory(nil)}
	factory.base.Sink = event.FuncSink(func(e event.Event) {
		if e.Kind == event.AskRequest || e.Kind == event.MCPInteractionRequest {
			requests <- e
		}
		if e.Kind == event.PromptAnswered {
			decisions <- e
		}
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "prompt-ticket", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	initial, ok := manager.CommandSnapshot()
	if !ok {
		t.Fatal("missing actual controller")
	}
	catalogue := newPreviewDesktopCatalogue(manager, nil)
	defer catalogue.Close()
	entries, err := catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal("actual directory missing", err)
	}
	entry := entries[0]
	if err := manager.SubmitOwned(context.Background(), initial, "private gated ask"); err != nil {
		t.Fatal(err)
	}
	call := awaitOwnedCommand(t, provider.calls)
	answers := make(chan []event.AskAnswer, 1)
	go func() {
		value, _ := factory.latest.controller.Ask(call.ctx, []event.AskQuestion{{ID: "q", Prompt: "private question", Options: []event.AskOption{{Label: "A"}, {Label: "B"}}}})
		answers <- value
	}()
	awaitOwnedCommand(t, requests)
	view, ok := manager.CommandSnapshot()
	if !ok || len(view.State.Pending) != 1 {
		t.Fatal("missing actual pending prompt")
	}
	prompt := view.State.Pending[0]
	pendingRead, err := catalogue.ReadPending(context.Background(), entry)
	if err != nil || len(pendingRead.Prompts) != 1 || pendingRead.Prompts[0].Ask == nil || pendingRead.Prompts[0].Scope.PromptID != prompt.ID {
		t.Fatal("unified reader lost actual Ask", err)
	}
	for _, mutate := range []func(*desktopbridge.OwnedPrompt){
		func(p *desktopbridge.OwnedPrompt) { p.ID += "-wrong" }, func(p *desktopbridge.OwnedPrompt) { p.TurnID += "-wrong" },
		func(p *desktopbridge.OwnedPrompt) { p.RuntimeEpoch += "-wrong" }, func(p *desktopbridge.OwnedPrompt) { p.Kind = "approval" },
	} {
		wrong := prompt
		mutate(&wrong)
		if _, err := manager.ReadOwnedPrompt(context.Background(), view.Scope, wrong); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
			t.Fatal("wrong prompt read accepted", err)
		}
	}
	tickets := newPreviewDesktopPrompts(manager)
	defer tickets.Close()
	route := desktopWatchTestRoute()
	id, payload, err := tickets.Issue(context.Background(), route, "operator", view.Scope, prompt)
	if err != nil || id == prompt.ID {
		t.Fatal("ticket not independent of core ID", err)
	}
	var wire eventwire.Event
	if err := json.Unmarshal(payload, &wire); err != nil || wire.Ask == nil || wire.Ask.Questions[0].Options[1].Label != "B" {
		t.Fatal("snapshot lost actual prompt", err)
	}
	payload[0] = '!'
	refreshed, freshPayload, err := tickets.Issue(context.Background(), route, "operator", view.Scope, prompt)
	if err != nil || refreshed != id || !json.Valid(freshPayload) {
		t.Fatal("display refresh changed identity or aliased consumer", err)
	}
	if len(requests) != 0 || len(decisions) != 0 {
		t.Fatal("private snapshot replayed into ordinary sink")
	}
	command := bot.DesktopCommand{Route: route, ActorID: "operator", Action: "answer", TargetID: id, AnswerText: "2"}
	// Simulate an old unknown attempt while the actual prompt remains pending.
	tickets.mu.Lock()
	original := tickets.tickets[id]
	spent := original
	spent.attempted = true
	spent.issued = time.Now().Add(-2 * previewDesktopPromptLifetime)
	tickets.tickets[id] = spent
	tickets.mu.Unlock()
	if refreshed, _, err := tickets.Issue(context.Background(), route, "operator", view.Scope, prompt); err != nil || refreshed != id {
		t.Fatal("expiry rearmed unknown local decision", err)
	}
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), command); err == nil || len(answers) != 0 {
		t.Fatal("expired spent ticket dispatched")
	}
	// Restore the unattempted fixture for the real Controller decision below.
	tickets.mu.Lock()
	tickets.tickets[id] = original
	tickets.mu.Unlock()
	wrong := command
	wrong.TargetID = prompt.ID
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
		t.Fatal("raw core ID granted authority")
	}
	wrong = command
	wrong.Route.ChatType = bot.ChatGroup
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
		t.Fatal("another audience inherited ticket")
	}
	wrong = command
	wrong.ActorID = "another-admin"
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), wrong); err == nil {
		t.Fatal("another actor inherited ticket")
	}
	wrong = command
	wrong.AnswerText = "unknown=2"
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), wrong); !errors.Is(err, bot.ErrDesktopAskAnswer) {
		t.Fatal("invalid answer silently dropped", err)
	}
	if len(answers) != 0 {
		t.Fatal("invalid command resolved actual Ask")
	}
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	actual := awaitOwnedCommand(t, answers)
	if len(actual) != 1 || actual[0].QuestionID != "q" || len(actual[0].Selected) != 1 || actual[0].Selected[0] != "B" {
		t.Fatalf("actual answers=%#v", actual)
	}
	awaitOwnedCommand(t, decisions)
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("ticket retried after decision")
	}
	if _, err := manager.ReadOwnedPrompt(context.Background(), view.Scope, prompt); err == nil {
		t.Fatal("resolved prompt remained readable")
	}
	// The next prompt in the same live turn is a distinct specialized surface;
	// an Ask ticket cannot drift to it, and generic approve cannot accept MCP.
	mcpResult := make(chan mcpinteraction.Result, 1)
	go func() {
		result, _ := factory.latest.controller.Interact(call.ctx, mcpinteraction.Request{Server: "private-fixture", Mode: "form", Message: "Confirm?", RequestedSchema: json.RawMessage(`{"type":"object","properties":{"approved":{"type":"boolean"},"label":{"type":"string"}},"required":["approved"]}`)})
		mcpResult <- result
	}()
	awaitOwnedCommand(t, requests)
	mcpView, ok := manager.CommandSnapshot()
	if !ok || len(mcpView.State.Pending) != 1 || mcpView.State.Pending[0].Kind != "mcp" {
		t.Fatal("actual MCP identity missing")
	}
	pendingRead, err = catalogue.ReadPending(context.Background(), entry)
	if err != nil || len(pendingRead.Prompts) != 1 || pendingRead.Prompts[0].MCPInteraction == nil || pendingRead.Prompts[0].Ask != nil || pendingRead.Prompts[0].Scope.PromptID != mcpView.State.Pending[0].ID || len(requests) != 0 || len(decisions) != 0 {
		t.Fatal("unified reader replayed or adopted previous Ask", err)
	}
	mcpID, _, err := tickets.Issue(context.Background(), route, "operator", mcpView.Scope, mcpView.State.Pending[0])
	if err != nil || mcpID == id {
		t.Fatal("MCP reused Ask capability", err)
	}
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("Ask ticket adopted next MCP prompt")
	}
	mcpCommand := bot.DesktopCommand{Route: route, ActorID: "operator", Action: "approve", TargetID: mcpID}
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), mcpCommand); err == nil {
		t.Fatal("generic approval accepted MCP")
	}
	mcpCommand.Action, mcpCommand.AnswerText = "mcp", `decline {"approved":true}`
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), mcpCommand); err == nil {
		t.Fatal("decline with content accepted")
	}
	if len(mcpResult) != 0 {
		t.Fatal("invalid MCP decision woke waiter")
	}
	mcpCommand.AnswerText = `accept {"approved":true,"label":"two  spaces"}`
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), mcpCommand); err != nil {
		t.Fatal(err)
	}
	actualMCP := awaitOwnedCommand(t, mcpResult)
	if actualMCP.Action != mcpinteraction.ActionAccept || actualMCP.Content["approved"] != true || actualMCP.Content["label"] != "two  spaces" {
		t.Fatalf("actual MCP result=%#v", actualMCP)
	}
	awaitOwnedCommand(t, decisions)
	close(provider.finish)
	awaitDesktopDriverIdle(t, manager)
	if _, err := manager.SetSessionModel(context.Background(), "prompt-ticket", "local/beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("same-ID replacement retargeted ticket")
	}
	if _, _, err := tickets.Issue(context.Background(), route, "operator", view.Scope, prompt); err == nil {
		t.Fatal("old owner issued a new capability")
	}
	tickets.Close()
	tickets.Close()
	if len(tickets.tickets) != 0 {
		t.Fatal("close retained private prompts")
	}
}
