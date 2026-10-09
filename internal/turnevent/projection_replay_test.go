package turnevent

import (
	"errors"
	"sync"
	"testing"
	"weak"

	"reasonix/internal/event"
)

func TestProjectionReplaySamplesOneActiveBoundary(t *testing.T) {
	l := openTestLedger(t, testSessionPath(t), "session")
	for turn := 0; turn < 3; turn++ {
		id, err := l.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, err := l.Append(event.Event{Kind: event.TurnStarted}, event.TurnInProgress); err != nil || !ok {
			t.Fatalf("start: %v %v", ok, err)
		}
		if _, ok, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil || !ok {
			t.Fatalf("text: %v %v", ok, err)
		}
		view, err := l.ProjectionReplay()
		if err != nil || view.ActiveTurnID != id || view.TurnStatus != event.TurnInProgress || len(view.Events) != 2 || view.ResetRequired || view.HasMore {
			t.Fatalf("projection: %+v %v", view, err)
		}
		if view.Events[0].Sequence != view.ReplayAfterSequence+1 || view.NextAfterSequence != view.LatestSequence {
			t.Fatal("projection cursor drift")
		}
		for _, entry := range view.Events {
			if entry.TurnID != id {
				t.Fatal("foreign turn in active suffix")
			}
		}
		if _, ok, err := l.Append(event.Event{Kind: event.TurnDone}, event.TurnCompleted); err != nil || !ok {
			t.Fatalf("done: %v %v", ok, err)
		}
		terminal, err := l.ProjectionReplay()
		if err != nil || terminal.ActiveTurnID != "" || terminal.TurnStatus != event.TurnCompleted || len(terminal.Events) != 0 || terminal.ReplayAfterSequence != terminal.LatestSequence {
			t.Fatalf("terminal: %+v %v", terminal, err)
		}
	}
}

func TestProjectionReplayBoundedPagesAndPoisonRemainStrict(t *testing.T) {
	l := openTestLedger(t, testSessionPath(t), "session")
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < replayMaxEvents+10; i++ {
		if _, ok, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil || !ok {
			t.Fatalf("append: %v %v", ok, err)
		}
	}
	view, err := l.ProjectionReplay()
	if err != nil || len(view.Events) != replayMaxEvents || !view.HasMore || view.NextAfterSequence != replayMaxEvents {
		t.Fatalf("bounded page: %+v %v", view, err)
	}
	l.mu.Lock()
	l.poisoned = errors.New("PRIVATE-storage-error")
	l.mu.Unlock()
	if _, err := l.ProjectionReplay(); !errors.Is(err, ErrTurnLedgerUnavailable) {
		t.Fatalf("poison: %v", err)
	}
	var absent *Ledger
	empty, err := absent.ProjectionReplay()
	if err != nil || empty.Events == nil || len(empty.Events) != 0 || empty.ActiveTurnID != "" {
		t.Fatal("nil projection must be explicit empty page")
	}
}

func TestProjectionReplayConcurrentTerminalAndAdmission(t *testing.T) {
	l := openTestLedger(t, testSessionPath(t), "session")
	var group sync.WaitGroup
	group.Add(1)
	errorsOut := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer group.Done()
		defer close(done)
		for i := 0; i < 30; i++ {
			if _, err := l.Begin(); err != nil {
				errorsOut <- err
				return
			}
			if _, _, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil {
				errorsOut <- err
				return
			}
			if _, _, err := l.Append(event.Event{Kind: event.TurnDone}, event.TurnCompleted); err != nil {
				errorsOut <- err
				return
			}
		}
	}()
	for {
		view, err := l.ProjectionReplay()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range view.Events {
			if entry.TurnID != view.ActiveTurnID || entry.Sequence <= view.ReplayAfterSequence || entry.Sequence > view.LatestSequence {
				t.Fatal("torn lifecycle projection")
			}
		}
		select {
		case <-done:
			group.Wait()
			select {
			case err := <-errorsOut:
				t.Fatal(err)
			default:
			}
			return
		default:
		}
	}
}

func TestProjectionReplayPageKeepsInitialCutoff(t *testing.T) {
	l := openTestLedger(t, testSessionPath(t), "owned")
	l.SetRoutingMetadata("owned-epoch", "owned-submission")
	id, err := l.Begin()
	if err != nil {
		t.Fatal(err)
	}
	l.SetTranscriptSnapshot(3, "initial-digest")
	l.SetTranscriptHead("initial-head", "initial-leaf")
	for i := 0; i < replayMaxEvents+10; i++ {
		if _, _, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil {
			t.Fatal(err)
		}
	}
	first, err := l.ProjectionReplay()
	if err != nil || !first.HasMore {
		t.Fatalf("first: %+v %v", first, err)
	}
	if _, _, err := l.Append(event.Event{Kind: event.Text, Text: "later"}, event.TurnInProgress); err != nil {
		t.Fatal(err)
	}
	l.SetTranscriptSnapshot(4, "terminal-digest")
	l.SetTranscriptHead("terminal-head", "terminal-leaf")
	if _, _, err := l.Append(event.Event{Kind: event.TurnDone}, event.TurnCompleted); err != nil {
		t.Fatal(err)
	}
	page, err := l.ProjectionReplayPage(first.Boundary, first.NextAfterSequence)
	if err != nil || page.HasMore || page.ResetRequired || len(page.Events) != 10 || page.LatestSequence != first.LatestSequence || page.NextAfterSequence != first.LatestSequence {
		t.Fatalf("cutoff: %+v %v", page, err)
	}
	if page.TranscriptRevision != 3 || page.TranscriptDigest != "initial-digest" || page.HeadID != "initial-head" || page.LeafMessageID != "initial-leaf" || page.RuntimeEpoch != "owned-epoch" {
		t.Fatalf("metadata crossed initial cut: %+v", page)
	}
	for _, entry := range page.Events {
		if entry.TurnID != id || entry.Event.Text == "later" || entry.Sequence > first.LatestSequence {
			t.Fatal("new content crossed initial cut")
		}
	}
	empty, err := l.ProjectionReplayPage(first.Boundary, first.LatestSequence)
	if err != nil || empty.Events == nil || len(empty.Events) != 0 || empty.HasMore {
		t.Fatalf("end: %+v %v", empty, err)
	}
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ProjectionReplayPage(first.Boundary, first.NextAfterSequence); !errors.Is(err, ErrProjectionReplayChanged) {
		t.Fatalf("new turn: %v", err)
	}
}

