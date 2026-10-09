package control

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
	"reasonix/internal/turnevent"
)

func TestTurnProjectionReplayPreservesLedgerOwnershipAndFailure(t *testing.T) {
	var absent *Controller
	empty, err := absent.TurnProjectionReplay()
	if err != nil || empty.Events == nil || len(empty.Events) != 0 {
		t.Fatal("absent controller must have explicit empty projection")
	}
	l, err := turnevent.Open(filepath.Join(t.TempDir(), "owned.jsonl"), "owned")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	})
	c := &Controller{}
	c.turnEvents.ledger = l
	id, err := l.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil || !ok {
		t.Fatalf("append: %v %v", ok, err)
	}
	view, err := c.TurnProjectionReplay()
	if err != nil || view.ActiveTurnID != id || len(view.Events) != 1 || view.Events[0].TurnID != id || view.ReplayAfterSequence != 0 || view.LatestSequence != 1 {
		t.Fatalf("view: %+v %v", view, err)
	}
	if l.ActiveTurnID() != id {
		t.Fatal("spectator read changed turn ownership")
	}
	page, err := c.TurnProjectionReplayPage(view.Boundary, view.ReplayAfterSequence)
	if err != nil || len(page.Events) != 1 || page.Events[0].TurnID != id || page.LatestSequence != view.LatestSequence {
		t.Fatalf("page: %+v %v", page, err)
	}
	if _, err := absent.TurnProjectionReplayPage(view.Boundary, 0); !errors.Is(err, turnevent.ErrProjectionReplayChanged) {
		t.Fatalf("nil page: %v", err)
	}
	retained, err := l.EventsAfter(0)
	if err != nil || len(retained) != 1 {
		t.Fatal("spectator read discarded retained events")
	}
	failed := errors.New("unavailable projection storage")
	c.turnEvents.err = failed
	if _, err := c.TurnProjectionReplay(); !errors.Is(err, failed) {
		t.Fatal("failed ledger must not become an empty ready projection")
	}
	if _, err := c.TurnProjectionReplayPage(view.Boundary, 0); !errors.Is(err, failed) {
		t.Fatalf("failed page: %v", err)
	}
}

func TestTurnProjectionViewSeparatesPrefixAndUserSuffix(t *testing.T) {
	session := agent.NewSession("SYS")
	session.Add(provider.Message{ID: "old-user", Role: provider.RoleUser, Content: "old question"})
	session.Add(provider.Message{ID: "old-answer", Role: provider.RoleAssistant, Content: "old answer"})
	executor := agent.New(&recordingProvider{}, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	runner := &turnEventGateRunner{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{}, 1)
	c := New(Options{Runner: runner, Executor: executor, SessionDir: t.TempDir(), SessionPath: filepath.Join(t.TempDir(), "owned.jsonl"), Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- struct{}{}
		}
	})})
	t.Cleanup(c.Close)
	t.Cleanup(func() {
		close(runner.release)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("owned turn did not terminate before profile cleanup")
		}
	})
	c.Submit("new question")
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not start")
	}
	session.Add(provider.Message{ID: "new-user", Role: provider.RoleUser, Content: "new question"})
	session.Add(provider.Message{ID: "unfinished-answer", Role: provider.RoleAssistant, Content: "partial answer"})
	view, err := c.TurnProjectionView()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Prefix) != 3 || view.Prefix[2].ID != "old-answer" || len(view.UserSuffix) != 1 || view.UserSuffix[0].ID != "new-user" || view.Projection.ActiveTurnID == "" {
		t.Fatalf("prefix/suffix: %+v", view)
	}
	// The canonical partial message must not appear in both prefix and replay.
	for _, message := range view.Prefix {
		if message.ID == "unfinished-answer" {
			t.Fatal("partial output leaked into prefix")
		}
	}
	if !c.Running() {
		t.Fatal("spectator read changed engine admission")
	}
	unchanged := session.Snapshot()
	edited := session.Snapshot()
	edited[2].Content = "same ID, different canonical answer"
	session.Replace(edited)
	if _, err := c.TurnProjectionView(); !errors.Is(err, ErrTurnProjectionChanged) {
		t.Fatalf("same-ID rewrite: %v", err)
	}
	session.Replace(unchanged)
	if _, err := c.TurnProjectionView(); err != nil {
		t.Fatalf("restored canonical prefix: %v", err)
	}
	// A rewrite/compaction that removes the base is an explicit reconcile, not
	// a positional cut of unrelated history with the old cursor.
	session.Replace([]provider.Message{{ID: "rewritten", Role: provider.RoleUser, Content: "new base"}})
	if _, err := c.TurnProjectionView(); !errors.Is(err, ErrTurnProjectionChanged) {
		t.Fatalf("rewrite: %v", err)
	}
	if !c.Running() {
		t.Fatal("display mismatch must not cancel provider execution")
	}
}

func TestTurnProjectionViewRejectsMissingAdmissionBase(t *testing.T) {
	l, err := turnevent.Open(filepath.Join(t.TempDir(), "owned.jsonl"), "owned")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	})
	c := &Controller{}
	c.turnEvents.ledger = l
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.TurnProjectionView(); !errors.Is(err, ErrTurnProjectionChanged) {
		t.Fatalf("missing base: %v", err)
	}
}

func TestUserAdmissionPublicationHasCanonicalProjectionAndDurableEvent(t *testing.T) {
	isolateControlConfigHome(t)
	session := agent.NewSession("system")
	prov := &recordingProvider{streams: [][]provider.Chunk{textTurn("fixture answer")}}
	executor := agent.New(prov, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	type observed struct {
		event event.Event
		view  TurnProjectionView
		err   error
	}
	seen := make(chan observed, 1)
	var c *Controller
	c = New(Options{Runner: executor, Executor: executor, SessionDir: t.TempDir(), SessionPath: filepath.Join(t.TempDir(), "owned.jsonl"), Sink: event.FuncSink(func(e event.Event) {
		if e.Kind != event.UserMessageAdmitted {
			return
		}
		view, err := c.TurnProjectionView()
		seen <- observed{e, view, err}
	})})
	t.Cleanup(func() { c.Cancel(); waitIdle(t, c); c.Close() })
	c.Submit("fixture user question")
	select {
	case got := <-seen:
		if got.err != nil || got.event.TurnID == "" || got.event.Sequence == 0 || len(got.view.UserSuffix) != 1 || got.view.UserSuffix[0].ID != got.event.MessageID {
			t.Fatalf("canonical admission projection: %+v", got)
		}
		found := false
		for _, envelope := range got.view.Projection.Events {
			if envelope.Sequence == got.event.Sequence && envelope.Event.Kind == "user_message_admitted" && envelope.Event.MessageID == got.event.MessageID {
				found = true
			}
		}
		if !found {
			t.Fatal("admission callback ran before ledger publication")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing user admission publication")
	}
}
