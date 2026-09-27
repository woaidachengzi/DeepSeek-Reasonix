import assert from "node:assert/strict";
import { observeTauriUsage } from "../tauri/tauriObservedUsage";
import type { TauriBridgeEvent } from "../lib/tauriBridge";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { TauriStatusBar } from "../tauri/TauriStatusBar";

function frame(sessionId: string, eventKind: string, payload: Record<string, unknown> = {}): TauriBridgeEvent {
  return { protocolVersion: 1, sequence: 1, sessionId, eventKind, payload };
}

let usage = observeTauriUsage(null, frame("a", "usage", { usage: { totalTokens: 120, cacheHitTokens: 40 } }));
assert.deepEqual(usage, { sessionId: "a", tokens: 120, turnTokens: 120 });
usage = observeTauriUsage(usage, frame("a", "usage", { usage: { totalTokens: 30, cacheHitTokens: 10 } }));
assert.deepEqual(usage, { sessionId: "a", tokens: 150, turnTokens: 150 });
usage = observeTauriUsage(usage, frame("a", "turn_started"));
assert.deepEqual(usage, { sessionId: "a", tokens: 150, turnTokens: 0 });
assert.equal(observeTauriUsage(usage, frame("a", "usage", { usage: { totalTokens: -3, cacheHitTokens: 0 } })), usage, "invalid counts cannot corrupt the display");
assert.deepEqual(observeTauriUsage(usage, frame("b", "usage", { usage: { totalTokens: 8, cacheHitTokens: 0 } })), { sessionId: "b", tokens: 8, turnTokens: 8 }, "a new session starts a fresh observation");
const html = renderToStaticMarkup(createElement(TauriStatusBar, { workspace: "/tmp/project", model: "example-model", sessionState: "running", bridgeRunning: true, observedUsage: usage, sessionMetrics: { contextUsedTokens: 2400, contextWindowTokens: 10000, compactThresholdPercent: 80, cacheHitTokens: 300, cacheMissTokens: 100 } }));
assert.match(html, /已观测 token · 150/);
assert.match(html, /本轮 token · 0/);
assert.match(html, /上下文 · 24%/);
assert.match(html, /压缩阈值 · 80%/);
assert.match(html, /会话缓存命中 · 75%/);
assert.match(html, /当前同步阶段收到的用量事件/, "the value explains why it is not a historical total");
console.log("tauri observed usage: OK");
