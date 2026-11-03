package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

// Keep the original whole-transcript projector as the compatibility oracle:
// the page's offsets count display rows, not provider or tool messages.
func assertBridgeHistoryPageParity(t *testing.T, cache *bridgeHistoryCache, raw []provider.Message, limit int) []desktopbridge.HistoryMessage {
	t.Helper()
	all := projectBridgeHistory(raw)
	start := max(0, len(all)-limit)
	want := append([]desktopbridge.HistoryMessage{}, all[start:]...)
	got, total := cache.snapshot(limit)
	if total != len(all) {
		t.Fatalf("projected total = %d, want %d (raw count %d)", total, len(all), len(raw))
	}
	if !reflect.DeepEqual(got, want) {
		for i := range min(len(got), len(want)) {
			if !reflect.DeepEqual(got[i], want[i]) {
				t.Fatalf("page[%d] differs from projected[%d]:\n got %#v\nwant %#v", i, start+i, got[i], want[i])
			}
		}
		t.Fatalf("page differs: length %d, want %d; nil = %v", len(got), len(want), got == nil)
	}
	return got
}

func bridgeHistoryPageUsage(tokens int) *provider.Usage {
	return &provider.Usage{PromptTokens: tokens, CompletionTokens: 1, TotalTokens: tokens + 1, CacheHitTokens: tokens, RequestCount: 1}
}

func bridgeHistoryPageFailure() provider.Message {
	return provider.Message{
		Role: provider.RoleAssistant, LocalOnly: true, Content: "private failed output", ReasoningContent: "private reasoning",
		RequestUsage: bridgeHistoryPageUsage(50_000), WorkDurationMs: 456, CreatedAt: 789,
		InterruptedTurn: &provider.InterruptedTurnRecovery{TerminalStatus: "failed", Pending: true,
			FailureDiagnostic: &provider.FailureDiagnostic{Kind: "no_deliverable_output", TraceID: "private trace"}},
	}
}

func TestBridgeHistoryPageProjectionBoundariesAndAppendParity(t *testing.T) {
	for _, count := range []int{0, 1, 199, 200, 201, 407} {
		t.Run(fmt.Sprintf("visible-%d", count), func(t *testing.T) {
			raw := []provider.Message{{Role: provider.RoleSystem, Content: "private system"}}
			for i := range count {
				// Hidden raw records deliberately make raw and projected offsets differ.
				raw = append(raw, provider.Message{Role: provider.RoleTool, Content: fmt.Sprintf("private tool %d", i)},
					provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("answer %d", i), RequestUsage: bridgeHistoryPageUsage(1)})
			}
			for _, chunk := range []int{1, 37, len(raw)} {
				t.Run(fmt.Sprintf("chunk-%d", chunk), func(t *testing.T) {
					var cache bridgeHistoryCache
					cache.update(nil, true)
					assertBridgeHistoryPageParity(t, &cache, nil, 200)
					for start := 0; start < len(raw); start += chunk {
						end := min(start+chunk, len(raw))
						cache.update(raw[start:end], false)
						assertBridgeHistoryPageParity(t, &cache, raw[:end], 200)
					}
					cache.update(nil, false)
					for _, limit := range []int{1, 7, 200} {
						assertBridgeHistoryPageParity(t, &cache, raw, limit)
					}
				})
			}
		})
	}
}

