package agent

import (
	"context"
	"path/filepath"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestRunPersistsRequestUsageAndAssistantTimeWithoutProviderLeak(t *testing.T) {
	mp := testutil.NewMock("model",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "call", Name: "echo", Arguments: `{"text":"hello"}`}}, Usage: &provider.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13}},
		testutil.Turn{Text: "done", Usage: &provider.Usage{PromptTokens: 20, CompletionTokens: 5, TotalTokens: 25}},
	)
	session := NewSession("system")
	a := New(mp, echoRegistry(), session, Options{}, event.Discard)
	if err := a.Run(withNoClosedLoop(context.Background()), "question"); err != nil {
		t.Fatal(err)
	}
	for _, req := range mp.Requests() {
		for _, message := range req.Messages {
			if message.RequestUsage != nil {
				t.Fatal("usage leaked to provider")
			}
		}
	}
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	var totals []int
	for _, m := range loaded.Messages {
		if m.Role == provider.RoleAssistant {
			if m.CreatedAt <= 0 || m.RequestUsage == nil {
				t.Fatalf("missing saved metadata: %+v", m)
			}
			totals = append(totals, m.RequestUsage.TotalTokens)
		}
	}
	if len(totals) != 2 || totals[0] != 13 || totals[1] != 25 {
		t.Fatalf("reloaded accounting: %v", totals)
	}
}

func TestPersistedUsageOwnsCopyAndRetainsUnknown(t *testing.T) {
	usage := &provider.Usage{TotalTokens: 12}
	saved := persistedRequestUsage(usage)
	usage.TotalTokens = 99
	if saved.TotalTokens != 12 || saved.RequestCount != 1 {
		t.Fatalf("saved: %+v", saved)
	}
	if unknown := persistedRequestUsage(nil); !unknown.Unknown || unknown.RequestCount != 1 {
		t.Fatalf("unknown: %+v", unknown)
	}
	if normalized := mergeSamplingUsage(nil, &provider.Usage{PromptTokens: 12, TotalTokens: 12}); !normalized.CacheAccountingUnknown || normalized.CacheMissTokens != 12 {
		t.Fatalf("missing cache split lost provenance: %+v", normalized)
	}
}
