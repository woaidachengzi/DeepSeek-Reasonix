package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/provider"
	"reasonix/internal/remote/controller"
)

func desktopViewFixture(t *testing.T) (*ownershipFixture, *controller.Client) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, "profile"))
	if err := os.MkdirAll(filepath.Dir(config.UserConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.UserConfigPath(), []byte("default_model = \"owned-unconfigured\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := newOwnershipFixture(t)
	f.server.titleProv = nil
	f.server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "owned-view-token"})
	s := httptest.NewServer(f.server.Handler())
	t.Cleanup(s.Close)
	response, err := http.Get(s.URL + "/desktop/session-view?session=" + url.QueryEscape(agent.CanonicalSessionPath(f.active)))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("snapshot bypassed token gate")
	}
	c, err := controller.Connect(context.Background(), context.Background(), s.URL, "owned-view-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return f, c
}

func TestDesktopSessionViewActualServeSavedAndOwnedHistory(t *testing.T) {
	f, c := desktopViewFixture(t)
	activePath := agent.CanonicalSessionPath(f.active)
	active, err := c.SessionView(context.Background(), activePath)
	if err != nil || !active.Current || active.Ownership != "serve" || active.RuntimeState == nil || active.RuntimeState.RuntimeEpoch == "" {
		t.Fatalf("foreground view %+v %v", active, err)
	}
	var ownedMessages []provider.Message
	for _, message := range f.server.ctl().History() {
		if message.Role != provider.RoleSystem {
			ownedMessages = append(ownedMessages, message)
		}
	}
	if len(ownedMessages) != len(active.History) {
		t.Fatal("owned history fixture unexpectedly contains filtered entries")
	}
	for i, row := range active.History {
		if row.ID != ownedMessages[i].ID {
			t.Fatal("owned snapshot replaced Controller identity")
		}
	}
	other := filepath.Join(f.dir, "中文 +&.jsonl")
	session := agent.NewSession("sys")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "saved question\nline2"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "saved answer", ReasoningContent: "actual reasoning", ToolCalls: []provider.ToolCall{{ID: "t1", Name: "read", Arguments: `{"path":"a"}`}}, ServerSearch: []provider.ServerSearchCall{{ID: "s1", Query: "search", Raw: json.RawMessage(`{"private":"encrypted"}`), Results: []provider.ServerSearchHit{{Title: "source", URL: "https://example.invalid"}}}}})
	session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "t1", Name: "read", Content: "tool result"})
	if err := session.Save(other); err != nil {
		t.Fatal(err)
	}
	view, err := c.SessionView(context.Background(), agent.CanonicalSessionPath(other))
	if err != nil || view.Ownership != "saved" || view.Current || view.RuntimeState != nil || view.ModelRef != "" || view.Label != "" {
		t.Fatalf("saved view %+v %v", view, err)
	}
	var got []string
	for _, m := range view.History {
		got = append(got, m.Content)
	}
	if !strings.Contains(strings.Join(got, "|"), "saved answer") || !strings.Contains(strings.Join(got, "|"), "tool result") {
		t.Fatal("selected saved history was replaced by foreground history")
	}
	data, _ := json.Marshal(view)
	if strings.Contains(string(data), "encrypted") || strings.Contains(string(data), `"raw"`) {
		t.Fatal("provider replay leaked")
	}
	if f.server.ctl().SessionPath() != f.active || f.server.sessionMirrored(other) {
		t.Fatal("read switched or adopted remote session")
	}
	again, err := c.SessionView(context.Background(), agent.CanonicalSessionPath(other))
	if err != nil || !reflect.DeepEqual(again.History, view.History) {
		t.Fatal("repeat snapshot changed backend entry identities")
	}
	for i, row := range view.History {
		if row.ID != session.Messages[i+1].ID {
			t.Fatal("display identity was not the persisted backend message ID")
		}
	}
	session.Add(provider.Message{Role: provider.RoleUser, Content: "new tail"})
	if err := session.Save(other); err != nil {
		t.Fatal(err)
	}
	appended, err := c.SessionView(context.Background(), agent.CanonicalSessionPath(other))
	if err != nil || len(appended.History) != len(view.History)+1 || !reflect.DeepEqual(appended.History[:len(view.History)], view.History) {
		t.Fatal("append renamed or changed existing display entries")
	}
	for _, raw := range []string{"", "/no-such-session.jsonl", filepath.Join(f.dir, "x.events"), agent.CanonicalSessionPath(other) + "&other=private"} {
		status, _ := f.get(t, "/desktop/session-view?session="+url.QueryEscape(raw))
		if status == 200 {
			t.Fatal("invalid selection fell back to foreground")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := session.Save(outside); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.dir, "escaped.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.get(t, "/desktop/session-view?session="+url.QueryEscape(link)); status == 200 {
		t.Fatal("symlink escaped session root")
	}
	if _, err := c.SessionView(context.Background(), outside); !errors.Is(err, controller.ErrSessionNotListed) {
		t.Fatal("unlisted path reached remote read")
	}
}

