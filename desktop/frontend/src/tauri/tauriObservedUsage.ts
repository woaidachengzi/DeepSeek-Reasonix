import type { TauriBridgeEvent } from "../lib/tauriBridge";

export interface TauriObservedUsage {
  sessionId: string;
  tokens: number;
  turnTokens: number;
  turnOutputTokens: number;
  turnCacheTokens: number;
  turns: number;
  turnCost: number;
  turnCurrency: string;
  turnCostComplete: boolean;
  sessionCost: number;
  sessionCurrency: string;
  costComplete: boolean;
}

function safeTokenCount(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

/** Count only usage frames actually received in this synchronized stream. */
export function observeTauriUsage(previous: TauriObservedUsage | null, event: TauriBridgeEvent): TauriObservedUsage | null {
  if (event.eventKind === "turn_started") {
    const sameSession = previous?.sessionId === event.sessionId;
    return {
      sessionId: event.sessionId,
      tokens: sameSession ? previous.tokens : 0,
      turnTokens: 0,
      turnOutputTokens: 0,
      turnCacheTokens: 0,
      turns: (sameSession ? previous.turns : 0) + 1,
      turnCost: 0,
      turnCurrency: "",
      turnCostComplete: true,
      sessionCost: sameSession ? previous.sessionCost : 0,
      sessionCurrency: sameSession ? previous.sessionCurrency : "",
      costComplete: sameSession ? previous.costComplete : true,
    };
  }
  if (event.eventKind !== "usage") return previous;
  const usage = event.payload.usage;
  if (!usage || typeof usage !== "object" || Array.isArray(usage)) return previous;
  const data = usage as Record<string, unknown>;
  const tokens = safeTokenCount(data.totalTokens);
  if (tokens === null) return previous;
  const outputTokens = safeTokenCount(data.completionTokens) ?? 0;
  const cacheTokens = (safeTokenCount(data.cacheHitTokens) ?? 0) + (safeTokenCount(data.cacheMissTokens) ?? 0);
  const sameSession = previous?.sessionId === event.sessionId;
  const baseline = sameSession ? previous : { sessionId: event.sessionId, tokens: 0, turnTokens: 0, turnOutputTokens: 0, turnCacheTokens: 0, turns: 1, turnCost: 0, turnCurrency: "", turnCostComplete: true, sessionCost: 0, sessionCurrency: "", costComplete: true };
  const nextTokens = baseline.tokens + tokens;
  const nextTurnTokens = baseline.turnTokens + tokens;
  const nextTurnOutputTokens = baseline.turnOutputTokens + outputTokens;
  const nextTurnCacheTokens = baseline.turnCacheTokens + cacheTokens;
  if (![nextTokens, nextTurnTokens, nextTurnOutputTokens, nextTurnCacheTokens].every(Number.isSafeInteger)) return previous;
  const quote = data.costQuote && typeof data.costQuote === "object" && !Array.isArray(data.costQuote) ? data.costQuote as Record<string, unknown> : null;
  const selected = quote?.selected && typeof quote.selected === "object" && !Array.isArray(quote.selected) ? quote.selected as Record<string, unknown> : null;
  const selectedAmount = typeof selected?.amount === "string" ? Number(selected.amount) : typeof selected?.amount === "number" ? selected.amount : typeof data.cost === "number" ? data.cost : Number.NaN;
  const selectedCurrency = typeof selected?.currency === "string" ? selected.currency : typeof data.currencyCode === "string" ? data.currencyCode : typeof data.currency === "string" ? data.currency : "";
  const hasCompleteCost = Number.isFinite(selectedAmount) && selectedAmount >= 0 && Boolean(selectedCurrency)
    && quote?.costComplete !== false && data.costComplete !== false;
  const costCurrency = baseline.sessionCurrency || selectedCurrency;
  const canAggregateCost = hasCompleteCost && (!baseline.sessionCurrency || baseline.sessionCurrency === selectedCurrency) && baseline.costComplete;
  const nextSessionCost = canAggregateCost ? baseline.sessionCost + selectedAmount : baseline.sessionCost;
  const turnCurrency = baseline.turnCurrency || selectedCurrency;
  const canAggregateTurnCost = hasCompleteCost && (!baseline.turnCurrency || baseline.turnCurrency === selectedCurrency) && baseline.turnCostComplete;
  const nextTurnCost = canAggregateTurnCost ? baseline.turnCost + selectedAmount : baseline.turnCost;
  return {
    sessionId: event.sessionId,
    tokens: nextTokens,
    turnTokens: nextTurnTokens,
    turnOutputTokens: nextTurnOutputTokens,
    turnCacheTokens: nextTurnCacheTokens,
    turns: baseline.turns,
    turnCost: Number.isFinite(nextTurnCost) ? nextTurnCost : baseline.turnCost,
    turnCurrency,
    turnCostComplete: canAggregateTurnCost,
    sessionCost: Number.isFinite(nextSessionCost) ? nextSessionCost : baseline.sessionCost,
    sessionCurrency: costCurrency,
    costComplete: canAggregateCost,
  };
}
