package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"reasonix/internal/bot"
	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

// Production registration remains off. This fixture exercises real gateway
// ingress/permissions/parser/replies; accidental legacy fallback panics.
type desktopCommandsGatewayFixture struct {
	bot.DesktopBridge
	commands *previewDesktopCommands
}

func (f *desktopCommandsGatewayFixture) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	return f.commands.ExecuteDesktopCommand(ctx, command)
}
func (f *desktopCommandsGatewayFixture) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	return f.commands.DesktopTakeoverActive(route, actor)
}

func TestPreviewDesktopCommandsWatchStoreOwnershipAndUnknownPersistence(t *testing.T) {
	f := remotePromptTestFixture(t, "ask")
	store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
	t.Cleanup(store.Close)
	var failed bool
	store.persist = func(w previewDesktopWatcher, enabled bool) error {
		if failed {
			return errors.New("private credential path")
		}
		return nil
	}
	h := newPreviewDesktopCommandsWithWatch(f.catalogue, store)
	command := bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: "watch", TargetID: "on"}
	if out, err := h.ExecuteDesktopCommand(context.Background(), command); err != nil || !strings.Contains(out, "已开启") || strings.Contains(out, f.route.ChatID) {
		t.Fatal(out, err)
	}
	command.TargetID = "status"
	if out, err := h.ExecuteDesktopCommand(context.Background(), command); err != nil || !strings.Contains(out, "已开启") {
		t.Fatal(out, err)
	}
	h.Close()
	if !store.Watching(f.route) {
		t.Fatal("gateway command close deleted shared persisted watch")
	}
	h = newPreviewDesktopCommandsWithWatch(f.catalogue, store)
	defer h.Close()
	failed = true
	command.TargetID = "off"
	if out, err := h.ExecuteDesktopCommand(context.Background(), command); err != errPreviewDesktopBinding || out != "" {
		t.Fatal("persistence failure leaked or confirmed", out, err)
	}
	if store.Watching(f.route) {
		t.Fatal("failed persistence silently restored old on state")
	}
	failed = false
	command.TargetID = "status"
	if out, err := h.ExecuteDesktopCommand(context.Background(), command); err != nil || !strings.Contains(out, "未开启") {
		t.Fatal(out, err)
	}
	command.AnswerText = "unexpected"
	if _, err := h.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("watch accepted extra body")
	}
}

func commandsPromptTicket(t *testing.T, text string) string {
	t.Helper()
	for _, field := range strings.Fields(text) {
		if (strings.HasPrefix(field, "rp-") && len(field) == 35) || (strings.HasPrefix(field, "p-") && len(field) == 34) {
			return field
		}
	}
	t.Fatal("no opaque decision ticket in reply", text)
	return ""
}

