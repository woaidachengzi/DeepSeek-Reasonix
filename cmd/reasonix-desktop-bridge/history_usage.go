package main

import (
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

const maxHistoryTokenCount int64 = 1<<53 - 1 // JavaScript's exact integer bound

type historyUsage struct {
	value      desktopbridge.HistoryTurnUsage
	seen       bool
	cacheKnown bool
	hit, miss  int64
}

func newHistoryUsage() *historyUsage {
	return &historyUsage{value: desktopbridge.HistoryTurnUsage{Complete: true}, cacheKnown: true}
}

func (h *historyUsage) add(u *provider.Usage) {
	if u == nil {
		h.value.Complete = false
		h.cacheKnown = false
		return
	}
	requests := u.RequestCount
	if requests <= 0 {
		requests = 1
	}
	counts := []int{u.PromptTokens, u.CompletionTokens, u.TotalTokens, requests, u.ReasoningTokens, u.CacheHitTokens, u.CacheMissTokens}
	totals := []int64{h.value.InputTokens, h.value.OutputTokens, h.value.TotalTokens, h.value.RequestCount, h.value.ReasoningTokens, h.hit, h.miss}
	for i, count := range counts {
		if count < 0 || int64(count) > maxHistoryTokenCount-totals[i] {
			h.value.Complete = false
			h.cacheKnown = false
			return
		}
		totals[i] += int64(count)
	}
	h.seen = true
	h.value.InputTokens, h.value.OutputTokens, h.value.TotalTokens = totals[0], totals[1], totals[2]
	h.value.RequestCount, h.value.ReasoningTokens = totals[3], totals[4]
	h.hit, h.miss = totals[5], totals[6]
	h.value.Estimated = h.value.Estimated || u.Estimated
	h.value.Complete = h.value.Complete && !u.Unknown
	// Providers that omit cache accounting must not look like 0% cache hits.
	h.cacheKnown = h.cacheKnown && !u.Unknown && !u.CacheAccountingUnknown && int64(u.CacheHitTokens)+int64(u.CacheMissTokens) == int64(u.PromptTokens)
}

func (h *historyUsage) snapshot(assistant bool) *desktopbridge.HistoryTurnUsage {
	if !assistant || !h.seen {
		return nil
	}
	copy := h.value
	if h.cacheKnown {
		hit, miss := h.hit, h.miss
		copy.CacheHitTokens, copy.CacheMissTokens = &hit, &miss
	}
	return &copy
}
