import assert from "node:assert/strict";
import { observeTauriUsage } from "../tauri/tauriObservedUsage";
import type { TauriBridgeEvent } from "../lib/tauriBridge";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { TauriStatusBar } from "../tauri/TauriStatusBar";
import { LocaleProvider } from "../lib/i18n";

function frame(sessionId: string, eventKind: string, payload: Record<string, unknown> = {}): TauriBridgeEvent {
  return { protocolVersion: 1, sequence: 1, sessionId, eventKind, payload };
}

let usage = observeTauriUsage(null, frame("a", "usage", { usage: { totalTokens: 120, completionTokens: 70, cacheHitTokens: 40, cacheMissTokens: 10 } }));
assert.deepEqual(usage, { sessionId: "a", tokens: 120, turnTokens: 120, turnOutputTokens: 70, turnCacheTokens: 50, turns: 1, turnCost: 0, turnCurrency: "", turnCostComplete: false, sessionCost: 0, sessionCurrency: "", costComplete: false });
usage = observeTauriUsage(usage, frame("a", "usage", { usage: { totalTokens: 30, completionTokens: 10, cacheHitTokens: 10 } }));
assert.deepEqual(usage, { sessionId: "a", tokens: 150, turnTokens: 150, turnOutputTokens: 80, turnCacheTokens: 60, turns: 1, turnCost: 0, turnCurrency: "", turnCostComplete: false, sessionCost: 0, sessionCurrency: "", costComplete: false });
usage = observeTauriUsage(usage, frame("a", "turn_started"));
assert.deepEqual(usage, { sessionId: "a", tokens: 150, turnTokens: 0, turnOutputTokens: 0, turnCacheTokens: 0, turns: 2, turnCost: 0, turnCurrency: "", turnCostComplete: true, sessionCost: 0, sessionCurrency: "", costComplete: false });
assert.equal(observeTauriUsage(usage, frame("a", "usage", { usage: { totalTokens: -3, cacheHitTokens: 0 } })), usage, "invalid counts cannot corrupt the display");
const fresh = observeTauriUsage(usage, frame("b", "usage", { usage: { totalTokens: 8, cacheHitTokens: 0, cost: 0.001, currencyCode: "USD", costComplete: true } }));
assert.deepEqual(fresh, { sessionId: "b", tokens: 8, turnTokens: 8, turnOutputTokens: 0, turnCacheTokens: 0, turns: 1, turnCost: 0.001, turnCurrency: "USD", turnCostComplete: true, sessionCost: 0.001, sessionCurrency: "USD", costComplete: true }, "a new session starts a fresh observation");
const withCost = observeTauriUsage(fresh, frame("b", "usage", { usage: { totalTokens: 10, cacheHitTokens: 0, costQuote: { selected: { amount: "0.002", currency: "USD" }, costComplete: true } } }));
assert.equal(withCost?.sessionCost, 0.003, "only complete costs in one currency are accumulated");
assert.equal(withCost?.turnCost, 0.003, "turn cost follows the same complete single-currency rule");
const nextTurn = observeTauriUsage(withCost, frame("b", "turn_started"));
assert.equal(nextTurn?.turns, 2, "turn count includes only observed turn-start events");
assert.equal(nextTurn?.turnCost, 0, "starting a new turn resets turn cost");
const mixed = observeTauriUsage(
  withCost,
  frame("b", "usage", {
    usage: {
      totalTokens: 10,
      costQuote: {
        selected: { amount: "0.002", currency: "CNY" },
        costComplete: true,
      },
    },
  }),
);
assert.equal(mixed?.costComplete, false, "mixed currencies cannot be combined");
const statusUsage = observeTauriUsage(withCost, frame("b", "usage", { usage: { totalTokens: 45, completionTokens: 37, cacheHitTokens: 5, cacheMissTokens: 2, cost: 0, currencyCode: "USD", costComplete: true } }));
const html = renderToStaticMarkup(createElement(LocaleProvider, null, createElement(TauriStatusBar, { workspace: "/tmp/project", model: "example-model", sessionState: "running", bridgeRunning: true, observedUsage: statusUsage, sessionMetrics: { contextUsedTokens: 2400, contextWindowTokens: 10000, compactThresholdPercent: 80, cacheHitTokens: 300, cacheMissTokens: 100 } })));
assert.match(html, /Observed tokens · 63/);
assert.match(html, /Turn tokens · 63/);
assert.match(html, /Turn output tokens · 37/);
assert.match(html, /Turn cache tokens · 7/);
assert.match(html, /Observed turns · 1/);
assert.match(html, /Estimated session cost|会话估算费用|工作階段估算費用/);
assert.match(html, /\$0.00/);
assert.match(html, /Context · 24%/);
assert.match(html, /Compaction threshold · 80%/);
assert.match(html, /Session cache hits · 75%/);
assert.match(html, /observed in the current sync/, "the value explains why it is not a historical total");
console.log("tauri observed usage: OK");