func TestProjectionReplayPageRejectsChangedFences(t *testing.T) {
	l := openTestLedger(t, testSessionPath(t), "owned")
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Append(event.Event{Kind: event.Text, Text: "owned"}, event.TurnInProgress); err != nil {
		t.Fatal(err)
	}
	first, err := l.ProjectionReplay()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*ProjectionReplayBoundary)
		after  uint64
	}{
		{"owner", func(b *ProjectionReplayBoundary) { b.owner = weak.Pointer[Ledger]{} }, 0},
		{"session", func(b *ProjectionReplayBoundary) { b.sessionID = "foreign" }, 0},
		{"turn", func(b *ProjectionReplayBoundary) { b.turnID = "foreign" }, 0},
		{"empty turn", func(b *ProjectionReplayBoundary) { b.turnID = "" }, 0},
		{"epoch", func(b *ProjectionReplayBoundary) { b.runtimeEpoch = "foreign" }, 0},
		{"baseline", func(b *ProjectionReplayBoundary) { b.replayAfterSequence++ }, 1},
		{"future cutoff", func(b *ProjectionReplayBoundary) { b.throughSequence++ }, 0},
		{"future cursor", func(b *ProjectionReplayBoundary) {}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			boundary := first.Boundary
			tc.change(&boundary)
			if _, err := l.ProjectionReplayPage(boundary, tc.after); !errors.Is(err, ErrProjectionReplayChanged) {
				t.Fatalf("fence accepted: %v", err)
			}
		})
	}
	if _, _, err := l.Append(event.Event{Kind: event.TurnDone}, event.TurnCompleted); err != nil {
		t.Fatal(err)
	}
	if err := l.AcknowledgeProjection(first.ActiveTurnID); err != nil {
		t.Fatal(err)
	}
	if err := l.Compact(); err != nil {
		t.Fatal(err)
	}
	if replay, err := l.Replay(0); err != nil || !replay.ResetRequired {
		t.Fatalf("fixture did not checkpoint: %+v %v", replay, err)
	}
	if _, err := l.ProjectionReplayPage(first.Boundary, first.ReplayAfterSequence); !errors.Is(err, ErrProjectionReplayChanged) {
		t.Fatalf("compaction: %v", err)
	}
	l.mu.Lock()
	l.poisoned = errors.New("owned storage failure")
	l.mu.Unlock()
	if _, err := l.ProjectionReplayPage(first.Boundary, first.ReplayAfterSequence); !errors.Is(err, ErrTurnLedgerUnavailable) {
		t.Fatalf("poison: %v", err)
	}
	var absent *Ledger
	if _, err := absent.ProjectionReplayPage(first.Boundary, 0); !errors.Is(err, ErrProjectionReplayChanged) {
		t.Fatalf("nil: %v", err)
	}
}

func TestSteerMessageIdentitySurvivesLedgerReplayAndReopen(t *testing.T) {
	path := testSessionPath(t)
	l := openTestLedger(t, path, "owned")
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	for _, steer := range []event.Event{
		{Kind: event.Steer, Text: "same guidance", ItemID: "owned-inbox", MessageID: "owned-message"},
		{Kind: event.Steer, Text: "same guidance"},
	} {
		if _, ok, err := l.Append(steer, event.TurnInProgress); err != nil || !ok {
			t.Fatalf("steer: %v %v", ok, err)
		}
	}
	first, err := l.ProjectionReplay()
	if err != nil {
		t.Fatal(err)
	}
	page, err := l.ProjectionReplayPage(first.Boundary, first.ReplayAfterSequence)
	if err != nil || len(page.Events) != 2 || page.Events[0].Event.MessageID != "owned-message" || page.Events[0].Event.ItemID != "owned-inbox" || page.Events[1].Event.MessageID != "" {
		t.Fatalf("active association: %+v %v", page, err)
	}
	if _, _, err := l.Append(event.Event{Kind: event.TurnDone}, event.TurnCompleted); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestLedger(t, path, "owned")
	replay, err := reopened.Replay(0)
	if err != nil || len(replay.Events) != 3 || replay.Events[0].Event.MessageID != "owned-message" || replay.Events[0].Event.ItemID != "owned-inbox" || replay.Events[1].Event.MessageID != "" {
		t.Fatalf("saved association: %+v %v", replay, err)
	}
	if _, err := reopened.ProjectionReplayPage(first.Boundary, 0); !errors.Is(err, ErrProjectionReplayChanged) {
		t.Fatalf("reopened ledger accepted old process fence: %v", err)
	}
}
