// Run: node --import tsx src/__tests__/tauri-history-surface.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document,
  HTMLElement: dom.window.HTMLElement, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const React = await import("react");
const { act, useCallback } = React;
const { createRoot } = await import("react-dom/client");
const { TauriHistorySurface, useTauriHistoryParsed } = await import("../tauri/TauriHistorySurface");
const readyCallbacks = new Map<string, () => void>();
const prepared: string[] = [];
function Row({ id, index, text }: { id: string; index: number; text: string }) {
  const parsed = useTauriHistoryParsed();
  readyCallbacks.set(id, useCallback(() => parsed(index, text), [parsed, index, text]));
  return <article data-transcript-block-key={`message-${index}`}>{text}</article>;
}
const root = createRoot(document.getElementById("root")!);
const render = async (session: string, entries: Array<{index: number; text: string}>) => act(async () => {
  root.render(<TauriHistorySurface key={session} loaded entries={entries} onPrepared={() => prepared.push(session)}>
    <div className="tauri-conversation">{entries.map(entry => <Row key={entry.index} id={`${session}-${entry.index}`} {...entry} />)}</div>
  </TauriHistorySurface>);
});
const hidden = () => document.querySelector('.tauri-history-surface')?.getAttribute('aria-hidden') === 'true';
try {
  await render('a', [{index:1,text:'# Raw heading A'}, {index:2,text:'| pending table |'}]);
  assert(hidden(), 'unformatted history is concealed without removing measurable DOM');
  assert.equal(document.querySelectorAll('[role=status]').length, 1, 'one indicator for the conversation');
  assert.equal(document.querySelectorAll('[data-transcript-block-key]').length, 2);
  await act(async () => readyCallbacks.get('a-1')!());
  assert(hidden(), 'one completed row cannot expose a partially formatted conversation');
  const staleReady = readyCallbacks.get('a-2')!;
  await render('b', [{index:1,text:'# Raw heading B'}]);
  await act(async () => staleReady());
  assert(hidden(), 'late A callback cannot reveal B');
  assert.deepEqual(prepared, [], 'old callbacks cannot commit another viewport');
  const staleB = readyCallbacks.get('b-1')!;
  await render('b', [{index:1,text:'# Updated B'}]);
  await act(async () => staleB());
  assert(hidden(), 'an older source revision cannot mark the new text ready');
  const currentReady = readyCallbacks.get('b-1')!;
  await act(async () => currentReady());
  assert(!hidden(), 'complete formatting reveals the existing DOM');
  assert.deepEqual(prepared, ['b'], 'viewport preparation runs once before reveal');
  assert.equal(document.querySelector('[role=status]'), null);
  await render('b', [{index:1,text:'# Updated B'}, {index:2,text:'new streamed answer'}]);
  assert(!hidden(), 'later streamed turns never re-hide the surface');
  await act(async () => readyCallbacks.get('b-2')!());
  assert.deepEqual(prepared, ['b']);
  await render('empty', []);
  assert(!hidden(), 'empty histories do not wait for non-existent Markdown');
  console.log('PASS atomic history reveal, measurable DOM, source identity, stale generation fencing and live updates');
} finally { await act(async () => root.unmount()); dom.window.close(); }
