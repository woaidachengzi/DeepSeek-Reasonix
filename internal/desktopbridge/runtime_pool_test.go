package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type parallelRuntime struct {
	*fakeRuntime
	phase atomic.Int32
}

func (r *parallelRuntime) State() string {
	switch r.phase.Load() {
	case 1:
		return "running"
	case 2:
		return "paused"
	case 3:
		return "deleted"
	}
	return "idle"
}
func (r *parallelRuntime) Submit(string) { r.phase.Store(1) }
func (r *parallelRuntime) Cancel()       { r.cancelCalls.Add(1); r.phase.Store(0) }
func (r *parallelRuntime) Delete() error { r.phase.Store(3); return nil }

func TestRuntimePoolParallelTurnsAndIndependentCancel(t *testing.T) {
	ctx := context.Background()
	a := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", history: []HistoryMessage{{Role: "user", Content: "A"}}}}
	b := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/b.jsonl", history: []HistoryMessage{{Role: "user", Content: "B"}}}}
	var builds atomic.Int32
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		builds.Add(1)
		if req.SessionID == "a" {
			return a, nil
		}
		return b, nil
	}))
	defer pool.Shutdown()
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Switch(ctx, OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Submit(ctx, id, "run"); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if _, err := pool.History(id); err != nil {
					t.Error(err)
				}
				if _, ok := pool.SessionSnapshot(id); !ok {
					t.Error("missing owner")
				}
			}
		}()
	}
	wg.Wait()
	if got := pool.Snapshots(); len(got) != 2 || got[0].State != "running" || got[1].State != "running" {
		t.Fatalf("parallel state: %+v", got)
	}
	if view, err := pool.Switch(ctx, OpenRequest{SessionID: "a"}); err != nil || view.State != "running" {
		t.Fatalf("reattach: %+v %v", view, err)
	}
	if builds.Load() != 2 || a.shutdownCalls.Load() != 0 || b.shutdownCalls.Load() != 0 {
		t.Fatal("navigation replaced a runtime")
	}
	if _, err := pool.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	if a.State() != "idle" || b.State() != "running" || b.cancelCalls.Load() != 0 {
		t.Fatal("cancel reached the wrong session")
	}
	if _, err := pool.Cancel("absent"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
	if _, err := pool.Switch(ctx, OpenRequest{SessionID: "a", WorkspaceRoot: "/wrong"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatal("workspace ownership changed", err)
	}
}

func TestRuntimePoolFailedNavigationPreservesRunningOwner(t *testing.T) {
	a := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl"}}
	failure := errors.New("target cannot open")
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		if req.SessionID == "a" {
			return a, nil
		}
		return nil, failure
	}))
	defer pool.Shutdown()
	_, _ = pool.Open(context.Background(), OpenRequest{SessionID: "a"})
	_, _ = pool.Submit(context.Background(), "a", "run")
	if _, err := pool.Switch(context.Background(), OpenRequest{SessionID: "b"}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if view, ok := pool.Snapshot(); !ok || view.ID != "a" || view.State != "running" {
		t.Fatal("failed navigation lost selected owner")
	}
	if len(pool.Snapshots()) != 1 || a.shutdownCalls.Load() != 0 {
		t.Fatal("failure damaged live slots")
	}
}

func TestRuntimePoolOwnedPromptsRemainScopedInBackground(t *testing.T) {
	makeRuntime := func(id string) *ownedCommandRuntime {
		return &ownedCommandRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/" + id + ".jsonl", state: "paused"}, epoch: "epoch-" + id, commandState: OwnedCommandState{Revision: 1, Phase: "paused", Pending: []OwnedPrompt{{ID: "same-prompt", Kind: "ask", TurnID: "same-turn"}}}}
	}
	a, b := makeRuntime("a"), makeRuntime("b")
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		if req.SessionID == "a" {
			return a, nil
		}
		return b, nil
	}))
	defer pool.Shutdown()
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	views := pool.CommandSnapshots()
	if len(views) != 2 {
		t.Fatal("background prompt owner missing")
	}
	captured := views[0]
	forged := captured.Scope
	forged.SessionID = "b"
	answer := json.RawMessage(`{"questions":[]}`)
	if err := pool.ResolveOwnedPrompt(context.Background(), forged, captured.State.Pending[0], answer); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal("foreign scope admitted", err)
	}
	forged.SessionID = "absent"
	if err := pool.ResolveOwnedPrompt(context.Background(), forged, captured.State.Pending[0], answer); !errors.Is(err, ErrOwnedRuntimeChanged) {
		t.Fatal("missing owner was not fenced", err)
	}
	if err := pool.ResolveOwnedPrompt(context.Background(), captured.Scope, captured.State.Pending[0], answer); err != nil {
		t.Fatal(err)
	}
	if a.decisions != 1 || b.decisions != 0 || string(answer) != `{"questions":[]}` {
		t.Fatal("prompt answer crossed owners")
	}
	if selected, ok := pool.CommandSnapshot(); !ok || selected.Session.ID != "b" {
		t.Fatal("background approval changed navigation")
	}
}

