import type { TauriBridgeEvent } from "../lib/tauriBridge";

export interface TauriObservedUsage {
  sessionId: string;
  tokens: number;
  turnTokens: number;
  turnOutputTokens: number;
  turnTpsTokens: number;
  turnCacheTokens: number;
  latestCacheHitTokens: number | null;
  latestCacheMissTokens: number | null;
  turnTps: number | null;
  turnModelMs: number;
  streamAttemptStartedAt: number | null;
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
export function observeTauriUsage(previous: TauriObservedUsage | null, event: TauriBridgeEvent, nowMs = Date.now()): TauriObservedUsage | null {
  if (event.eventKind === "turn_started") {
    const sameSession = previous?.sessionId === event.sessionId;
    return {
      sessionId: event.sessionId,
      tokens: sameSession ? previous.tokens : 0,
      turnTokens: 0,
      turnOutputTokens: 0,
      turnTpsTokens: 0,
      turnCacheTokens: 0,
      latestCacheHitTokens: null,
      latestCacheMissTokens: null,
      turnTps: null,
      turnModelMs: 0,
      streamAttemptStartedAt: null,
      turns: (sameSession ? previous.turns : 0) + 1,
      turnCost: 0,
      turnCurrency: "",
      turnCostComplete: true,
      sessionCost: sameSession ? previous.sessionCost : 0,
      sessionCurrency: sameSession ? previous.sessionCurrency : "",
      costComplete: sameSession ? previous.costComplete : true,
    };
  }
  if (event.eventKind === "stream_attempt") {
    if (!previous || previous.sessionId !== event.sessionId) return previous;
    const attempt = event.payload.streamAttempt;
    if (!attempt || typeof attempt !== "object" || Array.isArray(attempt)) return previous;
    const action = (attempt as Record<string, unknown>).action;
    if (action === "begin") return { ...previous, streamAttemptStartedAt: nowMs };
    if (action === "discard") return { ...previous, streamAttemptStartedAt: null };
    if (action === "commit") {
      const startedAt = previous.streamAttemptStartedAt;
      const elapsed = startedAt !== null ? nowMs - startedAt : 0;
      return {
        ...previous,
        streamAttemptStartedAt: null,
        turnModelMs: elapsed > 0 && Number.isSafeInteger(elapsed) && Number.isSafeInteger(previous.turnModelMs + elapsed)
          ? previous.turnModelMs + elapsed
          : previous.turnModelMs,
      };
    }
    return previous;
  }
  if (event.eventKind === "turn_done") {
    if (!previous || previous.sessionId !== event.sessionId) return previous;
    const turnTps = previous.turnModelMs > 0 && previous.turnTpsTokens > 0
      ? previous.turnTpsTokens / (previous.turnModelMs / 1_000)
      : null;
    return { ...previous, turnTps: Number.isFinite(turnTps) && turnTps !== null && turnTps > 0 ? turnTps : null, streamAttemptStartedAt: null };
  }
  if (event.eventKind !== "usage") return previous;
  const usage = event.payload.usage;
  if (!usage || typeof usage !== "object" || Array.isArray(usage)) return previous;
  const data = usage as Record<string, unknown>;
  const tokens = safeTokenCount(data.totalTokens);
  if (tokens === null) return previous;
  const outputTokens = safeTokenCount(data.completionTokens) ?? 0;
  const tpsTokens = outputTokens + (safeTokenCount(data.reasoningTokens) ?? 0);
  const cacheHitTokens = safeTokenCount(data.cacheHitTokens);
  const cacheMissTokens = safeTokenCount(data.cacheMissTokens);
  const cacheTokens = (cacheHitTokens ?? 0) + (cacheMissTokens ?? 0);
  const sameSession = previous?.sessionId === event.sessionId;
  const baseline = sameSession ? previous : { sessionId: event.sessionId, tokens: 0, turnTokens: 0, turnOutputTokens: 0, turnTpsTokens: 0, turnCacheTokens: 0, latestCacheHitTokens: null, latestCacheMissTokens: null, turnTps: null, turnModelMs: 0, streamAttemptStartedAt: null, turns: 1, turnCost: 0, turnCurrency: "", turnCostComplete: true, sessionCost: 0, sessionCurrency: "", costComplete: true };
  const nextTokens = baseline.tokens + tokens;
  const nextTurnTokens = baseline.turnTokens + tokens;
  const nextTurnOutputTokens = baseline.turnOutputTokens + outputTokens;
  const nextTurnTpsTokens = baseline.turnTpsTokens + tpsTokens;
  const nextTurnCacheTokens = baseline.turnCacheTokens + cacheTokens;
  if (![nextTokens, nextTurnTokens, nextTurnOutputTokens, nextTurnTpsTokens, nextTurnCacheTokens].every(Number.isSafeInteger)) return previous;
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
  const hasLatestCacheRate = cacheHitTokens !== null && cacheMissTokens !== null && cacheHitTokens + cacheMissTokens > 0;
  return {
    sessionId: event.sessionId,
    tokens: nextTokens,
    turnTokens: nextTurnTokens,
    turnOutputTokens: nextTurnOutputTokens,
    turnTpsTokens: nextTurnTpsTokens,
    turnCacheTokens: nextTurnCacheTokens,
    latestCacheHitTokens: hasLatestCacheRate ? cacheHitTokens : null,
    latestCacheMissTokens: hasLatestCacheRate ? cacheMissTokens : null,
    turnTps: baseline.turnTps,
    turnModelMs: baseline.turnModelMs,
    streamAttemptStartedAt: baseline.streamAttemptStartedAt,
    turns: baseline.turns,
    turnCost: Number.isFinite(nextTurnCost) ? nextTurnCost : baseline.turnCost,
    turnCurrency,
    turnCostComplete: canAggregateTurnCost,
    sessionCost: Number.isFinite(nextSessionCost) ? nextSessionCost : baseline.sessionCost,
    sessionCurrency: costCurrency,
    costComplete: canAggregateCost,
  };
}
