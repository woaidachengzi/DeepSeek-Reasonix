package main

import (
	"testing"

	"reasonix/internal/provider"
)

func TestHistoryUsageIncludesHiddenToolRequestsAndResetsAtUser(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "first"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call", Name: "echo"}}, RequestUsage: &provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CacheHitTokens: 70, CacheMissTokens: 30}},
		{Role: provider.RoleTool, Content: "result"},
		{Role: provider.RoleAssistant, Content: "answer", CreatedAt: 123, RequestUsage: &provider.Usage{PromptTokens: 200, CompletionTokens: 50, TotalTokens: 250, ReasoningTokens: 10, CacheHitTokens: 150, CacheMissTokens: 50, RequestCount: 2}},
		{Role: provider.RoleUser, Content: "second"},
		{Role: provider.RoleAssistant, Content: "answer two", RequestUsage: &provider.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}},
	}
	history := projectBridgeHistory(messages)
	if len(history) != 4 {
		t.Fatalf("visible history: %+v", history)
	}
	u := history[1].TurnUsage
	if u == nil || u.TotalTokens != 370 || u.InputTokens != 300 || u.OutputTokens != 70 || u.ReasoningTokens != 10 || u.RequestCount != 3 || !u.Complete || u.Estimated || u.CacheHitTokens == nil || *u.CacheHitTokens != 220 || history[1].CreatedAtMs != 123 {
		t.Fatalf("first turn accounting: %+v", u)
	}
	if history[0].TurnUsage != nil || history[3].TurnUsage.TotalTokens != 7 || history[3].TurnUsage.CacheHitTokens != nil {
		t.Fatalf("cross-turn or unknown cache: %+v", history)
	}
}

func TestHistoryUsageMissingEstimatedInvalidAndSnapshotIsolation(t *testing.T) {
	h := newHistoryUsage()
	h.add(nil)
	if h.snapshot(true) != nil {
		t.Fatal("legacy usage was fabricated")
	}
	h.add(&provider.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13, Estimated: true})
	first := h.snapshot(true)
	if first.Complete || !first.Estimated || first.TotalTokens != 13 || first.CacheHitTokens != nil {
		t.Fatalf("partial estimated: %+v", first)
	}
	h.add(&provider.Usage{TotalTokens: -1})
	h.add(&provider.Usage{TotalTokens: int(maxHistoryTokenCount)})
	if h.snapshot(true).TotalTokens != 13 || first.TotalTokens != 13 {
		t.Fatal("invalid count overflow or mutable snapshot")
	}
	u := newHistoryUsage()
	u.add(&provider.Usage{Unknown: true, RequestCount: 1})
	if u.snapshot(true).Complete || u.snapshot(true).TotalTokens != 0 {
		t.Fatal("unknown request is not exact zero usage")
	}
}
