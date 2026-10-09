package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type ownedCommandRuntime struct {
	*fakeRuntime
	epoch        string
	commandState OwnedCommandState
	decisions    int
}

func (r *ownedCommandRuntime) CommandSnapshot() (string, OwnedCommandState) {
	return r.epoch, r.commandState
}
func (r *ownedCommandRuntime) SubmitOwnedContext(ctx context.Context, view OwnedCommandView, text string) error {
	if ctx.Err() != nil || view.State.Revision != r.commandState.Revision {
		return ErrOwnedRuntimeChanged
	}
	r.submits = append(r.submits, text)
	r.commandState.Revision++
	return nil
}
func (r *ownedCommandRuntime) ResolveOwnedContext(_ context.Context, _ OwnedCommandScope, _ OwnedPrompt, answer json.RawMessage) error {
	r.decisions++
	answer[0] = '!' // manager must give this consumer an independent buffer
	return nil
}

func commandManager(t *testing.T) (*RuntimeManager, *ownedCommandRuntime, OwnedCommandView) {
	t.Helper()
	r := &ownedCommandRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/owned.jsonl", state: "idle"}, epoch: "controller-epoch",
		commandState: OwnedCommandState{Revision: 1, Phase: "idle", Pending: []OwnedPrompt{{ID: "p", Kind: "ask", TurnID: "turn"}}}}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned", WorkspaceRoot: "/owned-workspace"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	view, ok := m.CommandSnapshot()
	if !ok {
		t.Fatal("owned command view absent")
	}
	return m, r, view
}

func TestOwnedCommandsCaptureAndFenceEveryIdentity(t *testing.T) {
	m, r, view := commandManager(t)
	mutations := []func(*OwnedCommandScope){
		func(s *OwnedCommandScope) { s.SessionID = "foreign" }, func(s *OwnedCommandScope) { s.OwnerEpoch++ },
		func(s *OwnedCommandScope) { s.SessionPath += ".other" }, func(s *OwnedCommandScope) { s.RuntimeEpoch += "-other" },
	}
	for _, mutate := range mutations {
		stale := view
		mutate(&stale.Scope)
		if err := m.SubmitOwned(context.Background(), stale, "text"); !errors.Is(err, ErrOwnedRuntimeChanged) {
			t.Fatal(err)
		}
		if err := m.ResolveOwnedPrompt(context.Background(), stale.Scope, view.State.Pending[0], json.RawMessage(`{"questions":[]}`)); !errors.Is(err, ErrOwnedRuntimeChanged) {
			t.Fatal(err)
		}
	}
	if len(r.submits) != 0 || r.decisions != 0 {
		t.Fatal("invalid identity dispatched")
	}
	view.State.Pending[0].ID = "consumer mutation"
	if r.commandState.Pending[0].ID != "p" {
		t.Fatal("snapshot aliases provider pending identities")
	}
	if err := m.SubmitOwned(context.Background(), view, "first"); err != nil {
		t.Fatal(err)
	}
	if err := m.SubmitOwned(context.Background(), view, "duplicate"); err == nil {
		t.Fatal("old revision submitted again")
	}
	if len(r.submits) != 1 {
		t.Fatal(r.submits)
	}
	raw := json.RawMessage(`{"questions":[]}`)
	if err := m.ResolveOwnedPrompt(context.Background(), view.Scope, OwnedPrompt{ID: "p", Kind: "ask", TurnID: "turn"}, raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"questions":[]}` {
		t.Fatal("provider mutated caller answer")
	}
}

func TestOwnedCommandsRevocationAfterManagerLockWait(t *testing.T) {
	m, r, view := commandManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	result := make(chan error, 1)
	go func() { result <- m.SubmitOwned(ctx, view, "must not run") }()
	cancel()
	m.mu.Unlock()
	if err := <-result; !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if len(r.submits) != 0 {
		t.Fatal("revoked command dispatched")
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.CommandSnapshot(); ok {
		t.Fatal("closed manager exposed commands")
	}
	if err := m.SubmitOwned(context.Background(), view, "closed"); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
}

func TestOwnedCommandsNeverFallBackToCompatibilityRuntime(t *testing.T) {
	r := &fakeRuntime{path: "/sessions/old.jsonl", state: "idle"}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "old"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	if _, ok := m.CommandSnapshot(); ok {
		t.Fatal("compatibility runtime gained command capability")
	}
	view := OwnedCommandView{Scope: OwnedCommandScope{"old", 1, r.path, "assumed"}, State: OwnedCommandState{Revision: 1}}
	if err := m.SubmitOwned(context.Background(), view, "must not fall back"); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if _, err := m.ReadOwnedPrompt(context.Background(), view.Scope, OwnedPrompt{ID: "assumed", TurnID: "turn", Kind: "ask"}); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal("compatibility runtime gained prompt reading", err)
	}
	if len(r.submits) != 0 {
		t.Fatal("compatibility Submit was called")
	}
}
