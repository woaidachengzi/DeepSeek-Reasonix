package desktopbridge

// RuntimeHistoryPageProvider returns at most limit newest display rows and
// their total projected count from one snapshot. Returned data stays immutable;
// the manager copies it. Runtimes without this optional surface use History.
type RuntimeHistoryPageProvider interface {
	HistoryPage(limit int) (messages []HistoryMessage, total int)
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
