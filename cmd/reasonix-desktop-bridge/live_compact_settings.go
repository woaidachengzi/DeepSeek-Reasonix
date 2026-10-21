package main

import (
	"math"
	"sync"
	"sync/atomic"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// Settings writes invalidate sources even if their runtime is still opening
// and cannot yet be enumerated by the resident pool.
var compactPreferenceRevision atomic.Uint64

type liveCompactionSettings struct {
	bits     atomic.Uint64
	revision atomic.Uint64
	mu       sync.Mutex
	root     string
}

func newLiveCompactionSettings(root string) *liveCompactionSettings {
	s := &liveCompactionSettings{root: root}
	s.revision.Store(compactPreferenceRevision.Load())
	return s
}

func (s *liveCompactionSettings) ratio() float64 {
	revision := compactPreferenceRevision.Load()
	if s.revision.Load() != revision {
		s.mu.Lock()
		if s.revision.Load() != revision {
			if cfg, err := appconfig.LoadForRootWithoutCredentialsReadOnly(s.root); err == nil {
				s.bits.Store(math.Float64bits(cfg.Agent.CompactRatio))
			}
			s.revision.Store(revision)
		}
		s.mu.Unlock()
	}
	return math.Float64frombits(s.bits.Load())
}

func (r *controllerRuntime) SetCompactRatio(ratio float64) bool {
	if r.liveCompactRatio == nil || !(ratio > 0 && ratio < 1) {
		return false
	}
	r.liveCompactRatio.mu.Lock()
	r.liveCompactRatio.bits.Store(math.Float64bits(ratio))
	r.liveCompactRatio.revision.Store(compactPreferenceRevision.Load())
	r.liveCompactRatio.mu.Unlock()
	return true
}

func (r *controllerRuntime) SessionMetrics() desktopbridge.SessionMetrics {
	used, window := r.controller.ContextSnapshot()
	hit, miss := r.controller.SessionCache()
	return desktopbridge.SessionMetrics{
		ContextUsedTokens: max(0, used), ContextWindowTokens: max(0, window),
		CompactThresholdPercent: max(0, min(100, int(r.controller.CompactRatio()*100))),
		CacheHitTokens:          max(0, hit), CacheMissTokens: max(0, miss),
	}
}
