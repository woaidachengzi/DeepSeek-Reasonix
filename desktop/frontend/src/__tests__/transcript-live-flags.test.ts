import assert from "node:assert/strict";
import React, { act, useContext, useMemo } from "react";
import { createRoot } from "react-dom/client";
import { createTranscriptHarness } from "./transcript-dom-harness";
import type { LiveStream, Item } from "../lib/useController";
import type { TranscriptLiveFlags } from "../lib/transcriptRows";

const harness = await createTranscriptHarness({ deterministic: true });
const { useTranscriptLiveFlags } = await harness.loadModule<{
  useTranscriptLiveFlags: (live: LiveStream | undefined) => TranscriptLiveFlags;
}>("/src/lib/useTranscriptLiveFlags.ts");
const host = document.createElement("div");
document.body.append(host);
const root = createRoot(host);
const StreamContext = React.createContext<LiveStream | undefined>(undefined);
let flags: TranscriptLiveFlags, builds = 0;
function LatestText() {
  const live = useContext(StreamContext);
  return React.createElement("output", null, `${live?.text ?? ""}|${live?.reasoning ?? ""}`);
}
function Probe({ live }: { live?: LiveStream }) {
  flags = useTranscriptLiveFlags(live);
  useMemo(() => { builds++; return {}; }, [flags]);
  return React.createElement(StreamContext.Provider, { value: live }, React.createElement(LatestText));
}
const render = async (live?: LiveStream) => {
  await act(async () => root.render(React.createElement(Probe, { live })));
};
try {
  await render();
  const absent = flags!;
  let live: LiveStream = { id: "first", text: "", reasoning: "", reasoningComplete: false };
  await render(live);
  assert.notEqual(flags!, absent, "a new stream owns its own structure");
  const empty = flags!;
  await render({ ...live, text: " \n\t" });
  assert.equal(flags!, empty, "whitespace does not create an answer row");
  live = { ...live, text: "answer 0", reasoning: "proof 0" };
  await render(live);
  const started = flags!, startedBuilds = builds;
  for (let i = 1; i <= 120; i++) {
    live = { ...live, text: `answer ${i}`, reasoning: `proof ${i}` };
    await render(live);
    assert.equal(flags!, started, "text-only deltas keep the structural dependency identical");
    assert.equal(host.textContent, `answer ${i}|proof ${i}`, "descendant context still delivers every new text value");
  }
  assert.equal(builds, startedBuilds, "text growth does not re-run structural work");
  await render({ ...live, reasoningComplete: true });
  assert.notEqual(flags!, started, "reasoning completion invalidates structure");
  const complete = flags!;
  await render({ ...live, id: "second", reasoningComplete: true });
  assert.notEqual(flags!, complete, "new stream identity invalidates structure even with equal presence flags");
  await render({ ...live, reasoning: "", text: " " });
  assert.equal(flags!.hasAnswerText, false);
  assert.equal(flags!.hasReasoning, false);
  await render();
  assert.equal(flags!, absent, "discard/settlement restores the absent-stream shape");
} finally {
  await act(async () => root.unmount());
  host.remove();
}

// Exercise the actual Transcript consumers, not just a structural hook probe.
const items: Item[] = [
  { kind: "user", id: "stream-user", text: "latest live text" },
  { kind: "assistant", id: "stream-answer", text: "", reasoning: "", streaming: true },
];
try {
  await harness.render(items, { running: true, live: { id: "stream-answer", text: "first answer", reasoning: "", reasoningComplete: false } });
  await harness.settle();
  const block = harness.container.querySelector('[data-transcript-block-phase="active"]');
  assert(block, "the active semantic turn is mounted");
  await harness.render(items, { running: true, live: { id: "stream-answer", text: "updated answer ✅", reasoning: "", reasoningComplete: false } });
  await harness.settle();
  assert(harness.container.textContent?.includes("updated answer ✅"), "real message presentation receives current stream text");
  assert.equal(harness.container.querySelector('[data-transcript-block-phase="active"]'), block, "stream growth retains the native active block");
} finally {
  await harness.unmount();
  await act(async () => { await harness.close(); });
}
console.log("PASS stable live structural flags, raw context growth, completion and stream identity");
