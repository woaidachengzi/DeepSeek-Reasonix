package bot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/event"
)

type scopedDesktopFixture struct {
	*fakeDesktopBridge
	commands []DesktopCommand
	active   bool
	probed   DesktopWatchRoute
	execute  func(context.Context, DesktopCommand) (string, error)
}

func (f *scopedDesktopFixture) ExecuteDesktopCommand(ctx context.Context, command DesktopCommand) (string, error) {
	f.commands = append(f.commands, command)
	if f.execute != nil {
		return f.execute(ctx, command)
	}
	return "scoped receipt", nil
}
func (f *scopedDesktopFixture) DesktopTakeoverActive(route DesktopWatchRoute, _ string) bool {
	f.probed = route
	return f.active
}
func (*scopedDesktopFixture) Sessions() []DesktopSessionInfo         { panic("legacy sessions") }
func (*scopedDesktopFixture) SetWatch(DesktopWatchRoute, bool) error { panic("legacy watch") }
func (*scopedDesktopFixture) Watching(DesktopWatchRoute) bool        { panic("legacy watching") }
func (*scopedDesktopFixture) Approve(string, bool) (string, error)   { panic("legacy approval") }
func (*scopedDesktopFixture) AskQuestions(string) ([]event.AskQuestion, bool) {
	panic("legacy question lookup")
}
func (*scopedDesktopFixture) Answer(string, []event.AskAnswer) (string, error) {
	panic("legacy answer")
}
func (*scopedDesktopFixture) Takeover(DesktopWatchRoute, string) (string, error) {
	panic("legacy takeover")
}
func (*scopedDesktopFixture) Release(DesktopWatchRoute) (string, error) { panic("legacy release") }
func (*scopedDesktopFixture) TakeoverTab(DesktopWatchRoute) string      { panic("legacy binding lookup") }
func (*scopedDesktopFixture) DriveInput(DesktopWatchRoute, string) (string, error) {
	panic("legacy drive")
}

func scopedDesktopGateway() (*BotGateway, *scopedDesktopFixture) {
	f := &scopedDesktopFixture{fakeDesktopBridge: newFakeDesktopBridge()}
	gw := &BotGateway{
		cfg: GatewayConfig{Desktop: f, Allowlist: AllowlistConfig{
			AllowAll: true, Admins: map[Platform][]string{PlatformFeishu: {"admin-user", "operator"}},
		}},
		logger: discardLogger(), adapterHealth: map[string]*AdapterHealthSnapshot{},
	}
	return gw, f
}

func TestDesktopScopedCommandActualSlashDispatch(t *testing.T) {
	gw, fixture := scopedDesktopGateway()
	adapter := newFakeAdapter(PlatformFeishu, "scoped")
	cases := []struct{ text, action, target, answer string }{
		{"/desktop", "status", "", ""},
		{"/desktop sessions", "status", "", ""},
		{"/desktop pending s-handle", "pending", "s-handle", ""},
		{"/desktop watch on", "watch", "on", ""},
		{"/desktop watch off", "watch", "off", ""},
		{"/desktop watch status", "watch", "status", ""},
		{"/desktop watch state", "watch", "status", ""},
		{"/desktop approve prompt", "approve", "prompt", ""},
		{"/desktop deny prompt", "deny", "prompt", ""},
		{"/desktop answer prompt 1=2; 2=free text", "answer", "prompt", "1=2; 2=free text"},
		{"/desktop plan ticket revise_plan fix it", "plan", "ticket", "revise_plan fix it"},
		{"/desktop recovery ticket continue_task", "recovery", "ticket", "continue_task"},
		{`/desktop mcp ticket accept {"label":"two  spaces"}`, "mcp", "ticket", `accept {"label":"two  spaces"}`},
		{"/desktop takeover tab", "takeover", "tab", ""},
		{"/desktop release", "release", "", ""},
	}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "inbound lifetime")
	fixture.execute = func(actual context.Context, _ DesktopCommand) (string, error) {
		if actual != ctx {
			t.Fatal("dispatch replaced inbound context")
		}
		return "scoped receipt", nil
	}
	for _, tc := range cases {
		msg := desktopTestMessage(tc.text)
		msg.UserID, msg.OperatorID = "routing-only-not-admin", "operator"
		gw.handleMessage(ctx, AdapterBinding{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: adapter}, msg)
		if len(fixture.commands) == 0 {
			t.Fatal("missing scoped command")
		}
		got := fixture.commands[len(fixture.commands)-1]
		if got.Action != tc.action || got.TargetID != tc.target || got.AnswerText != tc.answer || got.ActorID != "operator" || got.Route != desktopRouteFromMessage(msg) {
			t.Fatalf("%q command = %#v", tc.text, got)
		}
	}
	if len(fixture.commands) != len(cases) || len(adapter.sentMessages()) != len(cases) {
		t.Fatal("dispatch count differs")
	}
	for _, sent := range adapter.sentMessages() {
		if sent.Text != "scoped receipt" {
			t.Fatalf("unexpected receipt %q", sent.Text)
		}
	}
}