func TestRuntimePoolLifecycleTouchesOnlyTargetAndClosesEveryOwner(t *testing.T) {
	a := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", shutdownErr: errors.New("close failure")}}
	b := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/b.jsonl"}}
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		if req.SessionID == "a" {
			return a, nil
		}
		return b, nil
	}))
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = pool.Submit(context.Background(), "a", "run")
	var commits int
	if err := pool.ChangeArchive("saved", true, func(active bool) error {
		if active {
			t.Error("saved file became a runtime")
		}
		commits++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.ChangeArchive("a", true, func(bool) error { commits++; return nil }); !errors.Is(err, ErrSessionConflict) {
		t.Fatal(err)
	}
	if commits != 1 {
		t.Fatal("busy archive committed")
	}
	if err := pool.DeleteSession("b"); err != nil {
		t.Fatal(err)
	}
	if a.State() != "running" || a.shutdownCalls.Load() != 0 || b.State() != "deleted" {
		t.Fatal("delete affected another owner")
	}
	if err := pool.Shutdown(); !errors.Is(err, a.shutdownErr) {
		t.Fatal(err)
	}
	if a.shutdownCalls.Load() != 1 || len(pool.Snapshots()) != 0 {
		t.Fatal("shutdown did not close remaining owners")
	}
	if err := pool.Shutdown(); err != nil {
		t.Fatal("shutdown is not idempotent", err)
	}
}

func TestRuntimePoolShutdownFencesAnOpeningOwner(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	late := &fakeRuntime{path: "/sessions/late.jsonl", state: "idle"}
	pool := NewRuntimePool(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { close(started); <-release; return late, nil }))
	done := make(chan error, 1)
	go func() { _, err := pool.Open(context.Background(), OpenRequest{SessionID: "late"}); done <- err }()
	<-started
	if err := pool.Shutdown(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if late.shutdownCalls.Load() != 1 {
		t.Fatal("late runtime survived shutdown")
	}
	if _, err := pool.Open(context.Background(), OpenRequest{SessionID: "other"}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestRuntimePoolCancelledOpenDoesNotPublishOrLeak(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	late := &fakeRuntime{path: "/sessions/late.jsonl", state: "idle"}
	pool := NewRuntimePool(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		close(started)
		<-release
		return late, nil
	}))
	defer pool.Shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := pool.Open(ctx, OpenRequest{SessionID: "late"}); done <- err }()
	<-started
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if late.shutdownCalls.Load() != 1 || len(pool.Snapshots()) != 0 {
		t.Fatal("cancelled navigation published a runtime")
	}
}

func TestRuntimePoolShutdownClosesOtherSlotsAfterFailure(t *testing.T) {
	failure := errors.New("A close failed")
	a := &fakeRuntime{path: "/sessions/a.jsonl", state: "running", shutdownErr: failure}
	b := &fakeRuntime{path: "/sessions/b.jsonl", state: "paused"}
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		if req.SessionID == "a" {
			return a, nil
		}
		return b, nil
	}))
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.Shutdown(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if a.shutdownCalls.Load() != 1 || b.shutdownCalls.Load() != 1 {
		t.Fatal("one failure skipped another owner's shutdown")
	}
}

type retainedParallelRuntime struct {
	*parallelRuntime
	retained bool
}

func (r *retainedParallelRuntime) KeepRuntimeResident() bool { return r.retained }

func TestRuntimePoolEvictsOnlyUnselectedIdleOwners(t *testing.T) {
	runtimes := make(map[string]*parallelRuntime)
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
		runtime := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/" + req.SessionID + ".jsonl"}}
		runtimes[req.SessionID] = runtime
		return runtime, nil
	}))
	pool.capacity = 3
	defer pool.Shutdown()
	for _, id := range []string{"a", "b", "c"} {
		if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = pool.Submit(context.Background(), "a", "run")
	if _, err := pool.Open(context.Background(), OpenRequest{SessionID: "d"}); err != nil {
		t.Fatal(err)
	}
	if runtimes["b"].shutdownCalls.Load() != 1 || runtimes["a"].shutdownCalls.Load() != 0 || runtimes["c"].shutdownCalls.Load() != 0 {
		t.Fatal("eviction reached live or selected owner")
	}
	if len(pool.Snapshots()) != 3 {
		t.Fatal("resident bound was exceeded")
	}
	if _, ok := pool.SessionSnapshot("b"); ok {
		t.Fatal("retired owner is still published")
	}
	if _, err := pool.Switch(context.Background(), OpenRequest{SessionID: "b"}); err != nil {
		t.Fatal("evicted history cannot reopen", err)
	}
	if runtimes["a"].State() != "running" {
		t.Fatal("history reopening interrupted background turn")
	}
}

func TestRuntimePoolCapacityPreservesActiveAndRetainedResources(t *testing.T) {
	for _, state := range []string{"running", "paused", "retained"} {
		t.Run(state, func(t *testing.T) {
			a := &retainedParallelRuntime{parallelRuntime: &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl"}}, retained: state == "retained"}
			if state == "running" {
				a.phase.Store(1)
			}
			if state == "paused" {
				a.phase.Store(2)
			}
			b := &parallelRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/b.jsonl"}}
			var builds atomic.Int32
			pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, req OpenRequest) (Runtime, error) {
				builds.Add(1)
				if req.SessionID == "a" {
					return a, nil
				}
				return b, nil
			}))
			pool.capacity = 2
			defer pool.Shutdown()
			for _, id := range []string{"a", "b"} {
				if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Switch(context.Background(), OpenRequest{SessionID: "c"}); !errors.Is(err, ErrRuntimeCapacity) {
				t.Fatal(err)
			}
			if builds.Load() != 2 || a.shutdownCalls.Load() != 0 {
				t.Fatal("capacity cancelled a protected runtime")
			}
			if view, ok := pool.Snapshot(); !ok || view.ID != "b" {
				t.Fatal("capacity failure changed selection")
			}
		})
	}
}