func TestPreviewDesktopCommandsActualGatewayStatusPendingAndDecision(t *testing.T) {
	f := remotePromptTestFixture(t, "ask")
	catalogue := newPreviewDesktopCatalogue(catalogueActualManager(t), f.catalogue.remotes)
	defer catalogue.Close()
	commands := newPreviewDesktopCommands(catalogue)
	defer commands.Close()
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	route := f.route
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Desktop: &desktopCommandsGatewayFixture{commands: commands}, Enabled: map[bot.Platform]bool{route.Platform: true},
		ConnectionAccess: map[string]bot.AccessConfig{route.ConnectionID: {Enabled: true, Users: []string{"member"}, Admins: []string{"owner", "second-admin"}}},
	}, []bot.AdapterBinding{{ID: route.ConnectionID, Domain: route.Domain, Platform: route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()
	send := func(actor, text string, chat bot.ChatType) string {
		t.Helper()
		adapter.messages <- bot.InboundMessage{Platform: route.Platform, ConnectionID: route.ConnectionID, Domain: route.Domain, ChatID: route.ChatID, ChatType: chat, UserID: actor, Text: text}
		return notificationWait(t, adapter.sent).Text
	}
	status := send("owner", "/desktop status", bot.ChatDM)
	if !strings.Contains(status, "本地") || !strings.Contains(status, "远程") || strings.Contains(status, "/remote/") || strings.Contains(status, "private") {
		t.Fatal("status omitted owners or disclosed identity", status)
	}
	entries, err := catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	var remote previewDesktopCatalogueEntry
	for _, entry := range entries {
		if entry.remote != nil {
			remote = entry
		}
	}
	denied := send("member", "/desktop pending "+remote.handle, bot.ChatDM)
	if !strings.Contains(denied, "没有") || strings.Contains(denied, "private") || len(commands.remote.tickets) != 0 {
		t.Fatal("member bypassed role gate", denied)
	}
	shared := send("owner", "/desktop pending "+remote.handle, bot.ChatGroup)
	if strings.Contains(shared, "private") || strings.Contains(shared, "/remote/") {
		t.Fatal("group received private prompt", shared)
	}
	groupTicket := commandsPromptTicket(t, shared)
	private := send("owner", "/desktop pending "+remote.handle, bot.ChatDM)
	if !strings.Contains(private, "private question") || !strings.Contains(private, "2. B") || strings.Contains(private, "private-id") || strings.Contains(private, "routing") {
		t.Fatal("private prompt lost content or leaked routing", private)
	}
	ticket := commandsPromptTicket(t, private)
	if ticket == groupTicket {
		t.Fatal("shared and private audience inherited ticket")
	}
	for _, tc := range []struct {
		actor, id string
		chat      bot.ChatType
	}{
		{"second-admin", ticket, bot.ChatDM}, {"owner", groupTicket, bot.ChatDM}, {"owner", "private-id", bot.ChatDM},
	} {
		if result := send(tc.actor, "/desktop answer "+tc.id+" 2", tc.chat); !strings.Contains(result, "未确认") || f.writes.Load() != 0 {
			t.Fatal("wrong actor/audience/raw ID dispatched", result)
		}
	}
	if result := send("owner", "/desktop answer "+ticket+" 2", bot.ChatDM); !strings.Contains(result, "已提交") || f.writes.Load() != 1 {
		t.Fatal("real gateway did not dispatch sole scoped decision", result)
	}
	request := notificationWait(t, f.requests)
	answer, err := controller.DecodeSessionPromptAnswer(request)
	if err != nil || answer.Questions[0].Selected[0] != "B" || request.SessionPromptScope != f.scope {
		t.Fatal(request, err)
	}
	if result := send("owner", "/desktop answer "+ticket+" 2", bot.ChatDM); !strings.Contains(result, "未确认") || f.writes.Load() != 1 {
		t.Fatal("gateway retried spent decision", result)
	}
	commands.Close()
	if result := send("owner", "/desktop status", bot.ChatDM); !strings.Contains(result, "未确认") {
		t.Fatal("closed command owner served stale catalogue", result)
	}
}

func TestPreviewDesktopCommandsFiveKindsDisplayAndExactDispatch(t *testing.T) {
	for _, tc := range []struct{ kind, action, answer string }{
		{"ask", "answer", "2"}, {"approval", "approve", ""}, {"plan", "plan", "start_execution"}, {"recovery", "recovery", "continue"}, {"mcp", "mcp", "accept {}"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f := remotePromptTestFixture(t, tc.kind)
			h := newPreviewDesktopCommands(f.catalogue)
			defer h.Close()
			command := bot.DesktopCommand{Route: f.route, ActorID: "owner", Action: "pending", TargetID: f.entry.handle}
			text, err := h.ExecuteDesktopCommand(context.Background(), command)
			if err != nil || !strings.Contains(text, tc.kind) {
				t.Fatal(text, err)
			}
			ticket := commandsPromptTicket(t, text)
			command.Action, command.TargetID, command.AnswerText = tc.action, ticket, tc.answer
			if _, err := h.ExecuteDesktopCommand(context.Background(), command); err != nil || f.writes.Load() != 1 {
				t.Fatal("display and decision did not share ticket", err)
			}
		})
	}
}