func TestBridgeHistoryPageUsageAcrossEvictionAndHiddenRows(t *testing.T) {
	context := sessioncontext.Build(sessioncontext.Sections{Environment: "darwin/arm64", Workspace: "Current workspace: /tmp/history-parity"})
	raw := []provider.Message{{Role: provider.RoleUser, Content: "first question"}}
	// A single turn exceeds the display page. The first returned row must still
	// include usage from discarded visible rows and hidden tool requests.
	for i := range 230 {
		raw = append(raw,
			provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: fmt.Sprintf("call-%d", i), Name: "echo"}}, RequestUsage: bridgeHistoryPageUsage(2)},
			provider.Message{Role: provider.RoleTool, Content: "private tool result"},
			provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("answer %d", i), RequestUsage: bridgeHistoryPageUsage(3)})
	}
	raw = append(raw,
		provider.Message{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "private host instruction"},
		provider.Message{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: context.Content},
		provider.Message{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "<pinned_context_revision>private pinned revision</pinned_context_revision>"},
		provider.Message{Role: provider.RoleAssistant, Content: "host rows did not reset usage", RequestUsage: bridgeHistoryPageUsage(4)},
		// Visible user-role messages reset usage before empty display text is skipped.
		provider.Message{Role: provider.RoleUser, Content: " \t\r\n"},
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "missing-usage", Name: "echo"}}},
		bridgeHistoryPageFailure(), // Synthetic failure must not add its usage.
		provider.Message{Role: provider.RoleAssistant, Content: "after blank user", RequestUsage: bridgeHistoryPageUsage(5)},
		provider.Message{Role: provider.RoleAssistant, Content: "invalid usage", RequestUsage: &provider.Usage{TotalTokens: -1}},
		provider.Message{Role: provider.RoleAssistant, Content: "overflow usage", RequestUsage: &provider.Usage{TotalTokens: int(maxHistoryTokenCount)}},
		provider.Message{Role: provider.RoleUser, Content: "<pinned_context_revision>user quotation stays visible</pinned_context_revision>"},
		provider.Message{Role: provider.RoleAssistant, Content: "known zero", RequestUsage: &provider.Usage{Unknown: true}},
		provider.Message{Role: provider.RoleUser, Content: "provider wrapper", RawContent: " exact raw question "},
		provider.Message{Role: provider.RoleAssistant, Content: "tool-call text stays visible", ToolCalls: []provider.ToolCall{{ID: "visible-call", Name: "echo"}}, RequestUsage: bridgeHistoryPageUsage(6)},
		provider.Message{Role: provider.RoleAssistant, Content: "local assistant remains visible", LocalOnly: true, RequestUsage: bridgeHistoryPageUsage(7)})
	var cache bridgeHistoryCache
	for i := range raw {
		cache.update(raw[i:i+1], i == 0)
		if i > len(raw)-20 || i%97 == 0 {
			assertBridgeHistoryPageParity(t, &cache, raw[:i+1], 200)
		}
	}
	page := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if page[0].TurnUsage == nil || page[0].TurnUsage.TotalTokens <= 7 {
		t.Fatal("fixture did not retain usage from before the display page")
	}
	for _, row := range page {
		if strings.Contains(row.Content, "private") {
			t.Fatalf("private row crossed display boundary: %q", row.Content)
		}
		if row.Content == "after blank user" && (row.TurnUsage == nil || row.TurnUsage.TotalTokens != 6 || row.TurnUsage.Complete || row.TurnUsage.CacheHitTokens != nil) {
			t.Fatalf("blank reset, missing usage, or synthetic failure accounting changed: %#v", row.TurnUsage)
		}
		if row.Content == "tool-call text stays visible" && row.ForkToken != "" {
			t.Fatal("tool-call assistant acquired a fork token")
		}
	}
}

func TestBridgeHistoryPageUnicodeAndRawUserContentParity(t *testing.T) {
	boundary := strings.Repeat("a", bridgeHistoryMaxContentRunes-2) + "界😀"
	raw := []provider.Message{
		{Role: provider.RoleAssistant, Content: boundary},
		{Role: provider.RoleAssistant, Content: boundary + "e\u0301"},
		{Role: provider.RoleUser, Content: "provider-safe summary", RawContent: "  " + boundary + "界  "},
		{Role: provider.RoleAssistant, Content: "legacy invalid UTF-8 \xff"},
		{Role: provider.RoleAssistant, Content: "\xff" + strings.Repeat("界", bridgeHistoryMaxContentRunes)},
	}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	page := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if page[0].Truncated || !page[1].Truncated || !page[2].Truncated || page[3].Truncated || !page[4].Truncated {
		t.Fatalf("rune boundary truncation changed: %#v", page)
	}
	if utf8.ValidString(page[3].Content) || !utf8.ValidString(page[4].Content) {
		t.Fatal("legacy invalid UTF-8 normalization changed at the truncation boundary")
	}
}

