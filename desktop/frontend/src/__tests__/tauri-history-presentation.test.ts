import assert from "node:assert/strict";
import { formatTauriMessageClock, formatTauriWorkDuration, groupTauriHistory } from "../tauri/historyPresentation";

const messages = [
  { role: "user" as const, content: "检查提交" },
  { role: "assistant" as const, content: "我先查看工作区", workDurationMs: 5_000 },
  { role: "assistant" as const, content: "发现两个功能提交", workDurationMs: 70_000 },
  { role: "assistant" as const, content: "结论：需要测试", workDurationMs: 960_000 },
  { role: "user" as const, content: "继续" },
  { role: "assistant" as const, content: "已完成" },
];

const grouped = groupTauriHistory(messages, 20, false);
assert.deepEqual(grouped.map(item => item.kind), ["message", "progress", "message", "message", "message"]);
assert.equal(grouped[1].kind, "progress");
if (grouped[1].kind === "progress") {
  assert.deepEqual(grouped[1].entries.map(entry => entry.index), [21, 22]);
  assert.equal(grouped[1].durationMs, 960_000);
}
assert.equal(grouped[2].kind, "message");
if (grouped[2].kind === "message") assert.equal(grouped[2].entry.message.content, "结论：需要测试");
assert.equal(grouped[4].kind, "message", "a single assistant answer remains visible");

const active = groupTauriHistory(messages.slice(0, 3), 0, true);
assert.deepEqual(active.map(item => item.kind), ["message", "progress"]);
if (active[1].kind === "progress") {
  assert.equal(active[1].active, true);
  assert.equal(active[1].entries.length, 2);
}

assert.equal(formatTauriWorkDuration(960_000), "用时 16分0秒");
assert.equal(formatTauriWorkDuration(1_100), "用时 1秒");
assert.equal(formatTauriWorkDuration(47_900), "用时 47秒");
assert.equal(formatTauriWorkDuration(65_999), "用时 1分5秒");
assert.equal(formatTauriWorkDuration(3_723_000), "用时 1小时2分3秒");
assert.equal(formatTauriWorkDuration(0), null);
assert.equal(formatTauriWorkDuration(Number.NaN), null);
const simple = groupTauriHistory([{ role: "assistant", content: "答案", workDurationMs: 47_000 }], 12, false);
assert.deepEqual(simple.map(item => item.kind), ["progress", "message"], "a timed simple reply has a header without hiding the answer");
assert.ok(simple[0].kind === "progress" && simple[0].entries.length === 0 && simple[0].anchorIndex === 12);
const later = groupTauriHistory([{ role: "assistant", content: "另一个答案", workDurationMs: 47_000 }], 19, false);
assert.ok(later[0].kind === "progress" && later[0].anchorIndex === 19, "equal durations cannot collide in DOM identity");
const now = new Date(2026, 9, 5, 9, 0).getTime();
assert.equal(formatTauriMessageClock(new Date(2026, 9, 5, 8, 2).getTime(), now), "08:02");
assert.equal(formatTauriMessageClock(new Date(2026, 9, 4, 22, 21).getTime(), now), "10月4日 22:21");
assert.equal(formatTauriMessageClock(new Date(2025, 9, 4, 22, 21).getTime(), now), "2025年10月4日 22:21");
assert.equal(formatTauriMessageClock(new Date(2026, 9, 4, 22, 21).getTime(), now, "en"), "10/4 22:21");
for (const invalid of [undefined, 0, -1, Number.NaN, Number.MAX_SAFE_INTEGER]) assert.equal(formatTauriMessageClock(invalid, now), null);
console.log("tauri progress grouping and duration: OK");
