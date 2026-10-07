import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { TranscriptTestClock } from "./transcript-test-clock";
import { TranscriptKernelClockContext, useTranscriptKernel } from "../lib/useTranscriptKernel";

const dom = new JSDOM('<div id="root"></div>', { url: "http://localhost" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, IS_REACT_ACT_ENVIRONMENT: true });
const clock = new TranscriptTestClock();
let height = 2400;
let physicalTop = 0;
let writes = 0;
const proto = dom.window.HTMLElement.prototype;
Object.defineProperties(proto, {
  clientHeight: { configurable: true, get: () => 400 },
  scrollHeight: { configurable: true, get: () => height },
  scrollTop: { configurable: true, get: () => physicalTop, set: value => { physicalTop = Number(value); writes++; } },
});
proto.getBoundingClientRect = function () {
  const top = this.hasAttribute("data-transcript-block-key") ? 100 - physicalTop : 0;
  return { top, bottom: top + 400, height: 400, left: 0, right: 800, width: 800, x: 0, y: top, toJSON: () => ({}) };
};
let viewport: ReturnType<typeof useTranscriptKernel>;
function Surface({ session, revision }: { session: string; revision: number }) {
  viewport = useTranscriptKernel({ sessionKey: session, geometryRevision: `${session}:${revision}`, initialPosition: "tail" });
  return <div ref={viewport.setScroller} onScroll={viewport.onScroll} onWheelCapture={viewport.onWheelCapture}>
    <article data-transcript-block-key="question-42">最近的问题和回答</article>
  </div>;
}
const root = createRoot(document.getElementById("root")!);
const render = async (session: string, revision: number) => {
  await act(async () => { root.render(<TranscriptKernelClockContext.Provider value={clock}><Surface session={session} revision={revision} /></TranscriptKernelClockContext.Provider>); });
};
const flush = async () => { await act(async () => { clock.flushFrames(); }); };
const nativeScroll = async (top: number) => {
  await act(async () => {
    viewport!.scrollElement!.dispatchEvent(new dom.window.WheelEvent("wheel", { bubbles: true, deltaY: -300 }));
    physicalTop = top;
    viewport!.scrollElement!.dispatchEvent(new dom.window.Event("scroll", { bubbles: true }));
  });
};
await render("first", 0);
await flush();
assert.equal(physicalTop, 2000, "opening history starts at the latest response");
await act(async () => { viewport!.scrollElement!.dispatchEvent(new dom.window.Event("scroll", { bubbles: true })); });
await nativeScroll(700);
const readerWrites = writes;
height = 3000;
await render("first", 1);
await flush();
assert.equal(writes, readerWrites, "stream growth performs zero writes during reader input");
await act(async () => { clock.advance(320); });
await render("first", 2);
await flush();
assert.equal(writes, readerWrites, "stream growth preserves reader position after the lease settles");
assert.equal(physicalTop, 700);
await act(async () => { viewport!.scrollToBottom(); });
assert.equal(physicalTop, 2600, "sending a new question explicitly resumes tail following");
height = 3500;
await render("first", 3);
await flush();
assert.equal(physicalTop, 3100, "subsequent answer growth follows the physical bottom");
await act(async () => { viewport!.scrollElement!.dispatchEvent(new dom.window.Event("scroll", { bubbles: true })); });
await nativeScroll(3100);
height = 3800;
const leasedTailWrites = writes;
await render("first", 4);
await flush();
assert.equal(writes, leasedTailWrites, "tail growth waits for native input to release");
await act(async () => { clock.advance(320); });
await flush();
assert.equal(physicalTop, 3400, "returning to bottom reconciles growth received before native scrolling settles");
height = 3500;
await act(async () => { viewport!.scrollElement!.dispatchEvent(new dom.window.Event("scroll", { bubbles: true })); });
await nativeScroll(500);
await act(async () => { clock.advance(320); });
await render("second", 0);
await flush();
await render("first", 5);
await flush();
assert.equal(physicalTop, 3100, "reopening a session shows latest content rather than its remembered reader position");
height = 3800;
await render("first", 6);
const staleCallbacks = [...clock.frames.values()];
await render("second", 1);
await flush();
const switchedWrites = writes;
await act(async () => { staleCallbacks.forEach(callback => callback(clock.time)); });
assert.equal(writes, switchedWrites, "late frames from the replaced session cannot scroll the new session");
await act(async () => { root.unmount(); });
assert.equal(clock.frames.size, 0);
dom.window.close();
console.log("Tauri transcript viewport latest/reader/stream/switch races: OK");