func TestPreviewDesktopPromptDetailIncludesConsentContextWithoutRoutingOrExternalURL(t *testing.T) {
	prompt := controller.SessionPendingPrompt{Approval: &eventwire.Approval{Tool: "bash", Subject: "subject\nforged line", Reason: "reason", Recovery: &eventwire.RecoveryApproval{NextAction: "next", CanGrantTask: true, TaskGrantScope: "scope"}, WriteAccess: &eventwire.WriteAccessApproval{DisplayDirectories: []string{"directory"}, BroadHomeAccess: true}}}
	var out strings.Builder
	writePreviewPromptDetail(&out, prompt)
	for _, expected := range []string{"bash", "subject forged line", "reason", "next", "scope", "directory", "broad_home_access"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatal("consent context omitted", expected)
		}
	}
	out.Reset()
	writePreviewPromptDetail(&out, controller.SessionPendingPrompt{MCPInteraction: &eventwire.MCPInteraction{ID: "raw-id", TurnID: "raw-turn", ElicitationID: "raw-elicitation", Server: "service", Mode: "url", Message: "message", URL: "https://private.example/token"}})
	if strings.Contains(out.String(), "raw-") || strings.Contains(out.String(), "private.example") || !strings.Contains(out.String(), "桌面核对") || !strings.Contains(out.String(), "message") {
		t.Fatal("MCP routing/URL leaked or context omitted", out.String())
	}
	prompt.Scope.Kind = "recovery"
	prompt.Approval.Recovery.CanGrantTask = false
	if strings.Contains(previewPromptInstructions(prompt, "ticket"), "continue_task") {
		t.Fatal("advertised unavailable task grant")
	}
	prompt.Approval.Recovery.CanGrantTask = true
	if !strings.Contains(previewPromptInstructions(prompt, "ticket"), "continue_task") {
		t.Fatal("omitted explicitly available task grant")
	}
}

func TestPreviewDesktopCommandsRejectReplacementAndCancelInFlightDecision(t *testing.T) {
	f := remotePromptTestFixture(t, "ask")
	h := newPreviewDesktopCommands(f.catalogue)
	defer h.Close()
	command := bot.DesktopCommand{Route: f.route, ActorID: "owner", Action: "pending", TargetID: f.entry.handle}
	text, err := h.ExecuteDesktopCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	ticket := commandsPromptTicket(t, text)
	f.generation.Store(2)
	if _, err := h.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("old handle recaptured same-path replacement")
	}
	command.Action, command.TargetID, command.AnswerText = "answer", ticket, "2"
	if _, err := h.ExecuteDesktopCommand(context.Background(), command); err == nil || f.writes.Load() != 0 {
		t.Fatal("old ticket dispatched to replacement")
	}

	// Separate original owner fixture proves Close interrupts admitted HTTP IO.
	g := remotePromptTestFixture(t, "ask")
	defer close(g.release)
	active := newPreviewDesktopCommands(g.catalogue)
	defer active.Close()
	command = bot.DesktopCommand{Route: g.route, ActorID: "owner", Action: "pending", TargetID: g.entry.handle}
	text, err = active.ExecuteDesktopCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	command.Action, command.TargetID, command.AnswerText = "answer", commandsPromptTicket(t, text), "2"
	g.block.Store(true)
	done := make(chan error, 1)
	go func() { _, err := active.ExecuteDesktopCommand(context.Background(), command); done <- err }()
	notificationWait(t, g.requests)
	active.Close()
	if err := notificationWait(t, done); err != errPreviewDesktopBinding || g.writes.Load() != 1 {
		t.Fatal("composition close did not cancel the sole dispatch", err)
	}
}

func TestPreviewDesktopCommandsOversizeNeverPublishesTruncatedConsent(t *testing.T) {
	path := "/remote/live.jsonl"
	epoch := "captured-" + path
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: path}}, "captured"))
			return
		}
		if r.URL.Path != "/desktop/session-pending" {
			t.Error("display dispatched an operation")
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(controller.SessionPendingView{ProtocolVersion: 1, SessionPendingScope: controller.SessionPendingScope{SessionPath: path, RuntimeEpoch: epoch}, Revision: 1, TurnID: "turn", Prompts: []controller.SessionPendingPrompt{{Scope: controller.SessionPromptScope{SessionPath: path, RuntimeEpoch: epoch, TurnID: "turn", PromptID: "private-id", Kind: "ask"}, Ask: &eventwire.Ask{ID: "private-id", TurnID: "turn", Questions: []eventwire.AskQuestion{{ID: "q", Prompt: strings.Repeat("private detail ", 2000)}}}}}})
	})
	attachController(t, b, "/owned")
	c := newPreviewDesktopCatalogue(nil, b.remoteSessions)
	defer c.Close()
	h := newPreviewDesktopCommands(c)
	defer h.Close()
	entries, err := c.Refresh(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	text, err := h.ExecuteDesktopCommand(context.Background(), bot.DesktopCommand{Route: desktopWatchTestRoute(), ActorID: "owner", Action: "pending", TargetID: entries[0].handle})
	if err != nil || !strings.Contains(text, "超过安全展示上限") || strings.Contains(text, "private") || strings.Contains(text, "rp-") || len(text) > previewDesktopDisplayLimit {
		t.Fatal("oversized consent was truncated/partially advertised", text, err)
	}
}