func TestDesktopScopedRejectsMalformedRevokedAndCanceled(t *testing.T) {
	gw, fixture := scopedDesktopGateway()
	for _, text := range []string{
		"/desktop-other approve prompt", "/desktop approve", "/desktop approve prompt extra",
		"/desktop watch maybe", "/desktop watch on extra", "/desktop status extra",
		"/desktop release extra", "/desktop answer prompt", "/desktop takeover tab extra",
		"/desktop pending", "/desktop pending one extra",
		"/desktop " + strings.Repeat("x", desktopCommandBytes),
	} {
		if got := gw.handleDesktopCommandContext(context.Background(), desktopTestMessage(text)); got != desktopScopedCommandUsage {
			t.Fatalf("%q accepted: %q", text[:min(len(text), 70)], got)
		}
	}
	for _, change := range []func(*InboundMessage){
		func(m *InboundMessage) { m.ConnectionID = "" },
		func(m *InboundMessage) { m.ChatID = "" },
		func(m *InboundMessage) { m.ChatType = "unknown" },
		func(m *InboundMessage) { m.Platform = "unknown" },
	} {
		msg := desktopTestMessage("/desktop status")
		change(&msg)
		gw.handleDesktopCommandContext(context.Background(), msg)
	}
	msg := desktopTestMessage("/desktop approve prompt")
	msg.OperatorID = "revoked-operator" // must not inherit routing UserID's admin.
	if got := gw.handleDesktopCommandContext(context.Background(), msg); !strings.Contains(got, "没有") {
		t.Fatalf("revoked reply = %q", got)
	}
	msg = desktopTestMessage("/desktop approve prompt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := gw.handleDesktopCommandContext(ctx, msg); got != desktopOperationUnknown {
		t.Fatalf("cancel reply = %q", got)
	}
	if got := gw.handleDesktopCommandContext(nil, msg); got != desktopOperationUnknown {
		t.Fatalf("nil context reply = %q", got)
	}
	if len(fixture.commands) != 0 {
		t.Fatal("rejected command dispatched")
	}

	fixture.execute = func(context.Context, DesktopCommand) (string, error) {
		return "private success", errors.New("private provider key/workspace")
	}
	if got := gw.handleDesktopCommandContext(context.Background(), msg); got != desktopOperationUnknown {
		t.Fatalf("private failure leaked: %q", got)
	}
	ctx, cancel = context.WithCancel(context.Background())
	fixture.execute = func(context.Context, DesktopCommand) (string, error) {
		cancel()
		return "accepted before cancellation", nil
	}
	if got := gw.handleDesktopCommandContext(ctx, msg); got != desktopOperationUnknown {
		t.Fatalf("late cancellation = %q", got)
	}
	if len(fixture.commands) != 2 {
		t.Fatal("unknown result retried or fell back")
	}
}

func TestDesktopScopedInboundRequiresConnectionAdmissionAndAdmin(t *testing.T) {
	gw, fixture := scopedDesktopGateway()
	gw.cfg.ConnectionAccess = map[string]AccessConfig{
		"feishu-main": {Enabled: true, Users: []string{"member"}, Approvers: []string{"member"}, Admins: []string{"owner"}},
		"other":       {Enabled: true, Users: []string{"owner"}, Admins: []string{"other-owner"}},
	}
	adapter := newFakeAdapter(PlatformFeishu, "scoped")
	for _, tc := range []struct {
		connection, user, operator string
		allowed                    bool
	}{
		{"feishu-main", "owner", "", true},
		{"feishu-main", "member", "", false},
		{"feishu-main", "unknown", "", false},
		{"feishu-main", "owner", "member", false},
		{"feishu-main", "routing-only", "owner", true},
		{"other", "owner", "", false},
	} {
		before := len(fixture.commands)
		msg := desktopTestMessage("/desktop approve prompt")
		msg.ConnectionID, msg.UserID, msg.OperatorID = "", tc.user, tc.operator
		gw.handleMessage(context.Background(), AdapterBinding{ID: tc.connection, Domain: msg.Domain, Platform: msg.Platform, Adapter: adapter}, msg)
		if (len(fixture.commands) == before+1) != tc.allowed {
			t.Fatalf("admission %#v commands=%#v", tc, fixture.commands)
		}
		if tc.allowed && fixture.commands[len(fixture.commands)-1].Route.ConnectionID != tc.connection {
			t.Fatal("lost concrete adapter connection")
		}
	}
}

