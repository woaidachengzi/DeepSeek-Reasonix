// Run: node --import tsx src/__tests__/markdown-cold-readiness.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { createServer } from "vite";
const dom = new JSDOM("<!doctype html><div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, Node: dom.window.Node,
  HTMLElement: dom.window.HTMLElement, IS_REACT_ACT_ENVIRONMENT: true,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window) });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
let releaseRenderer!: () => void;
const rendererGate = new Promise<void>(resolve => { releaseRenderer = resolve; });
let rendererRequested = false;
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true },
  plugins: [{ name: "hold-cold-markdown-renderer", enforce: "pre", async load(id) {
    if (id.split("?")[0].endsWith("/MarkdownRenderer.tsx")) { rendererRequested = true; await rendererGate; }
    return null;
  } }] });
const { Markdown } = await server.ssrLoadModule("/src/components/Markdown.tsx");
const { TauriHistorySurface, useTauriHistoryParsed } = await server.ssrLoadModule("/src/tauri/TauriHistorySurface.tsx");
const worker = await server.ssrLoadModule("/src/lib/markdownWorkerClient.ts");
worker.setMarkdownWorkerClientForTest(new worker.MarkdownWorkerClient({ parseInProcess() { throw new Error("cold-worker-failure"); } }));
const text = "# Cold answer\n\n| Name | Result |\n| --- | --- |\n| first | **formatted** |";
let ready = 0;
let formattedAtReveal = false;
function Row() {
  const markParsed = useTauriHistoryParsed();
  return <Markdown text={text} onHistoryReady={React.useCallback(() => markParsed(1, text), [markParsed])} />;
}
const root = createRoot(document.getElementById("root")!);
const until = async (condition: () => boolean) => {
  const deadline = Date.now() + 5000;
  while (!condition()) {
    assert(Date.now() < deadline, "cold renderer state did not settle");
    await act(async () => { await new Promise<void>(resolve => setImmediate(resolve)); });
  }
};
try {
  await act(async () => root.render(<React.StrictMode><TauriHistorySurface loaded entries={[{ index: 1, text }]} onPrepared={() => {
    ready += 1;
    formattedAtReveal = Boolean(document.querySelector("h1") && document.querySelector("table"));
  }}><div className="tauri-conversation"><Row /></div></TauriHistorySurface></React.StrictMode>));
  await until(() => rendererRequested);
  assert.equal(ready, 0, "worker failure must not release the presentation gate");
  assert.equal(document.querySelector('.tauri-history-surface')?.getAttribute('aria-hidden'), 'true');
  assert.equal(document.querySelectorAll('[role=status]').length, 1);
  releaseRenderer();
  await until(() => ready === 1);
  assert(formattedAtReveal, "the real formatted DOM is committed before readiness is reported");
  assert.equal(document.querySelector('.tauri-history-surface')?.hasAttribute('aria-hidden'), false);
  assert.equal(document.querySelector('[data-transcript-geometry-pending]'), null);
  console.log('PASS cold first history: parser failure waits for lazy fallback DOM before revealing');
  worker.disposeMarkdownWorkerClient();
  const { parseMarkdown } = await server.ssrLoadModule("/src/lib/markdownPipeline.ts");
  worker.setMarkdownWorkerClientForTest(new worker.MarkdownWorkerClient({ parseInProcess: parseMarkdown }));
  await act(async () => root.render(<React.StrictMode><TauriHistorySurface key="successful-parser" loaded entries={[{ index: 1, text }]} onPrepared={() => {
    ready += 1;
    formattedAtReveal = Boolean(document.querySelector("h1") && document.querySelector("table"));
  }}><div className="tauri-conversation"><Row /></div></TauriHistorySurface></React.StrictMode>));
  await until(() => ready === 2);
  assert(formattedAtReveal, "successful parsing also reports readiness after formatted DOM commits");
  assert.equal(document.querySelector('[data-transcript-geometry-pending]'), null);
  console.log('PASS successful first history: AST presentation commits before revealing');
} finally {
  releaseRenderer(); await act(async () => root.unmount()); worker.disposeMarkdownWorkerClient(); await server.close(); dom.window.close();
}
