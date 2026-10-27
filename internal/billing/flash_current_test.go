package billing

import (
	"testing"
	"time"
)

func TestFlashSeptemberCutoverPreservesHistoricalRates(t *testing.T) {
	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		for _, tc := range []struct {
			at, currency       string
			hit, input, output float64
			band               string
		}{
			{"2026-09-10T03:59:59Z", "USD", 0.014, 0.44, 1.32, RateBandPeak},
			{"2026-09-10T04:00:00Z", "USD", 0.003, 0.15, 0.60, RateBandOffPeak},
			{"2026-09-10T06:00:00Z", "USD", 0.006, 0.30, 1.20, RateBandPeak},
			{"2026-09-10T03:59:59Z", "CNY", 0.10, 3, 9, RateBandPeak},
			{"2026-09-10T04:00:00Z", "CNY", 0.02, 1, 4, RateBandOffPeak},
			{"2026-09-10T06:00:00Z", "CNY", 0.04, 2, 8, RateBandPeak},
		} {
			at, _ := time.Parse(time.RFC3339, tc.at)
			got, ok := ResolveScheduledRate("deepseek", model, tc.currency, BillingModePAYG, ScheduleDeepSeekV4August2026, at)
			if !ok || got.Card != (RateCard{CacheHit: tc.hit, Input: tc.input, Output: tc.output, Currency: tc.currency}) || got.RateBand != tc.band {
				t.Fatalf("%s/%s/%s: %+v %v", model, tc.currency, tc.at, got, ok)
			}
		}
	}
	for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		for _, currency := range []string{"CNY", "USD"} {
			row, ok := LookupCatalog("deepseek", model, currency, BillingModePAYG)
			if !ok || !row.EffectiveFrom.Equal(deepSeekFlashSeptember2026EffectiveAt) {
				t.Fatalf("obsolete current anchor: %+v", row)
			}
			at := time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)
			quote := BuildQuote(QuoteInput{Usage: UsageTokens{PromptTokens: 1_000_000}, Rates: RateCardFromCatalog(row), OccurredAt: at,
				ProviderKind: "deepseek", ModelID: model, ScheduleID: ScheduleDeepSeekV4August2026, DisplayCurrency: currency, BillingMode: BillingModePAYG})
			if quote.RateBand != RateBandPeak || quote.Original.Float64() != row.Input {
				t.Fatalf("current quote: %+v", quote)
			}
		}
	}
}