func TestDesktopScopedTakeoverUsesExactRouteAndNeverLegacy(t *testing.T) {
	gw, fixture := scopedDesktopGateway()
	adapter := newFakeAdapter(PlatformFeishu, "scoped")
	msg := desktopTestMessage("  exact input\nincluding spaces  ")
	if gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("inactive takeover consumed input")
	}
	fixture.active = true
	msg.ChatType = ChatGroup
	if !gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("active takeover not consumed")
	}
	got := fixture.commands[0]
	if got.Action != "drive" || got.AnswerText != msg.Text || got.Route != desktopRouteFromMessage(msg) || fixture.probed != got.Route {
		t.Fatalf("drive = %#v", got)
	}
	msg.OperatorID = "revoked-operator"
	if !gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("revoked takeover not consumed")
	}
	if len(fixture.commands) != 2 || fixture.commands[1].Action != "release" || fixture.commands[1].ActorID != msg.OperatorID {
		t.Fatalf("revocation = %#v", fixture.commands)
	}
	fixture.execute = func(context.Context, DesktopCommand) (string, error) { return "", errors.New("private release error") }
	if !gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("failed revocation must still consume input")
	}
	sent := adapter.sentMessages()
	if got := sent[len(sent)-1].Text; !strings.Contains(got, "解除接管未确认") || strings.Contains(got, "private") || strings.Contains(got, "已解除") {
		t.Fatalf("false release receipt = %q", got)
	}
	msg = desktopTestMessage("/desktop release")
	if gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("slash command diverted")
	}
	msg = desktopTestMessage(strings.Repeat("x", desktopCommandBytes+1))
	if !gw.divertToDesktopTakeover(context.Background(), adapter, msg) || len(fixture.commands) != 3 {
		t.Fatal("oversize input dispatched")
	}
	msg.ChatType = ""
	if gw.divertToDesktopTakeover(context.Background(), adapter, msg) {
		t.Fatal("unknown audience inherited takeover")
	}
}

func TestDesktopWatchStatusAliasMatchesHelp(t *testing.T) {
	bridge := newFakeDesktopBridge()
	gw := &BotGateway{cfg: GatewayConfig{Desktop: bridge}}
	msg := desktopTestMessage("/desktop watch status")
	if got := gw.handleDesktopCommand(msg); !strings.Contains(got, "未订阅") {
		t.Fatalf("off status = %q", got)
	}
	bridge.watching[desktopRouteFromMessage(msg).Key()] = true
	if got := gw.handleDesktopCommand(msg); !strings.Contains(got, "正在订阅") {
		t.Fatalf("on status = %q", got)
	}
}

func TestDesktopCapturedAskParserRejectsDiscardedInput(t *testing.T) {
	questions := []event.AskQuestion{
		{ID: "first", Options: []event.AskOption{{Label: "A"}, {Label: "B"}}, Multi: true},
		{ID: "second", Options: []event.AskOption{{Label: "C"}}},
	}
	answers, err := ParseDesktopAskAnswers(questions, "1=1,2;second=free text")
	if err != nil || len(answers) != 2 || strings.Join(answers[0].Selected, ",") != "A,B" || answers[1].Selected[0] != "free text" {
		t.Fatalf("answers=%#v err=%v", answers, err)
	}
	for _, raw := range []string{"", "1=1", "1=1;unknown=2", "1=1;first=2;2=1", "1=1,1;2=1", "1=;2=1", "1=1;2=1;bad", "1"} {
		if _, err := ParseDesktopAskAnswers(questions, raw); err == nil {
			t.Fatalf("invalid assignment accepted: %q", raw)
		}
	}
	ambiguous := []event.AskQuestion{{ID: "2"}, {ID: "named"}}
	if _, err := ParseDesktopAskAnswers(ambiguous, "2=text;named=text"); err == nil {
		t.Fatal("numeric alias collision accepted")
	}
}
