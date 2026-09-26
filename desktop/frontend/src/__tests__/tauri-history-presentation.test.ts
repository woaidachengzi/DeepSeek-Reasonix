import assert from "node:assert/strict";
import { formatTauriWorkDuration, groupTauriHistory } from "../tauri/historyPresentation";

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

assert.equal(formatTauriWorkDuration(960_000), "用时 16 分钟");
assert.equal(formatTauriWorkDuration(1_100), "用时 2 秒");
assert.equal(formatTauriWorkDuration(0), null);
console.log("tauri progress grouping and duration: OK");
