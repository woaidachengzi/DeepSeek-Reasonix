// Run: node --import ./scripts/svg-stub-register.mjs --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-session-switch.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true, requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window), cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window) });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
dom.window.HTMLElement.prototype.scrollTo = function (options: ScrollToOptions | number, y?: number) { this.scrollTop = typeof options === "number" ? y ?? 0 : options.top ?? this.scrollTop; };
(globalThis as typeof globalThis & { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = {
  invoke: async (command: string) => command === "workspace_external_openers" ? [] : null,
  metadata: { currentWindow: { label: "main" }, currentWebview: { label: "main" } },
  transformCallback: () => 1,
};
Object.assign(dom.window, {
  __TAURI_INTERNALS__: (globalThis as typeof globalThis & { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__,
  __TAURI_EVENT_PLUGIN_INTERNALS__: { unregisterListener: () => {} },
});
const state = globalThis as typeof globalThis & {
  isTauri: boolean;
  __workbenchSessions: Array<{sessionId: string; title: string; workspaceRoot: string}>;
  __tauriHistoryMessages: Array<{role: string; content: string}>;
  __tauriBridgeCalls: Array<{name: string; args: {sessionId?: string; beforeIndex?: number}}>;
  __historyBySession?: Record<string, Array<{role: string; content: string}>>;
  __historyPageSize?: number;
  __olderHistoryGate?: Promise<void>;
  __rememberSessionGate?: Promise<void>;
  __replayGate?: Promise<void>;
  __historyError?: string;
};
state.isTauri = true;
state.__workbenchSessions = [{sessionId:"history-a",title:"History A",workspaceRoot:"/tmp/ws"},{sessionId:"history-b",title:"History B",workspaceRoot:"/tmp/ws"}];
state.__tauriHistoryMessages = [{role:"assistant",content:"Visible history before maintenance completes"}];
const React = await import("react");
const {act} = React;
const {createRoot} = await import("react-dom/client");
const {TauriSessionApp} = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const tick = () => act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 0)); });
async function until(condition: () => boolean) { for (let i=0;i<100;i++) { if(condition()) return; await tick(); } assert.ok(condition(), "UI state did not settle: "+document.body.textContent); }
function button(title: string) { const row=[...document.querySelectorAll(".tauri-session-row")].find(row=>row.textContent?.includes(title)); assert.ok(row, title); return row.querySelector<HTMLButtonElement>(".tauri-sidebar__session")!; }
const calls = (name:string,id?:string) => state.__tauriBridgeCalls.filter(call=>call.name===name && (!id || call.args?.sessionId===id)).length;
const loadEarlier = () => document.querySelector<HTMLButtonElement>(".tauri-history-note button");
const questionRail = () => document.querySelector<HTMLDivElement>(".jump-scroll");
const question = (index: number) => document.getElementById(`tauri-question-${index}`);
try {
  await act(async () => {root.render(React.createElement(TauriSessionApp));});
  await until(()=>document.querySelectorAll(".tauri-session-row").length===2);
  const auditBefore=calls("workbench_session_page");
  let rejectMaintenance!: (reason: Error)=>void;
  let resumeReplay!: ()=>void;
  state.__rememberSessionGate=new Promise<void>((_,reject)=>{rejectMaintenance=reject;});
  state.__replayGate=new Promise<void>(resolve=>{resumeReplay=resolve;});
  await act(async()=>{button("History A").click();});
  await until(()=>calls("bridge_replay_pending_prompts","history-a")===1 && calls("bridge_session_history","history-a")===1);
  await until(()=>document.querySelector(".tauri-transcript")?.textContent?.includes("Visible history before maintenance completes")===true);
  assert.equal(button("History B").disabled,false,"pending catalog persistence must not block navigation");
  assert.equal(calls("workbench_session_page"),auditBefore,"ordinary reopen performs no full directory scan");
  assert.equal(calls("bridge_start_events"),1,"surface activation starts the stream once");
  delete state.__rememberSessionGate; delete state.__replayGate;
  await act(async()=>{button("History B").click();});
  await until(()=>calls("bridge_session_history","history-b")===1 && calls("bridge_replay_pending_prompts","history-b")===1);
  await act(async()=>{rejectMaintenance(new Error("late-maintenance-error"));resumeReplay();});
  await tick(); await tick();
  assert.equal(calls("bridge_start_events"),2,"two surfaces start exactly two streams, with no maintenance restart");
  assert.equal(calls("bridge_session_history","history-a"),1);
  assert.equal(calls("bridge_session_history","history-b"),1);
  assert.ok(!document.body.textContent?.includes("late-maintenance-error"),"old maintenance errors cannot overwrite the new surface");
  const before=calls("bridge_switch_session");
  await act(async()=>{button("History B").click();});await tick();
  assert.equal(calls("bridge_switch_session"),before,"clicking the ready active session is a no-op");
  state.__historyError="模拟历史加载失败";
  await act(async()=>button("History A").click());
  await until(()=>document.body.textContent?.includes("模拟历史加载失败")===true);
  await until(()=>!button("History A").disabled);
  delete state.__historyError;
  const failedReads=calls("bridge_session_history","history-a");
  await act(async()=>button("History A").click());
  await until(()=>calls("bridge_session_history","history-a")===failedReads+1 && document.querySelector(".tauri-transcript")?.textContent?.includes("Visible history before maintenance completes")===true);
  assert.ok(!document.body.textContent?.includes("模拟历史加载失败"),"clicking a failed active session retries history");
  console.log("PASS history visible before blocked prompt replay/catalog maintenance; no reopen audit; one stream/history per switch; stale errors fenced; active row no-op");

  state.__historyPageSize = 4;
  state.__historyBySession = {
    "history-a": Array.from({ length: 10 }, (_, index) => ({
      role: index % 2 === 0 ? "user" : "assistant",
      content: `Paged history A row ${index}`,
    })),
    "history-b": [{ role: "user", content: "History B only" }],
  };
  await act(async () => button("History B").click());
  await until(() => document.querySelector(".tauri-transcript")?.textContent?.includes("History B only") === true);
  await act(async () => button("History A").click());
  await until(() => loadEarlier()?.disabled === false && question(6) !== null);
  assert.match(document.querySelector(".tauri-history-note")!.textContent!, /Showing 4 of 10 messages|已加载 4 条，共 10 条可见消息/);
  assert.equal(questionRail()?.getAttribute("aria-valuemax"), "2", "the rail starts with the loaded questions");
  assert.equal(question(0), null, "opening reads the latest page first");

  let resolveOlder!: () => void;
  state.__olderHistoryGate = new Promise<void>(resolve => { resolveOlder = resolve; });
  await act(async () => loadEarlier()!.click());
  assert.equal(loadEarlier()?.disabled, true, "a pending history load disables repeat clicks");
  const olderReads = () => state.__tauriBridgeCalls
    .filter(call => call.name === "bridge_session_history" && call.args.sessionId === "history-a" && call.args.beforeIndex !== undefined)
    .map(call => call.args.beforeIndex);
  assert.deepEqual(olderReads(), [6], "request preceding rows using the first visible index as exclusive cursor");
  await act(async () => button("History B").click());
  await until(() => document.querySelector(".tauri-transcript")?.textContent?.includes("History B only") === true);
  delete state.__olderHistoryGate;
  await act(async () => resolveOlder());
  assert.equal(loadEarlier(), null, "an older response for A must not create pagination controls in B");
  assert.ok(!document.querySelector(".tauri-transcript")?.textContent?.includes("Paged history A"), "a late page must not leak into the new session");

  await act(async () => button("History A").click());
  await until(() => loadEarlier()?.disabled === false && question(6) !== null);
  const firstLoadedQuestion = question(6)!;
  // jsdom has no layout. Give the existing reader anchor a position that
  // changes with the actual rendered prefix, so scroll compensation is tested.
  const scroller = document.querySelector<HTMLElement>(".tauri-conversation")!;
  const rows = () => Array.from(scroller.querySelectorAll(".tauri-transcript > [data-transcript-block-key]"));
  const noteHeight = () => loadEarlier() ? 40 : 0;
  Object.defineProperties(scroller, {
    clientHeight: { value: 100 },
    scrollHeight: { get: () => rows().length * 100 + noteHeight() },
  });
  scroller.getBoundingClientRect = () => new dom.window.DOMRect(0, 0, 500, 100);
  firstLoadedQuestion.getBoundingClientRect = () =>
    new dom.window.DOMRect(0, rows().indexOf(firstLoadedQuestion) * 100 + noteHeight() - scroller.scrollTop, 500, 100);
  await act(async () => {
    questionRail()!.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Home", bubbles: true }));
  });
  assert.equal(scroller.scrollTop, 40, "jump to the first loaded question puts the viewport in reader mode");
  await act(async () => loadEarlier()!.click());
  await until(() => loadEarlier()?.disabled === false && question(2) !== null);
  assert.equal(question(6), firstLoadedQuestion, "prepending preserves the existing question DOM identity");
  assert.equal(firstLoadedQuestion.getBoundingClientRect().top, 0, "adding the prefix preserves the question's viewport position");
  assert.ok(question(2)!.compareDocumentPosition(firstLoadedQuestion) & dom.window.Node.DOCUMENT_POSITION_FOLLOWING, "earlier questions appear ahead of the latest page");
  assert.equal(questionRail()?.getAttribute("aria-valuemax"), "4", "earlier questions become navigable");
  assert.match(document.querySelector(".tauri-history-note")!.textContent!, /Showing 8 of 10 messages|已加载 8 条，共 10 条可见消息/);
  await act(async () => loadEarlier()!.click());
  await until(() => loadEarlier() === null && question(0) !== null);
  assert.deepEqual(olderReads(), [6, 6, 2], "each request uses the current first row as its cursor");
  assert.equal(questionRail()?.getAttribute("aria-valuemax"), "5");
  assert.equal(question(6), firstLoadedQuestion);
  assert.equal(firstLoadedQuestion.getBoundingClientRect().top, 0, "removing the exhausted pager preserves the same viewport anchor");
  console.log("PASS history loads earlier pages on demand, expands the question rail, and ignores a late page after session switch");
} finally {await act(async()=>root.unmount());dom.window.close();}
