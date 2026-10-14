// Run with CSS and tauri-bridge stub register loaders.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { TauriBridgeEvent, TauriLiveState } from "../lib/tauriBridge";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true, isTauri: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
dom.window.HTMLElement.prototype.scrollTo = function (options: ScrollToOptions | number, y?: number) { this.scrollTop = typeof options === "number" ? y ?? 0 : options.top ?? this.scrollTop; };
(dom.window as unknown as { __TAURI__: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };

Object.assign(dom.window, {
  __TAURI_INTERNALS__: { invoke: async (command: string) => command === "workspace_external_openers" ? [] : null, metadata: { currentWindow: { label: "main" }, currentWebview: { label: "main" } }, transformCallback: () => 1 },
  __TAURI_EVENT_PLUGIN_INTERNALS__: { unregisterListener: () => {} },
});

const fixture = globalThis as typeof globalThis & {
  __workbenchSessions: { sessionId: string; title: string; workspaceRoot: string }[];
  __historyBySession: Record<string, { role: string; content: string }[]>;
  __parallelSessionStates: Record<string, "idle" | "running" | "paused">;
  __parallelLiveStates: Record<string, TauriLiveState>;
  __parallelApprovals: Record<string, { id: string; tool: string; subject: string }>;
  __snapshotSequence: number;
  __emitBridgeEvent?: (event: TauriBridgeEvent) => void;
  __tauriBridgeCalls: { name: string; args: Record<string, unknown> }[];
  __historyGate?: Promise<void>;
  __switchFailure?: string;
  __steerFailure?: string;
  __chosenAttachmentPaths?: string[];
};
fixture.__workbenchSessions = ["a", "b"].map(id => ({ sessionId: id, title: "Task " + id.toUpperCase(), workspaceRoot: "/tmp/parallel" }));
fixture.__historyBySession = { a: [{ role: "user", content: "question A" }], b: [{ role: "user", content: "question B" }] };
fixture.__parallelSessionStates = { a: "running", b: "idle" };
fixture.__snapshotSequence = 10;
const live = (text: string): TauriLiveState => ({ active: true, answer: { segments: [text], streaming: true }, progress: { phase: "answering", tools: [] }, truncated: false });
fixture.__parallelLiveStates = { a: live("A checkpoint") };
fixture.__parallelApprovals = {};

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const settle = async () => { for (let i = 0; i < 3; i++) await new Promise(resolve => setTimeout(resolve, 0)); };
const body = () => document.body.textContent ?? "";
function row(id: string) { return [...document.querySelectorAll<HTMLElement>(".tauri-session-row")].find(item => item.textContent?.includes("Task " + id.toUpperCase()))!; }
async function select(id: string) { await act(async () => { row(id).querySelector<HTMLButtonElement>(".tauri-sidebar__session")!.click(); await settle(); }); }
async function typeDraft(value: string) { await act(async () => {
  const textarea = document.querySelector("textarea")!;
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(textarea, value);
  textarea.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  await settle();
}); }
async function emit(id: string, text: string) { await act(async () => {
  fixture.__emitBridgeEvent?.({ protocolVersion: 1, sessionId: id, sequence: ++fixture.__snapshotSequence, eventKind: "text", payload: { kind: "text", text } }); await settle();
}); }

try {
  await act(async () => { root.render(React.createElement(TauriSessionApp)); await settle(); });
  await select("a");
  assert(body().includes("A checkpoint"), "running response restores from authoritative checkpoint");
  assert.equal(row("b").querySelector<HTMLButtonElement>(".tauri-sidebar__session")!.disabled, false, "another session is selectable while A runs");
  assert(row("a").textContent?.includes("运行中"), "sidebar shows A activity");
  await typeDraft("先分析，不修改文件");
  fixture.__steerFailure = "当前任务未接受引导";
  await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="引导当前任务"]')!.click(); await settle(); });
  assert.equal(document.querySelector("textarea")!.value, "先分析，不修改文件", "rejected steering preserves the draft");
  fixture.__steerFailure = undefined;
  await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="引导当前任务"]')!.click(); await settle(); });
  assert.equal(fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_steer").at(-1)?.args.sessionId, "a");
  assert.equal(document.querySelector("textarea")!.value, "", "accepted guidance clears its own input");
  assert(body().includes("A checkpoint") && body().includes("引导已提交"), "guidance preserves the active answer and acknowledges admission");
  assert(document.querySelector('[aria-label="停止生成"]'), "independent stop control remains available");
  await typeDraft("unsent A");
  await select("b");
  assert.equal(document.querySelector("textarea")!.value, "", "A draft does not enter B");
  await typeDraft("unsent B");
  fixture.__chosenAttachmentPaths = ["/tmp/b-attachment.txt"];
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-attach-button")!.click(); await settle(); });
  assert(document.querySelector(".tauri-composer__attachments"), "B owns its attachment draft");
  await emit("a", "foreign A delta");
  assert(!body().includes("foreign A delta") && !body().includes("A checkpoint"), "background text does not mutate B transcript");
  const activityReads = fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_live_sessions").length;
  await act(async () => {
    for (let i = 0; i < 500; i++) fixture.__emitBridgeEvent?.({ protocolVersion: 1, sessionId: "a", sequence: ++fixture.__snapshotSequence, eventKind: "text", payload: { kind: "text", text: "background token" } });
    await settle();
  });
  assert.equal(fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_live_sessions").length, activityReads, "background token burst does not trigger sidebar requests");
  fixture.__parallelLiveStates.a = live("A recovered after background work");
  await select("a");
  assert.equal(document.querySelector("textarea")!.value, "unsent A", "A draft restored");
  assert(!document.querySelector(".tauri-composer__attachments"), "B attachment did not move into A");
  assert(body().includes("A recovered after background work"), "A output refreshed after switch back");
  await select("b");
  assert.equal(document.querySelector("textarea")!.value, "unsent B", "B draft restored");
  assert(document.querySelector(".tauri-composer__attachments"), "B attachment survives navigation");

  // A pending old-history promise must not repaint a newer selected surface.
  let releaseHistory!: () => void;
  fixture.__historyGate = new Promise<void>(resolve => { releaseHistory = resolve; });
  await select("a");
  fixture.__historyGate = undefined;
  await select("b");
  await act(async () => { releaseHistory(); await settle(); });
  assert(body().includes("question B") && !body().includes("question A"), "late A history cannot overwrite B");

  fixture.__parallelSessionStates.a = "paused";
  fixture.__parallelSessionStates.b = "paused";
  fixture.__parallelApprovals = { a: { id: "same-id", tool: "tool A", subject: "Approval A" }, b: { id: "same-id", tool: "tool B", subject: "Approval B" } };
  await select("a");
  assert(body().includes("Approval A") && !body().includes("Approval B"), "A prompt replays with exact ownership");
  await select("b");
  assert(body().includes("Approval B") && !body().includes("Approval A"), "paused A does not block B prompt");
  await typeDraft("先检查再继续");
  await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="引导当前任务"]')!.click(); await settle(); });
  assert(body().includes("Approval B"), "steering does not bypass pending approval");
  assert(document.querySelector(".tauri-composer__attachments"), "text steering retains staged attachments");
  await typeDraft("unsent B");
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-prompt-card__allow")!.click(); await settle(); });
  const approval = fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_approve").at(-1);
  assert.equal(approval?.args.sessionId, "b", "same-number approval submits to B only");

  fixture.__switchFailure = "fixture open failed";
  await select("a");
  assert(body().includes("question B"), "failed navigation preserves B surface");
  assert.equal(document.querySelector("textarea")!.value, "unsent B", "failed navigation preserves B draft");
  fixture.__switchFailure = undefined;
  fixture.__parallelSessionStates.b = "running";
  delete fixture.__parallelApprovals.b;
  await select("a");
  await select("b");
  await act(async () => { document.querySelector<HTMLButtonElement>('button[aria-label="停止生成"]')!.click(); await settle(); });
  assert.equal(fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_cancel").at(-1)?.args.sessionId, "b", "stop routes only to selected B");
  assert.equal(fixture.__parallelSessionStates.a, "paused", "stopping B preserves A waiting state");
  // Empty composer continues only an existing, loaded and idle conversation.
  await typeDraft("");
  await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label^="移除文件"]')!.click(); await settle(); });
  await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="继续运行"]')!.click(); await settle(); });
  assert.equal(fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_continue").at(-1)?.args.sessionId, "b");
  assert.equal(fixture.__parallelSessionStates.a, "paused", "empty continuation preserves the other owner");
  assert(!body().includes("使用当前模型继续，不追加消息"), "continuation instructions do not enter the transcript");
  await act(async () => { [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.includes("新建对话"))!.click(); await settle(); });
  assert.equal(document.querySelector<HTMLButtonElement>('[aria-label="发送消息"]')!.disabled, true, "blank new conversations cannot continue");
  console.log("PASS parallel workspace: switching, checkpoints, drafts, attachments, late history, prompts and independent stop");
} finally {
  await act(async () => { root.unmount(); });
  dom.window.close();
}
