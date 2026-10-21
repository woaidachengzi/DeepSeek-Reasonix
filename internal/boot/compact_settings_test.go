package boot

import (
	"math"
	"sync/atomic"
	"testing"

	"reasonix/internal/ablation"
	"reasonix/internal/config"
)

func TestSubagentSkillsSnapshotCurrentCompactRatio(t *testing.T) {
	var bits atomic.Uint64
	source := func() float64 { return math.Float64frombits(bits.Load()) }
	factory := newSubagentSkillOptionsFactory(config.AgentConfig{CompactRatio: 0.8}, nil, nil, 0, 1, ablation.Set{}, nil, nil, source)
	if initial := factory(t.Context(), 0, nil, 10000, 1); initial.CompactRatio != 0.8 {
		t.Fatalf("unset source changed the initial threshold: %v", initial.CompactRatio)
	}
	bits.Store(math.Float64bits(0.7))
	first := factory(t.Context(), 0, nil, 10000, 1)
	bits.Store(math.Float64bits(0.75))
	next := factory(t.Context(), 0, nil, 10000, 1)
	if first.CompactRatio != 0.7 || next.CompactRatio != 0.75 || first.CompactRatioSource != nil || next.CompactRatioSource != nil {
		t.Fatal("skill children did not freeze their own construction-time settings")
	}
}
