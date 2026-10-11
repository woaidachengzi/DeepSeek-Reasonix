// Run: node --import ./scripts/svg-stub-register.mjs --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-session-switch.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
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
  __tauriBridgeCalls: Array<{name: string; args: {sessionId?: string}}>;
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
} finally {await act(async()=>root.unmount());dom.window.close();}
