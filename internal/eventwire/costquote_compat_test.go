package eventwire

import (
	"encoding/json"
	"reflect"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// Ensures older clients still see cost/currency aliases while new clients get costQuote.
func TestToWireUsageDualWritesCostQuoteAndLegacyAliases(t *testing.T) {
	// Synthetic serialization data, not prices or a quote calculation. Catalog
	// and schedule resolution belong to billing tests and must not affect this.
	selected := billing.Money{Amount: "5.67", Currency: "USD"}
	quote := &billing.CostQuote{
		Original: billing.Money{Amount: "12.34", Currency: "CNY"},
		Valuations: map[string]billing.Valuation{
			"CNY": {Money: billing.Money{Amount: "12.34", Currency: "CNY"}, Basis: billing.BasisIdentity, Source: "serialization-fixture"},
			"USD": {Money: selected, Basis: billing.BasisOfficialTable, Source: "serialization-fixture"},
		},
		Selected:           &selected,
		BillingMode:        billing.BillingModePAYG,
		Estimated:          true,
		CostComplete:       true,
		DisplayComplete:    true,
		Complete:           true,
		DisplayStatus:      billing.DisplayStatusMatched,
		AggregateMode:      billing.AggregateModeSingleCurrency,
		ModelRef:           "serialization/model",
		PricingFingerprint: "serialization-fixture",
		RateBand:           billing.RateBandOffPeak,
		RatedAt:            "2026-08-17T00:30:00Z",
	}
	e := event.Event{
		Kind:      event.Usage,
		ModelRef:  quote.ModelRef,
		Usage:     &provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000, TotalTokens: 2_000_000},
		CostQuote: quote,
	}
	w := ToWire(e)
	if w.Usage == nil || w.Usage.CostQuote == nil {
		t.Fatal("missing costQuote")
	}
	if w.Usage.CostQuote.Valuations["USD"].Basis != billing.BasisOfficialTable {
		t.Fatalf("USD basis = %q, want official_table", w.Usage.CostQuote.Valuations["USD"].Basis)
	}
	if w.Usage.Cost <= 0 || w.Usage.CostUSD != w.Usage.Cost {
		t.Fatalf("legacy cost aliases = cost:%v costUsd:%v", w.Usage.Cost, w.Usage.CostUSD)
	}
	if !reflect.DeepEqual(w.Usage.CostQuote, quote) || w.Usage.Cost != 5.67 || w.Usage.Currency != "$" || w.Usage.CurrencyCode != "USD" {
		t.Fatalf("provided quote or selected legacy aliases changed: %+v", w.Usage)
	}
	raw, err := json.Marshal(w.Usage)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("usage json invalid")
	}
	// Old clients ignore unknown fields; new field present.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["costQuote"]; !ok {
		t.Fatalf("costQuote missing from JSON: %s", raw)
	}
	if _, ok := m["cost"]; !ok {
		t.Fatalf("legacy cost missing: %s", raw)
	}
	if m["cost"] != 5.67 || m["costUsd"] != 5.67 || m["currency"] != "$" || m["currencyCode"] != "USD" {
		t.Fatalf("selected legacy aliases missing from JSON: %s", raw)
	}
	quoteJSON, _ := m["costQuote"].(map[string]any)
	if quoteJSON["rateBand"] != billing.RateBandOffPeak || quoteJSON["ratedAt"] != "2026-08-17T00:30:00Z" {
		t.Fatalf("scheduled quote metadata missing: %s", raw)
	}
	valuations, _ := quoteJSON["valuations"].(map[string]any)
	usd, _ := valuations["USD"].(map[string]any)
	if usd["basis"] != billing.BasisOfficialTable {
		t.Fatalf("valuation basis missing from JSON: %s", raw)
	}
}

func TestLegacyCostQuoteWithoutScheduleFieldsStillDecodes(t *testing.T) {
	var quote billing.CostQuote
	if err := json.Unmarshal([]byte(`{"original":{"amount":"1.25","currency":"CNY"},"estimated":true,"complete":true}`), &quote); err != nil {
		t.Fatal(err)
	}
	if quote.Original.Amount != "1.25" || quote.RateBand != "" || quote.RatedAt != "" {
		t.Fatalf("legacy quote changed during decode: %+v", quote)
	}
}