func TestDesktopSessionViewLegacyIDsAreReadOnlyAndDeterministic(t *testing.T) {
	f, c := desktopViewFixture(t)
	path := filepath.Join(f.dir, "legacy-ids.jsonl")
	contents := []byte("{\"role\":\"user\",\"content\":\"old question\"}\n{\"role\":\"assistant\",\"content\":\"old answer\"}\n")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	canonical := agent.CanonicalSessionPath(path)
	first, err := c.SessionView(context.Background(), canonical)
	if err != nil || len(first.History) != 2 || first.History[0].ID == "" || first.History[0].ID == first.History[1].ID {
		t.Fatalf("legacy projection %+v %v", first, err)
	}
	second, err := c.SessionView(context.Background(), canonical)
	if err != nil || !reflect.DeepEqual(first.History, second.History) {
		t.Fatal("legacy identities changed between reads")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatal("spectator read rewrote legacy file")
	}
}

func TestDesktopSessionViewRejectsDuplicateStoredIdentity(t *testing.T) {
	f, _ := desktopViewFixture(t)
	path := filepath.Join(f.dir, "duplicate-ids.jsonl")
	// Use a legacy transcript, not Session.Save: the DAG loader correctly
	// deduplicates duplicate event records before this display projection.
	contents := []byte("{\"id\":\"duplicate-entry\",\"role\":\"user\",\"content\":\"question\"}\n{\"id\":\"duplicate-entry\",\"role\":\"assistant\",\"content\":\"answer\"}\n")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	status, body := f.get(t, "/desktop/session-view?session="+url.QueryEscape(agent.CanonicalSessionPath(path)))
	if status != 422 || strings.Contains(body, "duplicate-entry") || strings.Contains(body, "question") {
		t.Fatalf("invalid identities published or diagnostic leaked: %d %s", status, body)
	}
}

