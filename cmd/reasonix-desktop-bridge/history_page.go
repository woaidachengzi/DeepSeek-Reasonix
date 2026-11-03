package main

import (
	"strings"
	"sync"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
)

// Keep only the bridge's display window. Canonical provider messages, tool
// results, reasoning and image data never become resident cache entries.
const bridgeHistoryCacheCapacity = 200

// History returns only display text; canonical reasoning, tool data and local
// execution metadata remain within the controller's private transcript.
func (r *controllerRuntime) History() []desktopbridge.HistoryMessage {
	return projectBridgeHistory(r.controller.History())
}

func (r *controllerRuntime) HistoryPage(limit int) ([]desktopbridge.HistoryMessage, int) {
	return r.historyCache.page(r.controller, limit)
}

type cachedBridgeHistoryMessage struct {
	value        desktopbridge.HistoryMessage
	localFailure bool
}

// Each concrete runtime owns its cache. SessionHistoryCursor additionally
// fences session replacement, rewrites and in-place edits within that runtime.
type bridgeHistoryCache struct {
	mu            sync.Mutex
	cursor        agent.SessionHistoryCursor
	prefix        agent.ConversationPrefixAccumulator
	usage         *historyUsage
	rows          [bridgeHistoryCacheCapacity]cachedBridgeHistoryMessage
	head          int
	count         int
	total         int
	invalidTokens bool
}

func (c *bridgeHistoryCache) page(controller *control.Controller, limit int) ([]desktopbridge.HistoryMessage, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readPage(controller.HistoryReader(), limit)
}

// update and snapshot run under mu in production. Splitting the incremental
// projector from its source also permits exact full-projector parity tests.
func (c *bridgeHistoryCache) update(messages []provider.Message, reset bool) {
	c.updateRetained(messages, reset, bridgeHistoryRetainFrom(messages))
}

// Only the newest display window needs text copies and per-row usage/token
// snapshots. Counting backwards normally examines just the last few hundred
// raw records; the forward pass still hashes and accounts for the full batch.
func bridgeHistoryRetainFrom(messages []provider.Message) int {
	visible := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if bridgeHistoryRowVisible(messages[index]) {
			visible++
			if visible == bridgeHistoryCacheCapacity {
				return index
			}
		}
	}
	return 0
}

// Synthetic failures are display rows even when their current localized text
// is empty. All ordinary rows follow the full projector's visibility and text
// rules, including RawContent and legacy user-wrapper stripping.
func bridgeHistoryRowVisible(message provider.Message) bool {
	if _, failure := bridgeNoDeliverableHistory(message); failure {
		return true
	}
	return bridgeHistoryMessageVisible(message) && strings.TrimSpace(bridgeHistoryMessageContent(message)) != ""
}

// retainFrom is a raw index relative to messages. It may exceed this slice's
// length when a larger atomic batch is processed in chunks: that chunk updates
// identity, total and usage only. The complete batch must be applied before a
// snapshot is exposed, so its retained tail can replace any older cache rows.
func (c *bridgeHistoryCache) updateRetained(messages []provider.Message, reset bool, retainFrom int) {
	if reset {
		c.prefix = agent.ConversationPrefixAccumulator{}
		clear(c.rows[:])
		c.head, c.count, c.total = 0, 0, 0
		c.invalidTokens = false
	}
	if c.usage == nil {
		c.usage = newHistoryUsage()
	} else if reset {
		resetBridgeHistoryUsage(c.usage)
	}
	for index := range messages {
		message := &messages[index]
		included, err := c.prefix.Append(message)
		c.invalidTokens = c.invalidTokens || err != nil
		if failure, ok := bridgeNoDeliverableHistory(*message); ok {
			if index < retainFrom {
				c.total++
				continue
			}
			// Localization is resolved at read time, including unchanged reads.
			failure.Content = ""
			c.append(cachedBridgeHistoryMessage{value: failure, localFailure: true})
			continue
		}
		visible := bridgeHistoryMessageVisible(*message)
		if message.Role == provider.RoleUser && visible {
			resetBridgeHistoryUsage(c.usage)
		}
		if message.Role == provider.RoleAssistant {
			c.usage.add(message.RequestUsage)
		}
		if !visible {
			continue
		}
		content := bridgeHistoryMessageContent(*message)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if index < retainFrom {
			c.total++
			continue
		}
		content, truncated := truncateBridgeHistoryContent(content)
		role := "user"
		forkToken := ""
		if message.Role == provider.RoleAssistant {
			role = "assistant"
			if included && len(message.ToolCalls) == 0 {
				forkToken = c.prefix.Token()
			}
		}
		c.append(cachedBridgeHistoryMessage{value: desktopbridge.HistoryMessage{
			Role: role, Content: strings.Clone(content), Truncated: truncated,
			WorkDurationMs: message.WorkDurationMs, CreatedAtMs: message.CreatedAt,
			TurnUsage: c.usage.snapshot(role == "assistant"), ForkToken: forkToken,
		}})
	}
}

func resetBridgeHistoryUsage(usage *historyUsage) {
	*usage = historyUsage{value: desktopbridge.HistoryTurnUsage{Complete: true}, cacheKnown: true}
}

func (c *bridgeHistoryCache) append(row cachedBridgeHistoryMessage) {
	if c.count < len(c.rows) {
		c.rows[(c.head+c.count)%len(c.rows)] = row
		c.count++
	} else {
		c.rows[c.head] = row
		c.head = (c.head + 1) % len(c.rows)
	}
	c.total++
}

func (c *bridgeHistoryCache) snapshot(limit int) ([]desktopbridge.HistoryMessage, int) {
	limit = max(0, min(limit, c.count))
	page := make([]desktopbridge.HistoryMessage, limit)
	for i := range page {
		row := c.rows[(c.head+c.count-limit+i)%len(c.rows)]
		value := row.value
		if row.localFailure {
			value.Content = i18n.M.ProviderErrNoDeliverable
		}
		if c.invalidTokens {
			value.ForkToken = ""
		}
		if value.TurnUsage != nil {
			usage := *value.TurnUsage
			if usage.CacheHitTokens != nil {
				hit := *usage.CacheHitTokens
				usage.CacheHitTokens = &hit
			}
			if usage.CacheMissTokens != nil {
				miss := *usage.CacheMissTokens
				usage.CacheMissTokens = &miss
			}
			value.TurnUsage = &usage
		}
		page[i] = value
	}
	return page, c.total
}
