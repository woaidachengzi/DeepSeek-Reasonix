import React, { act, type ComponentType, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { createTranscriptHarness } from "./transcript-dom-harness";
import type { TimelineBlock } from "../lib/transcriptTimeline";

// Geometry placement cannot rebuild unchanged semantic content or lose input.
async function verifyContentPlacementIsolation(): Promise<number> {
  const harness = await createTranscriptHarness({ deterministic: true });
  const { TranscriptBlockView } = await harness.loadModule<{
    TranscriptBlockView: ComponentType<{ block: TimelineBlock; tabId: string;
      renderRow: (row: unknown) => ReactNode; placement: { index: number; top: number } }>;
  }>("/src/components/TranscriptBlockView.tsx");
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  let failed = 0, renders = 0, clicked = "";
  const check = (value: boolean, label: string) => {
    console.log(`${value ? "PASS" : "FAIL"} ${label}`);
    if (!value) failed++;
  };
  const initial: TimelineBlock = { key: "turn:held", phase: "completed", contentRevision: 1, measurementRevision: "1",
    rows: [{ kind: "user", key: "u:held", turn: 0, layoutVariant: "text-flow",
      item: { kind: "user", id: "held", text: "held question" } }] };
  const renderer = (label: string) => () => {
    renders++;
    return React.createElement(React.Fragment, null,
      React.createElement("input", { defaultValue: "draft stays here" }),
      React.createElement("button", { onClick: () => { clicked = label; } }, label));
  };
  let renderRow = renderer("first");
  const render = async (block: TimelineBlock, top: number, index = 0, tabId = "first") => {
    await act(async () => root.render(React.createElement(TranscriptBlockView, { key: block.key, block, placement: { index, top }, tabId, renderRow })));
  };
  try {
    await render(initial, 100);
    const blockHost = host.firstElementChild, rowHost = host.querySelector(".transcript__row");
    const input = host.querySelector("input")!;
    input.focus(); input.setSelectionRange(2, 5);
    const firstRenders = renders;
    for (const [index, top] of [140, 180, 100, 220].entries()) await render(initial, top, index);
    check(renders === firstRenders, "placement commits do not re-render unchanged message content");
    check(host.firstElementChild === blockHost && host.querySelector(".transcript__row") === rowHost,
      "range commits retain native block and row hosts");
    check((blockHost as HTMLElement).style.top === "220px" && (blockHost as HTMLElement).dataset.index === "3",
      "the host receives every committed geometry position");
    check(document.activeElement === input && input.selectionStart === 2 && input.selectionEnd === 5,
      "placement commits preserve input focus and native selection endpoints");
    const patched: TimelineBlock = { ...initial, contentRevision: 2, measurementRevision: "2",
      rows: [{ ...initial.rows[0], item: { kind: "user", id: "held", text: "patched question" } } as TimelineBlock["rows"][number]] };
    await render(patched, 240);
    check(renders === firstRenders + 1 && host.querySelector("input") === input,
      "content patches re-render through the same keyed native row");
    renderRow = renderer("replacement");
    await render(patched, 260);
    await act(async () => host.querySelector("button")!.click());
    check(clicked === "replacement", "changed renderer installs current actions without stale callbacks");
    const beforeTab = renders;
    await render(patched, 260, 0, "second");
    check(renders === beforeTab + 1, "tab ownership changes invalidate the content boundary");
    await render({ ...patched, key: "turn:replacement" }, 280);
    check(host.firstElementChild !== blockHost && host.querySelector("input") !== input,
      "replacement block identity does not reuse the old native editor");
  } finally {
    await act(async () => root.unmount());
    host.remove();
    await harness.unmount();
    await harness.close();
  }
  return failed;
}
if (await verifyContentPlacementIsolation()) process.exit(1);
