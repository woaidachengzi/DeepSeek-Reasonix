package agent

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

func TestCompactRatioLiveSourceAndChildSnapshots(t *testing.T) {
	var bits atomic.Uint64
	source := func() float64 { return math.Float64frombits(bits.Load()) }
	a := New(nil, tool.NewRegistry(), NewSession(""), Options{ContextWindow: 10000, CompactRatio: 0.8, CompactRatioSource: source}, event.Discard)
	task := NewTaskToolWithOptions(TaskToolOptions{CompactRatio: 0.8, CompactRatioSource: source})
	if a.CompactRatio() != 0.8 || a.compactTrigger() != 8000 {
		t.Fatal("unset live source changed the initial threshold")
	}
	bits.Store(math.Float64bits(0.7))
	childOpts := task.subagentOptions(testTaskContext(), 0, nil, 10000, 1, "", nil)
	child := New(nil, tool.NewRegistry(), NewSession(""), childOpts, event.Discard)
	bits.Store(math.Float64bits(0.75))
	if a.compactTrigger() != 7500 || child.compactTrigger() != 7000 {
		t.Fatalf("root=%d child=%d", a.compactTrigger(), child.compactTrigger())
	}
	nextOpts := task.subagentOptions(testTaskContext(), 0, nil, 10000, 1, "", nil)
	if nextOpts.CompactRatio != 0.75 || nextOpts.CompactRatioSource != nil {
		t.Fatalf("new child did not snapshot current settings: %+v", nextOpts)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			bits.Store(math.Float64bits(0.7))
			bits.Store(math.Float64bits(0.8))
		}
	})
	for range 1000 {
		_ = a.compactTrigger()
		_ = task.currentCompactRatio()
	}
	wg.Wait()
}
