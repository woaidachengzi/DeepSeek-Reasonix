package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"reasonix/internal/i18n"
	"reasonix/internal/provider"
)

func TestBridgeHistoryRetainFromUsesActualDisplayRows(t *testing.T) {
	saved := i18n.M.ProviderErrNoDeliverable
	t.Cleanup(func() { i18n.M.ProviderErrNoDeliverable = saved })
	i18n.M.ProviderErrNoDeliverable = ""
	for _, count := range []int{0, 1, 199, 200, 201, 431} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			raw := []provider.Message{{Role: provider.RoleUser, Content: " \t\n"}}
			for i := range count {
				// A failure is a row even with an empty current translation; a
				// blank ordinary assistant and hidden tool result are not rows.
				row := provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprint(i)}
				if i%7 == 0 {
					row = bridgeHistoryPageFailure()
				}
				raw = append(raw, provider.Message{Role: provider.RoleTool, Content: "private result"}, row,
					provider.Message{Role: provider.RoleAssistant, Content: "\t"})
			}
			start := bridgeHistoryRetainFrom(raw)
			if count < bridgeHistoryCacheCapacity && start != 0 {
				t.Fatalf("small append dropped earlier rows: retainFrom=%d", start)
			}
			if got := len(projectBridgeHistory(raw[start:])); got != min(count, bridgeHistoryCacheCapacity) {
				t.Fatalf("retained display rows = %d, want %d", got, min(count, bridgeHistoryCacheCapacity))
			}
			if start > 0 && len(projectBridgeHistory(raw[:start])) != count-bridgeHistoryCacheCapacity {
				t.Fatal("retained boundary did not count skipped display rows exactly")
			}
			var cache bridgeHistoryCache
			cache.update(raw, true)
			assertBridgeHistoryPageParity(t, &cache, raw, 200)
		})
	}
}

func TestBridgeHistoryRetainedChunksMatchWholeBatch(t *testing.T) {
	for _, size := range []int{17, 260} {
		for _, reset := range []bool{false, true} {
			for _, chunk := range []int{1, 19, 128} {
				t.Run(fmt.Sprintf("turns-%d/reset-%v/chunk-%d", size, reset, chunk), func(t *testing.T) {
					base := historyBenchmarkFixture(48, 0)
					batch := historyBenchmarkFixture(size*4, 0)
					for i := range batch {
						batch[i].ID = "batch-" + batch[i].ID
					}
					var cache bridgeHistoryCache
					cache.update(base, true)
					retainFrom := bridgeHistoryRetainFrom(batch)
					for start := 0; start < len(batch); start += chunk {
						end := min(start+chunk, len(batch))
						cache.updateRetained(batch[start:end], reset && start == 0, max(0, retainFrom-start))
						if cache.count > bridgeHistoryCacheCapacity {
							t.Fatal("chunk processing exceeded the display cache capacity")
						}
					}
					want := batch
					if !reset {
						want = append(base, batch...)
					}
					assertBridgeHistoryPageParity(t, &cache, want, 200)
				})
			}
		}
	}
}

func TestBridgeHistoryRetainedChunksKeepUsageAndHashErrorsOutsideTail(t *testing.T) {
	for _, corrupt := range []string{"none", "discarded prefix", "hidden suffix"} {
		t.Run(corrupt, func(t *testing.T) {
			base := []provider.Message{{Role: provider.RoleUser, Content: "one long turn"}, {Role: provider.RoleAssistant, Content: "early answer", RequestUsage: bridgeHistoryPageUsage(9)}}
			batch := make([]provider.Message, 0, 1_000)
			bad := provider.Message{Role: provider.RoleTool, ReadResult: json.RawMessage(`{`)}
			for i := range 430 {
				batch = append(batch, provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("answer-%d", i), RequestUsage: bridgeHistoryPageUsage(3)},
					provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: fmt.Sprint(i), Name: "echo"}}, RequestUsage: bridgeHistoryPageUsage(2)})
				if i == 8 {
					batch = append(batch, bridgeHistoryPageFailure())
					if corrupt == "discarded prefix" {
						batch = append(batch, bad)
					}
				}
			}
			if corrupt == "hidden suffix" {
				batch = append(batch, bad)
			}
			var cache bridgeHistoryCache
			cache.update(base, true)
			before := assertBridgeHistoryPageParity(t, &cache, base, 200)
			retainFrom := bridgeHistoryRetainFrom(batch)
			if retainFrom < 200 {
				t.Fatal("fixture did not discard a substantial prefix")
			}
			for start := 0; start < len(batch); start += 37 {
				end := min(start+37, len(batch))
				cache.updateRetained(batch[start:end], false, max(0, retainFrom-start))
			}
			page := assertBridgeHistoryPageParity(t, &cache, append(base, batch...), 200)
			if page[0].TurnUsage == nil || page[0].TurnUsage.TotalTokens < 1_000 {
				t.Fatal("discarded rows lost long-turn usage carry")
			}
			if !reflect.DeepEqual(before, projectBridgeHistory(base)) {
				t.Fatal("discarding or hash invalidation mutated a previous returned page")
			}
		})
	}
}

func TestBridgeHistoryUsageResetReusesCarryWithoutChangingSnapshots(t *testing.T) {
	raw := []provider.Message{{Role: provider.RoleUser, Content: "first question"}, {Role: provider.RoleAssistant, Content: "first answer", RequestUsage: bridgeHistoryPageUsage(7)}}
	var cache bridgeHistoryCache
	cache.update(raw, true)
	carry := cache.usage
	before := assertBridgeHistoryPageParity(t, &cache, raw, 200)
	batch := []provider.Message{{Role: provider.RoleUser, Content: "\t"}, {Role: provider.RoleAssistant, Content: "after blank user", RequestUsage: bridgeHistoryPageUsage(2)}}
	cache.update(batch, false)
	if cache.usage != carry {
		t.Fatal("user boundary allocated a replacement usage carry")
	}
	assertBridgeHistoryPageParity(t, &cache, append(raw, batch...), 200)
	cache.update(raw, true)
	if cache.usage != carry {
		t.Fatal("history rebuild allocated a replacement usage carry")
	}
	assertBridgeHistoryPageParity(t, &cache, raw, 200)
	if !reflect.DeepEqual(before, projectBridgeHistory(raw)) {
		t.Fatal("in-place usage reset changed an earlier returned snapshot")
	}
}
