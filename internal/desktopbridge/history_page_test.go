package desktopbridge

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type historyPageRuntime struct {
	*fakeRuntime
	page         []HistoryMessage
	total        int
	pageCalls    atomic.Int32
	historyCalls atomic.Int32
	limit        atomic.Int64
}

func (r *historyPageRuntime) HistoryPage(limit int) ([]HistoryMessage, int) {
	r.pageCalls.Add(1)
	r.limit.Store(int64(limit))
	return r.page, r.total
}

func (r *historyPageRuntime) History() []HistoryMessage {
	r.historyCalls.Add(1)
	return r.fakeRuntime.History()
}

func TestRuntimeManagerUsesOptionalHistoryPage(t *testing.T) {
	for _, count := range []int{0, 2, maxHistoryMessages} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			page := make([]HistoryMessage, count)
			for i := range page {
				page[i] = HistoryMessage{Role: "user", Content: fmt.Sprint(i)}
			}
			total := count
			if count == maxHistoryMessages {
				total += 1000
			}
			runtime := &historyPageRuntime{
				fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "running"},
				page:        page,
				total:       total,
			}
			manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
			t.Cleanup(func() { _ = manager.Shutdown() })
			if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
				t.Fatal(err)
			}
			history, err := manager.History("a")
			if err != nil {
				t.Fatal(err)
			}
			if history.Session.ID != "a" || history.Session.State != "running" || history.StartIndex != total-count || history.TotalMessages != total || len(history.Messages) != count || history.Messages == nil {
				t.Fatalf("bounded history = %+v", history)
			}
			if runtime.pageCalls.Load() != 1 || runtime.historyCalls.Load() != 0 || runtime.limit.Load() != maxHistoryMessages {
				t.Fatalf("provider calls = page %d, history %d, limit %d", runtime.pageCalls.Load(), runtime.historyCalls.Load(), runtime.limit.Load())
			}
		})
	}
}

func TestRuntimeManagerHistoryPageIsolatesProviderCache(t *testing.T) {
	hit, miss := int64(7), int64(11)
	runtime := &historyPageRuntime{
		fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"},
		page: []HistoryMessage{{Role: "assistant", Content: "cached", TurnUsage: &HistoryTurnUsage{
			InputTokens: 23, CacheHitTokens: &hit, CacheMissTokens: &miss,
		}}},
		total: 1,
	}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	history, err := manager.History("a")
	if err != nil {
		t.Fatal(err)
	}
	history.Messages[0].Content = "modified"
	history.Messages[0].TurnUsage.InputTokens = 0
	*history.Messages[0].TurnUsage.CacheHitTokens = 0
	*history.Messages[0].TurnUsage.CacheMissTokens = 0
	second, err := manager.History("a")
	if err != nil {
		t.Fatal(err)
	}
	message := second.Messages[0]
	if message.Content != "cached" || message.TurnUsage.InputTokens != 23 || *message.TurnUsage.CacheHitTokens != 7 || *message.TurnUsage.CacheMissTokens != 11 {
		t.Fatalf("provider cache was modified through returned page: %+v", message)
	}
}

func TestRuntimeManagerInvalidHistoryPageFallsBack(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		total int
	}{
		{name: "negative total", total: -1},
		{name: "total smaller than page", count: 2, total: 1},
		{name: "page larger than limit", count: maxHistoryMessages + 1, total: maxHistoryMessages + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			full := make([]HistoryMessage, maxHistoryMessages+3)
			for i := range full {
				full[i] = HistoryMessage{Role: "user", Content: fmt.Sprint(i)}
			}
			runtime := &historyPageRuntime{
				fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle", history: full},
				page:        make([]HistoryMessage, test.count),
				total:       test.total,
			}
			manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
			t.Cleanup(func() { _ = manager.Shutdown() })
			if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
				t.Fatal(err)
			}
			history, err := manager.History("a")
			if err != nil {
				t.Fatal(err)
			}
			if runtime.pageCalls.Load() != 1 || runtime.historyCalls.Load() != 1 || history.StartIndex != 3 || history.TotalMessages != len(full) || len(history.Messages) != maxHistoryMessages || history.Messages[0].Content != "3" {
				t.Fatalf("fallback history = %+v; provider calls = page %d, history %d", history, runtime.pageCalls.Load(), runtime.historyCalls.Load())
			}
		})
	}
}

