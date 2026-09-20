package desktopbridge

import "testing"

func TestEventLedgerReplaysAndRequiresResyncAfterOverflow(t *testing.T) {
	ledger := NewEventLedger(2)
	first := ledger.Append(Event{EventKind: "first", SessionID: "a"})
	second := ledger.Append(Event{EventKind: "second", SessionID: "a"})
	third := ledger.Append(Event{EventKind: "third", SessionID: "a"})
	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("sequences = %d, %d, %d", first.Sequence, second.Sequence, third.Sequence)
	}
	if _, resync := ledger.After(0); !resync {
		t.Fatal("stale replay did not require resync")
	}
	events, resync := ledger.After(1)
	if resync || len(events) != 2 || events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("after(1) = %#v, resync=%v", events, resync)
	}
}
