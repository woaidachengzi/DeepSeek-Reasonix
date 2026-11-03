// Isolated real row-model compute under React. No renderer/DOM layout, IPC,
// Markdown parse, network, or model latency is measured here.
import React, { act, useMemo } from "react";
import { JSDOM } from "jsdom";
import { createRoot } from "react-dom/client";
import { performance } from "node:perf_hooks";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { useTranscriptLiveFlags } from "../src/lib/useTranscriptLiveFlags";
import { buildTurnModels, buildTranscriptRowBlocks, EMPTY_FOLDS, NO_LIVE, type TranscriptLiveFlags } from "../src/lib/transcriptRows";
import type { Item, LiveStream } from "../src/lib/useController";
const dom = new JSDOM('<div id="root"></div>', { url: "http://localhost" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true });

// Exact 0.1.77 dependency strategy; kept independent of the candidate hook.
function useLegacyFlags(live: LiveStream | undefined): TranscriptLiveFlags {
  return useMemo(() => live?.id ? {
    id: live.id, hasAnswerText: Boolean(live.text.trim()), hasReasoning: Boolean(live.reasoning), reasoningComplete: live.reasoningComplete,
  } : NO_LIVE, [live?.id, live?.reasoning, live?.reasoningComplete, live?.text]);
}
const updates = 80;
const cases = [{ turns: 200, body: 64 }, { turns: 1000, body: 64 }, { turns: 200, body: 512 * 1024 }];
async function sample(route: "baseline" | "candidate", turns: number, body: number) {
  const items: Item[] = [];
  for (let turn = 0; turn < turns; turn++) {
    items.push({ kind: "user", id: `u-${turn}`, text: `question ${turn}`, historyTurn: turn + 1 });
    items.push({ kind: "assistant", id: `a-${turn}`, text: turn === turns - 1 ? "" : "synthetic ".repeat(Math.ceil((turn === turns - 2 ? body : 64) / 10)), reasoning: "", streaming: turn === turns - 1 });
  }
  let builds = 0, rows = 0;
  const useFlags = route === "baseline" ? useLegacyFlags : useTranscriptLiveFlags;
  function Probe({ live }: { live: LiveStream }) {
    const flags = useFlags(live);
    const models = useMemo(() => { builds++; return buildTurnModels(items, flags, true, false); }, [flags]);
    const blocks = useMemo(() => buildTranscriptRowBlocks(models, {
      folds: EMPTY_FOLDS, sessionExperience: "standard", hasOlderHistory: false, creationMode: false,
      turnForUser: item => (item.historyTurn ?? 1) - 1,
    }), [models]);
    rows = blocks.reduce((count, block) => count + block.rows.length, 0);
    return null;
  }
  const root = createRoot(document.getElementById("root")!);
  const initial: LiveStream = { id: `a-${turns - 1}`, text: "answer", reasoning: "proof", reasoningComplete: false };
  await act(async () => root.render(React.createElement(Probe, { live: initial })));
  builds = 0;
  const started = performance.now();
  for (let i = 0; i < updates; i++) await act(async () => root.render(React.createElement(Probe, { live: { ...initial, text: `answer ${i}`, reasoning: `proof ${i}` } })));
  const ms = performance.now() - started;
  await act(async () => root.unmount());
  if (builds !== (route === "baseline" ? updates : 0)) throw Error("unexpected structural work count");
  return { route, turns, body, updates, ms, builds, rows };
}
const samples = [];
for (let round = -1; round < 5; round++) for (const route of (round % 2 === 0 ? ["baseline", "candidate"] : ["candidate", "baseline"]) as Array<"baseline" | "candidate">) {
  for (const fixture of cases) {
    const value = await sample(route, fixture.turns, fixture.body);
    if (round >= 0) samples.push({ round, ...value });
  }
}
const medians = cases.map(fixture => ({ ...fixture, ...Object.fromEntries(["baseline", "candidate"].map(route => {
  const rows = samples.filter(s => s.route === route && s.turns === fixture.turns && s.body === fixture.body);
  const times = rows.map(s => s.ms).sort((a, b) => a - b);
  return [route, { ms: times[2], builds: rows[0].builds, rows: rows[0].rows }];
})) }));
const sources = ["../src/lib/useTranscriptLiveFlags.ts", "../src/lib/transcriptRows.ts", "../src/components/Transcript.tsx", "./live-model.ts"];
console.log(JSON.stringify({ scope: "React/jsdom isolated row-model recomputation across80 text/reasoning-only updates; setup and initial model excluded, warm-up discarded, five alternating rounds; not WebView, layout, Markdown parsing or model latency", baselineCommit: "6fbbeb2c87434d2e1b33cc29ca6e7959faf87e11", candidateCommit: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(), candidateStatus: execFileSync("git", ["status", "--short"], { encoding: "utf8" }).trim(), node: process.version,
 sourceSha256: Object.fromEntries(sources.map(name => [name, createHash("sha256").update(readFileSync(new URL(name, import.meta.url))).digest("hex")])), samples, medians }, null, 2));