func TestRuntimeManagerHistoryPageRequiresOwnedRuntime(t *testing.T) {
	runtime := &historyPageRuntime{fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}}
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return runtime, nil }))
	if _, err := manager.History("a"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("history before open error = %v", err)
	}
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.History("b"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("wrong owner history error = %v", err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.History("a"); !errors.Is(err, ErrClosed) {
		t.Fatalf("history after shutdown error = %v", err)
	}
	if runtime.pageCalls.Load() != 0 || runtime.historyCalls.Load() != 0 {
		t.Fatal("history provider was called for an absent or closed owner")
	}
}

func TestRuntimePoolHistoryPagesKeepIndependentOwners(t *testing.T) {
	runtimes := make(map[string]*historyPageRuntime)
	for _, id := range []string{"a", "b"} {
		runtimes[id] = &historyPageRuntime{
			fakeRuntime: &fakeRuntime{path: "/sessions/" + id + ".jsonl", state: "idle"},
			page:        []HistoryMessage{{Role: "user", Content: id}},
			total:       501,
		}
	}
	pool := NewRuntimePool(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) { return runtimes[request.SessionID], nil }))
	t.Cleanup(func() { _ = pool.Shutdown() })
	for _, id := range []string{"a", "b"} {
		if _, err := pool.Open(context.Background(), OpenRequest{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				history, err := pool.History(id)
				if err != nil {
					t.Error(err)
					return
				}
				if history.Session.ID != id || history.StartIndex != 500 || history.TotalMessages != 501 || len(history.Messages) != 1 || history.Messages[0].Content != id {
					t.Errorf("history for %s = %+v", id, history)
					return
				}
				history.Messages[0].Content = "local caller change"
			}
		}()
	}
	group.Wait()
	for id, runtime := range runtimes {
		if runtime.pageCalls.Load() != 20 || runtime.historyCalls.Load() != 0 || runtime.page[0].Content != id {
			t.Errorf("owner %s page calls = %d, history calls = %d, content = %q", id, runtime.pageCalls.Load(), runtime.historyCalls.Load(), runtime.page[0].Content)
		}
	}
}

type settingsHistoryPageRuntime struct {
	*historyPageRuntime
	releases int
}

func (r *settingsHistoryPageRuntime) ModelSettingsState() (string, string, error) {
	return "old", "new", nil
}
func (r *settingsHistoryPageRuntime) ReleaseForReplacement() error {
	r.releases++
	return nil
}

type settingsHistoryPageFactory struct {
	RuntimeFactoryFunc
	next SettingsRuntime
	err  error
}

func (f *settingsHistoryPageFactory) Rebuild(context.Context, Runtime, OpenRequest) (SettingsRuntime, error) {
	return f.next, f.err
}

func TestRuntimeManagerHistoryPageFollowsSettingsReplacement(t *testing.T) {
	newRuntime := func(content string, total int) *settingsHistoryPageRuntime {
		return &settingsHistoryPageRuntime{historyPageRuntime: &historyPageRuntime{
			fakeRuntime: &fakeRuntime{path: "/sessions/a.jsonl", state: "idle"},
			page:        []HistoryMessage{{Role: "user", Content: content}},
			total:       total,
		}}
	}
	previous, next := newRuntime("previous generation", 301), newRuntime("next generation", 601)
	factory := &settingsHistoryPageFactory{
		RuntimeFactoryFunc: func(context.Context, OpenRequest) (Runtime, error) { return previous, nil },
		next:               next,
		err:                errors.New("build failed"),
	}
	manager := NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	assertHistory := func(content string, total int) {
		t.Helper()
		history, err := manager.History("a")
		if err != nil {
			t.Fatal(err)
		}
		if history.Session.ID != "a" || history.TotalMessages != total || history.StartIndex != total-1 || len(history.Messages) != 1 || history.Messages[0].Content != content {
			t.Fatalf("history generation = %+v", history)
		}
	}
	assertHistory("previous generation", 301)
	if _, err := manager.RebuildSettings(context.Background(), "a"); !errors.Is(err, ErrSessionSettingsApply) {
		t.Fatalf("failed settings rebuild error = %v", err)
	}
	assertHistory("previous generation", 301)
	factory.err = nil
	if _, err := manager.RebuildSettings(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	assertHistory("next generation", 601)
	if previous.pageCalls.Load() != 2 || previous.releases != 1 || next.pageCalls.Load() != 1 {
		t.Fatalf("generation calls: previous=%d released=%d next=%d", previous.pageCalls.Load(), previous.releases, next.pageCalls.Load())
	}
}