func TestBridgeHistoryPageResetRewindAndForkIdentity(t *testing.T) {
	raw := []provider.Message{{Role: provider.RoleSystem, Content: "first system"}}
	for i := range 120 {
		raw = append(raw, provider.Message{Role: provider.RoleUser, Content: fmt.Sprintf("question %d", i)},
			provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("answer %d", i), RequestUsage: bridgeHistoryPageUsage(1)})
	}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	assertBridgeHistoryPageParity(t, &cache, raw, 200)
	// Rewind removes rows and invalidates the former tail; subsequent appends
	// must begin hashing and accounting from the replacement transcript.
	raw = append([]provider.Message(nil), raw[:87]...)
	cache.update(raw, true)
	assertBridgeHistoryPageParity(t, &cache, raw, 200)
	appended := []provider.Message{{Role: provider.RoleUser, Content: "new branch"}, {Role: provider.RoleAssistant, Content: "branch answer", RequestUsage: bridgeHistoryPageUsage(3)}}
	raw = append(raw, appended...)
	cache.update(appended, false)
	before := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	// Refreshing excluded host context retains answer identities, while edits
	// to included metadata invalidate every later opaque token.
	raw[0].Content = "replacement system"
	cache.update(raw, true)
	after := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if before[len(before)-1].ForkToken != after[len(after)-1].ForkToken {
		t.Fatal("system refresh invalidated an existing answer identity")
	}
	raw[1].CreatedAt = 9
	cache.update(raw, true)
	changed := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if changed[len(changed)-1].ForkToken == after[len(after)-1].ForkToken {
		t.Fatal("same-length canonical metadata rewrite retained a stale fork token")
	}
	cache.update(nil, true)
	assertBridgeHistoryPageParity(t, &cache, nil, 200)
}

func TestBridgeHistoryPageForkHashErrorClearsWholePageUntilReset(t *testing.T) {
	raw := []provider.Message{
		// Excluded messages are never marshaled by the prefix hash.
		{Role: provider.RoleSystem, Content: "system", ReadResult: json.RawMessage(`{`)},
		{Role: provider.RoleUser, Content: "question"},
		{Role: provider.RoleAssistant, Content: "answer", RequestUsage: bridgeHistoryPageUsage(2)},
	}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	before := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if before[1].ForkToken == "" {
		t.Fatal("excluded malformed system metadata unexpectedly poisoned hashing")
	}
	bad := provider.Message{Role: provider.RoleTool, Content: "hidden malformed result", ReadResult: json.RawMessage(`{`)}
	raw = append(raw, bad)
	cache.update([]provider.Message{bad}, false)
	page := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if page[1].ForkToken != "" || before[1].ForkToken == "" {
		t.Fatal("hash error failed to clear prior cache tokens or mutated an earlier returned page")
	}
	next := provider.Message{Role: provider.RoleAssistant, Content: "later answer", RequestUsage: bridgeHistoryPageUsage(3)}
	raw = append(raw, next)
	cache.update([]provider.Message{next}, false)
	assertBridgeHistoryPageParity(t, &cache, raw, 200)
	raw = append(raw[:3:3], next)
	cache.update(raw, true)
	restored := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if restored[len(restored)-1].ForkToken == "" {
		t.Fatal("canonical reset did not recover from the removed hash error")
	}
}

func TestBridgeHistoryPageReturnedRowsAreIsolated(t *testing.T) {
	raw := []provider.Message{{Role: provider.RoleUser, Content: "question"}, {Role: provider.RoleAssistant, Content: "answer", RequestUsage: bridgeHistoryPageUsage(3)}}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	page := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	page[0].Content = "mutated user"
	page[1].Role = "tool"
	page[1].ForkToken = "mutated token"
	page[1].TurnUsage.TotalTokens = -1
	*page[1].TurnUsage.CacheHitTokens = -2
	*page[1].TurnUsage.CacheMissTokens = -3
	old := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	next := provider.Message{Role: provider.RoleAssistant, Content: "continuation", RequestUsage: bridgeHistoryPageUsage(4)}
	cache.update([]provider.Message{next}, false)
	assertBridgeHistoryPageParity(t, &cache, append(raw, next), 200)
	if !reflect.DeepEqual(old, projectBridgeHistory(raw)) {
		t.Fatal("appending changed a previously returned page")
	}
}

func TestBridgeHistoryPageSyntheticFailureReflectsCurrentLocale(t *testing.T) {
	saved := i18n.M.ProviderErrNoDeliverable
	t.Cleanup(func() { i18n.M.ProviderErrNoDeliverable = saved })
	i18n.M.ProviderErrNoDeliverable = "initial localized failure"
	raw := []provider.Message{{Role: provider.RoleAssistant, Content: i18n.M.ProviderErrNoDeliverable}, bridgeHistoryPageFailure()}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	old := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	i18n.M.ProviderErrNoDeliverable = "切换语言后的安全失败提示"
	page := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if page[0].Content != "initial localized failure" || page[1].Content != i18n.M.ProviderErrNoDeliverable || old[1].Content != "initial localized failure" {
		t.Fatal("locale refresh rewrote ordinary assistant text or an earlier returned page")
	}
}
