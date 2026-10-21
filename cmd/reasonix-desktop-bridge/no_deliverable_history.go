package main

import (
	"reasonix/internal/desktopbridge"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
)

// Only a known host failure becomes display text. Raw local records, provider
// reasoning, tool arguments, and diagnostic contents remain private.
func bridgeNoDeliverableHistory(m provider.Message) (desktopbridge.HistoryMessage, bool) {
	r := m.InterruptedTurn
	if !m.LocalOnly || r == nil || r.TerminalStatus != "failed" || r.FailureDiagnostic == nil || r.FailureDiagnostic.Kind != "no_deliverable_output" {
		return desktopbridge.HistoryMessage{}, false
	}
	return desktopbridge.HistoryMessage{
		Role: "assistant", Content: i18n.M.ProviderErrNoDeliverable,
		WorkDurationMs: m.WorkDurationMs, CreatedAtMs: m.CreatedAt,
	}, true
}
