package desktopbridge

// RuntimeHistoryPageProvider returns at most limit newest display rows and
// their total projected count from one snapshot. Returned data stays immutable;
// the manager copies it. Runtimes without this optional surface use History.
type RuntimeHistoryPageProvider interface {
	HistoryPage(limit int) (messages []HistoryMessage, total int)
}

// RuntimeHistoryBeforeProvider returns the contiguous display rows preceding
// before and the full projected count from the same snapshot.
type RuntimeHistoryBeforeProvider interface {
	HistoryBefore(before, limit int) (messages []HistoryMessage, total int)
}

func historyBefore(runtime Runtime, before int) ([]HistoryMessage, int, error) {
	if provider, ok := runtime.(RuntimeHistoryBeforeProvider); ok {
		messages, total := provider.HistoryBefore(before, maxHistoryMessages)
		if before > total {
			return nil, 0, ErrInvalidInput
		}
		if len(messages) == min(before, maxHistoryMessages) {
			return messages, total, nil
		}
	}
	messages := runtime.History()
	if before > len(messages) {
		return nil, 0, ErrInvalidInput
	}
	return messages[max(0, before-maxHistoryMessages):before], len(messages), nil
}

// Keep provider caches isolated, including nested usage counters. make also
// preserves the protocol's [] rather than null empty page.
func cloneHistoryMessages(messages []HistoryMessage) []HistoryMessage {
	page := make([]HistoryMessage, len(messages))
	copy(page, messages)
	for i := range page {
		if page[i].TurnUsage == nil {
			continue
		}
		usage := *page[i].TurnUsage
		if usage.CacheHitTokens != nil {
			value := *usage.CacheHitTokens
			usage.CacheHitTokens = &value
		}
		if usage.CacheMissTokens != nil {
			value := *usage.CacheMissTokens
			usage.CacheMissTokens = &value
		}
		page[i].TurnUsage = &usage
	}
	return page
}