func TestDesktopHistoryProjectionKeepsBackendIdentityThroughFiltering(t *testing.T) {
	marker := provider.Message{ID: "recovery-entry", Role: provider.RoleTool, LocalOnly: true, FinalReadinessRecovery: &provider.FinalReadinessRecovery{Pending: true, Missing: []string{"verification"}}}
	user := provider.Message{ID: "user-entry", Role: provider.RoleUser, Content: "question"}
	answer := provider.Message{ID: "answer-entry", Role: provider.RoleAssistant, Content: "answer"}
	private, err := desktopHistoryMessages([]provider.Message{{ID: "system-entry", Role: provider.RoleSystem, Content: "PRIVATE system prompt"}, {ID: "developer-entry", Role: provider.Role("developer"), Content: "PRIVATE developer prompt"}, user, answer})
	if err != nil || len(private) != 2 || private[0].ID != user.ID || private[1].ID != answer.ID {
		t.Fatal("private prompt became a remote conversation row")
	}
	rows, err := desktopHistoryMessages([]provider.Message{user, marker, answer})
	if err != nil || len(rows) != 3 || rows[1].ID != marker.ID || rows[1].Role != "final_readiness" {
		t.Fatalf("transformed identity %+v %v", rows, err)
	}
	marker.FinalReadinessRecovery.Pending = false
	filtered, err := desktopHistoryMessages([]provider.Message{user, marker, answer})
	if err != nil || len(filtered) != 2 || filtered[0].ID != rows[0].ID || filtered[1].ID != rows[2].ID {
		t.Fatal("filtering consumed recovery renamed visible entries")
	}
	reordered, err := desktopHistoryMessages([]provider.Message{answer, user})
	if err != nil || reordered[0].ID != answer.ID || reordered[1].ID != user.ID {
		t.Fatal("projection used positions instead of backend identities")
	}
	for _, invalid := range []provider.Message{{Role: provider.RoleUser}, {ID: "bad\nidentity", Role: provider.RoleUser}, {ID: strings.Repeat("x", 4097), Role: provider.RoleUser}, user} {
		if _, err := desktopHistoryMessages([]provider.Message{user, invalid}); err == nil {
			t.Fatal("invalid/duplicate display identity accepted")
		}
	}
	legacy, _ := json.Marshal(historyMessages([]provider.Message{user}))
	if strings.Contains(string(legacy), `"id"`) {
		t.Fatal("legacy Serve history wire changed")
	}
}

func TestDesktopSessionViewMirroredReadNeverAutoReclaims(t *testing.T) {
	f, c := desktopViewFixture(t)
	f.leases.Release()
	f.server.markMirrored(mirroredSession{path: f.active, mirrorID: "owned-mirror", phase: mirrorPhaseExternal, lastContact: time.Now().Add(-2 * mirrorStaleAfter)})
	extendSessionOnDisk(t, f.active, "writer's latest question")
	v, err := c.SessionView(context.Background(), agent.CanonicalSessionPath(f.active))
	if err != nil || v.Ownership != "external" || v.RuntimeState != nil || v.ModelRef != "" {
		t.Fatalf("external view %+v %v", v, err)
	}
	data, _ := json.Marshal(v)
	if !strings.Contains(string(data), "writer's latest question") || !f.server.sessionMirrored(f.active) || f.server.ctl().SessionPath() != f.active {
		t.Fatal("spectator read reclaimed or used frozen history")
	}
	if err := os.Remove(f.active); err != nil {
		t.Fatal(err)
	}
	status, body := f.get(t, "/desktop/session-view?session="+url.QueryEscape(agent.CanonicalSessionPath(f.active)))
	if status == 200 || strings.Contains(body, "writer's latest question") {
		t.Fatal("missing file fell back to memory")
	}
}

func TestDesktopSessionViewDetachedUsesItsRuntimeAndModel(t *testing.T) {
	f, c := desktopViewFixture(t)
	path := filepath.Join(f.dir, "detached.jsonl")
	saveServeTestSession(t, path)
	detached := control.New(control.Options{SessionDir: f.dir, SessionPath: path, ModelRef: "remote-owned/model", Label: "owned-label"})
	t.Cleanup(detached.Close)
	canonical := agent.CanonicalSessionPath(path)
	d := &detachedSession{path: canonical, ctrl: detached}
	f.server.detachedMu.Lock()
	f.server.detached[canonical] = d
	f.server.detachedMu.Unlock()
	v, err := c.SessionView(context.Background(), canonical)
	if err != nil || v.Current || v.Ownership != "serve" || v.ModelRef != "remote-owned/model" || v.Label != "owned-label" || v.RuntimeState == nil || v.RuntimeState.RuntimeEpoch != detached.RuntimeStateSnapshot().RuntimeEpoch {
		t.Fatalf("detached identity %+v %v", v, err)
	}
	f.server.detachedMu.Lock()
	d.retiring = true
	f.server.detachedMu.Unlock()
	status, _ := f.get(t, "/desktop/session-view?session="+url.QueryEscape(canonical))
	if status != 409 {
		t.Fatal("retiring owner fell back to saved/foreground view")
	}
}
