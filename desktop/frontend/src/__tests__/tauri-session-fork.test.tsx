// Run: node --import ./scripts/svg-stub-register.mjs --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-session-fork.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(dom.window.navigator, "language", { value: "zh-CN", configurable: true });
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
  __tauriHistoryMessages: Array<{role: string; content: string; forkToken?: string}>;
  __tauriBridgeCalls: Array<{name: string; args: {sessionId?: string; forkToken?: string}}>;
  __historyBySession?: Record<string, Array<{role: string; content: string; forkToken?: string}>>;
  __workbenchPages?: unknown[];
  __rememberSessionGate?: Promise<void>;
  __replayGate?: Promise<void>;
  __forkSessionHandler?: (id: string) => Promise<{ok: boolean; sessionId?: string}>;
};
state.isTauri = true;
state.__workbenchSessions = [{sessionId:"history-a",title:"History A",workspaceRoot:"/tmp/ws"},{sessionId:"history-b",title:"History B",workspaceRoot:"/tmp/ws"}];
state.__tauriHistoryMessages = [{role:"user",content:"First question"},{role:"assistant",content:"First answer",forkToken:"a".repeat(64)},{role:"user",content:"Later question"},{role:"assistant",content:"Later answer",forkToken:"b".repeat(64)}];
const React = await import("react");
const {act} = React;
const {createRoot} = await import("react-dom/client");
const {TauriSessionApp} = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const tick = () => act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 0)); });
async function until(condition: () => boolean) { for (let i=0;i<100;i++) { if(condition()) return; await tick(); } assert.ok(condition(), "UI state did not settle: " + document.body.textContent); }
function button(title: string) { const row=[...document.querySelectorAll(".tauri-session-row")].find(row=>row.querySelector(".tauri-sidebar__session span")?.textContent===title); assert.ok(row, title); return row.querySelector<HTMLButtonElement>(".tauri-sidebar__session")!; }
const calls = (name:string,id?:string) => state.__tauriBridgeCalls.filter(call=>call.name===name && (!id || call.args?.sessionId===id)).length;
const forkButton = () => document.querySelector<HTMLButtonElement>('[aria-label="分叉对话"]')!;
async function draft(text: string) {
 await act(async()=>{
  const textarea=document.querySelector<HTMLTextAreaElement>(".tauri-composer textarea")!;
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype,"value")!.set!.call(textarea,text);
  textarea.dispatchEvent(new dom.window.Event("input",{bubbles:true}));
 });
}
try {
 await act(async()=>root.render(React.createElement(TauriSessionApp)));
 await until(()=>document.querySelectorAll(".tauri-session-row").length===2);
 assert.equal(forkButton(),null,"new chat has no answer to fork");
 await act(async()=>button("History A").click());
 await until(()=>forkButton() && !forkButton().disabled);
 assert.equal(document.querySelectorAll(".tauri-answer-actions__fork").length,2,"each completed answer owns its fork action");
 assert.ok(forkButton().closest(".tauri-answer-actions"));
 assert.match(forkButton().title,/截至这里/);
 await draft("未发送内容");assert.equal(forkButton().disabled,true,"a fork must not discard an unsent draft");
 await draft("");assert.equal(forkButton().disabled,false);
 let rejectFork!: (error: Error)=>void;
 state.__forkSessionHandler=()=>new Promise((_,reject)=>{rejectFork=reject;});
 await act(async()=>{forkButton().click();forkButton().click();});
 assert.equal(calls("bridge_fork_session","history-a"),1,"rapid repeated clicks create one request");
 assert.equal(state.__tauriBridgeCalls.find(call=>call.name==="bridge_fork_session")?.args.forkToken,"a".repeat(64),"earlier reply sends its exact boundary token");
 assert.equal(forkButton().disabled,true);
 await act(async()=>rejectFork(new Error("模拟分叉失败")));
 await until(()=>!forkButton().disabled);
 assert.match(document.body.textContent??"",/无法分叉对话.*模拟分叉失败/);
 assert.equal(document.querySelector(".tauri-topbar__title strong")?.textContent,"History A","a failed fork keeps the source open");
 delete state.__forkSessionHandler;
 await act(async()=>forkButton().click());
 await until(()=>document.querySelector(".tauri-topbar__title strong")?.textContent==="History A（分叉）" && !forkButton().disabled);
 assert.equal(calls("bridge_fork_session","history-a"),2);
 assert.equal(calls("bridge_switch_session","tauri-fork-child"),1);
 assert.equal(calls("bridge_session_history","tauri-fork-child"),1);
 assert.equal(document.querySelectorAll(".tauri-session-row").length,3,"source and child remain in sidebar");
 assert.equal(button("History A（分叉）").closest(".tauri-project-group")?.getAttribute("aria-label"),"ws","fork stays in the source workspace");
 assert.ok(document.querySelector(".tauri-transcript")?.textContent?.includes("First answer"),"fork displays the selected answer");
 assert.ok(!document.querySelector(".tauri-transcript")?.textContent?.includes("Later answer"),"fork excludes later turns");
 assert.equal(document.querySelectorAll(".tauri-answer-actions__fork").length,1);
 await act(async()=>button("History A").click());
 await until(()=>document.querySelector(".tauri-topbar__title strong")?.textContent==="History A" && !forkButton().disabled);
 assert.ok(document.querySelector(".tauri-transcript")?.textContent?.includes("Later answer"),"source retains later turns");
 assert.equal(button("History A（分叉）").disabled,false,"source can reopen without losing the child");
 state.__forkSessionHandler=async()=>{
  state.__workbenchSessions.unshift({sessionId:"tauri-fork-recovery",title:"Saved fork",workspaceRoot:"/tmp/ws"});
  state.__historyBySession={...state.__historyBySession,"tauri-fork-recovery":state.__tauriHistoryMessages.slice(0,2)};
  state.__workbenchPages=[new Error("模拟目录刷新失败")];
  return {ok:true,sessionId:"tauri-fork-recovery"};
 };
 await act(async()=>forkButton().click());
 await until(()=>document.body.textContent?.includes("分叉已创建（tauri-fork-recovery）")===true);
 assert.equal(calls("bridge_switch_session","tauri-fork-recovery"),0,"failed directory verification must not activate the child through stale state");
 assert.match(document.body.textContent??"",/重新检查会话目录/);
 assert.ok(!document.body.textContent?.includes("无法分叉对话"),"saved forks must not be reported as failed creation");
 delete state.__forkSessionHandler;
 await act(async()=>[...document.querySelectorAll<HTMLButtonElement>("button")].find(item=>item.textContent==="重新检查会话目录")!.click());
 await until(()=>document.querySelectorAll(".tauri-session-row").length===4 && !forkButton().disabled);
 assert.equal(button("Saved fork").disabled,false,"the durable child is recoverable after directory recheck");
 await draft("开始生成");
 await act(async()=>document.querySelector<HTMLButtonElement>(".tauri-send-button")?.click());
 await until(()=>calls("bridge_submit","history-a")===1);
 assert.equal(forkButton().disabled,true,"running sessions cannot fork");
 console.log("PASS fork action: source and child visible, inherited history, independent activation, duplicate protection, failure recovery, drafts and running state guards");
}finally{await act(async()=>root.unmount());dom.window.close();}
