// Synthetic loaded-name filter audit; excludes disk, full text and rendering.
import { performance } from "node:perf_hooks";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { filterWorkbenchProjectGroups, groupWorkbenchSessions } from "../src/tauri/workbenchSessions";
const queries = ["s", "se", "ses", "sessi", "session", "session 7", "SESSION 7", "ＰＲＯＪＥＣＴ", "中文", "absent"];
const results = [];
for (const count of [200, 1000, 10000]) {
  const groups = groupWorkbenchSessions(Array.from({ length: count }, (_, i) => ({
    sessionId: String(i), title: `中文 session ${i} ﬁxture`, workspaceRoot: `/synthetic/project-${Math.floor(i / 100)}`,
  })));
  const times = [];
  for (let round = -2; round < 9; round++) {
    const start = performance.now();
    for (const query of queries) filterWorkbenchProjectGroups(groups, query);
    if (round >= 0) times.push((performance.now() - start) / queries.length);
  }
  const sorted = [...times].sort((a, b) => a - b);
  results.push({ count, queries: queries.length, msPerQueryMedian: sorted[4], msPerQueryMax: sorted.at(-1), samples: times });
}
console.log(JSON.stringify({
  scope: "Node synthetic existing Tauri name filter only; loaded groups, no render, no full-text or disk",
  queries, sourceSha256: createHash("sha256").update(readFileSync(new URL("../src/tauri/workbenchSessions.ts", import.meta.url))).digest("hex"),
  node: process.version, results,
}, null, 2));
