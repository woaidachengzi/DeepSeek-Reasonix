package desktopbridge

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

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

func TestEventLedgerReportsNewestSequenceAfterEviction(t *testing.T) {
	ledger := NewEventLedger(1)
	ledger.Append(Event{EventKind: "first"})
	ledger.Append(Event{EventKind: "second"})
	if got := ledger.LatestSequence(); got != 2 {
		t.Fatalf("latest sequence = %d, want 2", got)
	}
}

func TestEventLedgerWraparoundMatchesOrderedWindow(t *testing.T) {
	for _, capacity := range []int{1, 2, 7, 128} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			ledger := NewEventLedger(capacity)
			for i := 1; i <= capacity*5; i++ {
				payload := json.RawMessage(fmt.Sprintf(`{"n":%d}`, i))
				ledger.Append(Event{EventKind: "text", SessionID: "a", Payload: payload})
				payload[0] = '[' // The retained payload must own its source bytes.
				oldest := max(1, i-capacity+1)
				for _, cursor := range []uint64{0, uint64(oldest - 1), uint64(i - 1), uint64(i), math.MaxUint64} {
					got, stale := ledger.After(cursor)
					wantStale := cursor < uint64(oldest-1) || cursor > uint64(i)
					if stale != wantStale {
						t.Fatalf("i=%d cursor=%d stale=%v", i, cursor, stale)
					}
					if stale {
						continue
					}
					first := oldest
					if cursor >= uint64(i) {
						first = i + 1
					} else {
						first = max(first, int(cursor)+1)
					}
					if len(got) != i-first+1 {
						t.Fatalf("i=%d cursor=%d returned=%d", i, cursor, len(got))
					}
					for offset, item := range got {
						n := first + offset
						if item.Sequence != uint64(n) || string(item.Payload) != fmt.Sprintf(`{"n":%d}`, n) {
							t.Fatalf("unordered/mutated replay: %+v", item)
						}
						item.Payload[0] = '['
					}
				}
			}
		})
	}
}

func TestEventLedgerFutureCursorRequiresFreshSnapshot(t *testing.T) {
	ledger := NewEventLedger(2)
	if _, stale := ledger.After(1); !stale {
		t.Fatal("cursor from an earlier process can hide a restarted stream")
	}
	if _, stale := ledger.After(0); stale {
		t.Fatal("empty current stream cannot start")
	}
	ledger.Append(Event{EventKind: "turn_started"})
	if _, stale := ledger.After(2); !stale {
		t.Fatal("future cursor was silently accepted")
	}
	if events, stale := ledger.After(0); stale || len(events) != 1 {
		t.Fatal("valid replay changed")
	}
}
