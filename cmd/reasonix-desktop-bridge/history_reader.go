package main

import (
	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

const bridgeHistoryReadChunkSize = 64

type bridgeHistoryReader interface {
	Begin(agent.SessionHistoryCursor) (agent.SessionHistoryCursor, int, bool)
	Chunk(agent.SessionHistoryCursor, int, []provider.Message) ([]provider.Message, bool)
	Delta(agent.SessionHistoryCursor) ([]provider.Message, agent.SessionHistoryCursor, bool)
}

// Only complete, consistent rebuilds replace the current projection. Bounded
// retries keep a busy rewriting session from starving history requests.
func (c *bridgeHistoryCache) readPage(reader bridgeHistoryReader, limit int) ([]desktopbridge.HistoryMessage, int) {
	var scratch []provider.Message
	defer func() { clear(scratch) }()
	for attempt := 0; attempt < 2; attempt++ {
		target, start, reset := reader.Begin(c.cursor)
		if !reset {
			if start != target.MessageCount() {
				messages, next, changed := reader.Delta(c.cursor)
				c.update(messages, changed)
				target = next
			}
			c.cursor = target
			return c.snapshot(limit)
		}
		if scratch == nil {
			scratch = make([]provider.Message, bridgeHistoryReadChunkSize)
		}
		var replacement bridgeHistoryCache
		if replacement.rebuild(reader, target, scratch) {
			c.installProjection(&replacement)
			c.cursor = target
			return c.snapshot(limit)
		}
	}
	// The established atomic snapshot is the bounded-retry fallback. It keeps
	// the previous valid cache intact until a coherent replacement is available.
	messages, next, reset := reader.Delta(c.cursor)
	c.update(messages, reset)
	c.cursor = next
	return c.snapshot(limit)
}

func (c *bridgeHistoryCache) rebuild(reader bridgeHistoryReader, target agent.SessionHistoryCursor, scratch []provider.Message) bool {
	retainFrom, valid := historyReadRetainFrom(reader, target, scratch)
	if !valid {
		return false
	}
	c.updateRetained(nil, true, 0)
	return c.readChunks(reader, target, scratch, retainFrom, nil)
}

func (c *bridgeHistoryCache) readChunks(reader bridgeHistoryReader, target agent.SessionHistoryCursor, scratch []provider.Message, retainFrom int, visibleRange *bridgeHistoryRange) bool {
	for start := 0; start < target.MessageCount(); {
		messages, valid := reader.Chunk(target, start, scratch[:0])
		if !valid || len(messages) == 0 {
			return false
		}
		c.updateRange(messages, false, max(0, retainFrom-start), visibleRange)
		start += len(messages)
	}
	return true
}

// Cold requests project only the requested display rows and do not evict the
// runtime's hot newest page. The full scan still establishes count, turn
// usage and prefix tokens from one coherent transcript snapshot.
func readBridgeHistoryBefore(reader bridgeHistoryReader, before, limit int) ([]desktopbridge.HistoryMessage, int) {
	limit = min(limit, bridgeHistoryCacheCapacity)
	visibleRange := bridgeHistoryRange{start: max(0, before-limit), end: before}
	scratch := make([]provider.Message, bridgeHistoryReadChunkSize)
	defer clear(scratch)
	var page bridgeHistoryCache
	for range 2 {
		target, _, _ := reader.Begin(agent.SessionHistoryCursor{})
		page.updateRetained(nil, true, 0)
		if page.readChunks(reader, target, scratch, 0, &visibleRange) {
			return page.snapshot(limit)
		}
	}
	messages, _, _ := reader.Delta(agent.SessionHistoryCursor{})
	page.updateRange(messages, true, 0, &visibleRange)
	return page.snapshot(limit)
}

func historyReadRetainFrom(reader bridgeHistoryReader, target agent.SessionHistoryCursor, scratch []provider.Message) (int, bool) {
	visible := 0
	for end := target.MessageCount(); end > 0; {
		start := max(0, end-cap(scratch))
		messages, valid := reader.Chunk(target, start, scratch[:0:end-start])
		if !valid || len(messages) != end-start {
			return 0, false
		}
		for index := len(messages) - 1; index >= 0; index-- {
			if bridgeHistoryRowVisible(messages[index]) {
				visible++
				if visible == bridgeHistoryCacheCapacity {
					return start + index, true
				}
			}
		}
		end = start
	}
	return 0, true
}

// Transfer ownership once; never copy a live hash and then mutate both owners.
func (c *bridgeHistoryCache) installProjection(next *bridgeHistoryCache) {
	c.prefix, c.usage, c.rows = next.prefix, next.usage, next.rows
	c.head, c.count, c.total = next.head, next.count, next.total
	c.invalidTokens = next.invalidTokens
}
