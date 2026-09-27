import type { TauriBridgeEvent } from "../lib/tauriBridge";

export interface TauriObservedUsage {
  sessionId: string;
  tokens: number;
  turnTokens: number;
  cacheHitTokens: number;
}

function safeTokenCount(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

/** Count only usage frames actually received in this synchronized stream. */
export function observeTauriUsage(previous: TauriObservedUsage | null, event: TauriBridgeEvent): TauriObservedUsage | null {
  if (event.eventKind === "turn_started") {
    return previous?.sessionId === event.sessionId ? { ...previous, turnTokens: 0 } : null;
  }
  if (event.eventKind !== "usage") return previous;
  const usage = event.payload.usage;
  if (!usage || typeof usage !== "object" || Array.isArray(usage)) return previous;
  const data = usage as Record<string, unknown>;
  const tokens = safeTokenCount(data.totalTokens);
  const cacheHit = safeTokenCount(data.cacheHitTokens);
  if (tokens === null || cacheHit === null) return previous;
  const baseline = previous?.sessionId === event.sessionId ? previous : { sessionId: event.sessionId, tokens: 0, turnTokens: 0, cacheHitTokens: 0 };
  const nextTokens = baseline.tokens + tokens;
  const nextTurnTokens = baseline.turnTokens + tokens;
  const nextCacheHit = baseline.cacheHitTokens + cacheHit;
  if (![nextTokens, nextTurnTokens, nextCacheHit].every(Number.isSafeInteger)) return previous;
  return { sessionId: event.sessionId, tokens: nextTokens, turnTokens: nextTurnTokens, cacheHitTokens: nextCacheHit };
}
