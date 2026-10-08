package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge"
)

func terminalLeaseTestProfile(t *testing.T) {
	t.Helper()
	profile := t.TempDir()
	for key, value := range map[string]string{"HOME": t.TempDir(), "REASONIX_HOME": profile, "REASONIX_STATE_HOME": profile,
		"REASONIX_CACHE_HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file"} {
		t.Setenv(key, value)
	}
	const config = `default_model = "fixture/deepseek-v4-flash"
[[providers]]
name = "fixture"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
models = ["ordinary", "deepseek-v4-flash"]
default = "deepseek-v4-flash"
no_proxy = true
`
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestControllerTerminalLeaseRejectsForeignIdentityAndRestoresSource(t *testing.T) {
	terminalLeaseTestProfile(t)
	events := desktopbridge.NewEventStream(32)
	factory := newControllerFactory(events)
	root, otherRoot := t.TempDir(), t.TempDir()
	open := func(id, workspace string) *controllerRuntime {
		t.Helper()
		runtime, err := factory.Open(context.Background(), desktopbridge.OpenRequest{SessionID: id, WorkspaceRoot: workspace})
		if err != nil {
			t.Fatal(err)
		}
		r := runtime.(*controllerRuntime)
		t.Cleanup(func() { _ = r.ReleaseForReplacement() })
		return r
	}
	source := open("lease-owned", root)
	if _, err := source.TerminalWorkspace(); err != nil {
		t.Fatal(err)
	}
	manager := source.terminals
	foreignRoot, foreignID, candidate := open("lease-owned", otherRoot), open("lease-foreign", root), open("lease-owned", root)
	lease, err := source.RetainTerminals()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close() })
	if err := source.WriteTerminal("not-created", []byte("input")); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("detached source accepted a delayed operation")
	}
	for _, rejected := range []*controllerRuntime{foreignRoot, foreignID, nil} {
		if err := lease.Attach(rejected); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
			t.Fatal("terminal lease crossed backend identity")
		}
	}
	candidate.terminalEvents = desktopbridge.NewEventStream(32)
	if err := lease.Attach(candidate); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("terminal lease crossed event namespace")
	}
	candidate.terminalEvents = events
	if _, err := candidate.TerminalWorkspace(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Attach(candidate); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("terminal lease overwrote another owner")
	}
	if err := lease.Attach(source); err != nil {
		t.Fatal("failed candidate did not permit old-owner restoration", err)
	}
	if source.terminals != manager {
		t.Fatal("restoration recreated instead of retained the manager")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Workspace(); err != nil {
		t.Fatal("closing an attached lease closed live terminals")
	}
	if err := lease.Attach(foreignRoot); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("consumed terminal lease transferred twice")
	}
}

type rejectedTerminalHandoffFactory struct {
	*controllerFactory
	reject bool
}

func (f *rejectedTerminalHandoffFactory) Rebuild(ctx context.Context, old desktopbridge.Runtime, req desktopbridge.OpenRequest) (desktopbridge.SettingsRuntime, error) {
	next, err := f.controllerFactory.Rebuild(ctx, old, req)
	if err == nil && f.reject {
		next.(*controllerRuntime).terminalEvents = desktopbridge.NewEventStream(16)
	}
	return next, err
}

func TestRejectedTerminalHandoffRollsBackTentativeEffortMetadata(t *testing.T) {
	terminalLeaseTestProfile(t)
	ctx := context.Background()
	factory := &rejectedTerminalHandoffFactory{controllerFactory: newControllerFactory(desktopbridge.NewEventStream(32)), reject: true}
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "effort-terminal-handoff", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	before, exists, err := agent.LoadBranchMeta(view.Path)
	if err != nil || !exists || before.ReasoningEffort == nil {
		t.Fatal("reasoning fixture metadata missing", err)
	}
	if _, err := manager.TerminalWorkspace(view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionEffort(ctx, view.ID, view.ModelRef, "high"); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("injected handoff failure was ignored", err)
	}
	after, exists, err := agent.LoadBranchMeta(view.Path)
	if err != nil || !exists || after.ReasoningEffort == nil || *after.ReasoningEffort != *before.ReasoningEffort || after.ReasoningModel != before.ReasoningModel {
		t.Fatal("rejected candidate left committed reasoning metadata")
	}
	snapshot, ok := manager.Snapshot()
	if !ok || snapshot.Effort != view.Effort {
		t.Fatal("rejected handoff replaced the controller")
	}
	if _, err := manager.TerminalWorkspace(view.ID); err != nil {
		t.Fatal("rejected handoff failed to restore terminal gate", err)
	}
	factory.reject = false
	if _, err := manager.SetSessionEffort(ctx, view.ID, view.ModelRef, "high"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	committed, exists, err := agent.LoadBranchMeta(view.Path)
	if err != nil || !exists || committed.ReasoningEffort == nil || *committed.ReasoningEffort != "high" {
		t.Fatal("published candidate rolled back its committed selection")
	}
}

func TestTerminalRebuildAdmissionErrorsAreSafeHTTPConflicts(t *testing.T) {
	bridge := newBridgeServer(testToken, "terminal-retention-errors")
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{desktopbridge.ErrTerminalBusy, 409, "terminal_busy"},
		{desktopbridge.ErrTerminalUnavailable, 503, "terminal_unavailable"},
	} {
		w := httptest.NewRecorder()
		bridge.writeRuntimeError(w, test.err, "unsafe input should not be echoed")
		if w.Code != test.status || !strings.Contains(w.Body.String(), test.code) || strings.Contains(w.Body.String(), "unsafe input") {
			t.Fatal("terminal admission error was misclassified or leaked caller content")
		}
	}
}
