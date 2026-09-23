// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-chat-workspace-delete.test.tsx
//
// Mounts the Tauri chat workspace in jsdom and drives the real controls:
// workspace requests resolving out of order, followed by the two-step session
// delete flow and event-stream recovery.

import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", {
  url: "http://localhost/",
  pretendToBeVisual: true,
});
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});
// Node 22 exposes `navigator` as a getter-only global, so it needs a definition
// rather than an assignment.
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
// The adapter stub answers every Tauri call; these markers keep any host check
// inside the component on the Tauri path.
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };

let passed = 0;
let failed = 0;

function ok(value: unknown, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq(actual: unknown, expected: unknown, label: string) {
  ok(actual === expected, `${label} (expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)})`);
}

function settle(): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, 0));
}

function clickByClass(className: string): boolean {
  const element = document.querySelector<HTMLElement>(`.${className}`);
  if (!element) return false;
  element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return true;
}

function clickButton(label: string): boolean {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")]
    .find(candidate => candidate.textContent?.trim() === label);
  if (!button) return false;
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return true;
}

function clickSession(title: string): boolean {
  const row = [...document.querySelectorAll<HTMLElement>(".tauri-session-row")]
    .find(candidate => candidate.textContent?.includes(title));
  const button = row?.querySelector<HTMLElement>(".tauri-sidebar__session");
  if (!button) return false;
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return true;
}

function clickDeleteFor(title: string): boolean {
  const row = [...document.querySelectorAll<HTMLElement>(".tauri-session-row")]
    .find(candidate => candidate.textContent?.includes(title));
  const button = row?.querySelector<HTMLElement>(".tauri-session-row__delete");
  if (!button) return false;
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return true;
}

function text(): string {
  return document.body.textContent ?? "";
}

const bridgeCalls = (): Array<{ name: string; args: unknown }> =>
  (globalThis as unknown as { __tauriBridgeCalls: Array<{ name: string; args: unknown }> }).__tauriBridgeCalls;

async function main() {
  (globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [
    { sessionId: "tauri-doomed-row", title: "待删除会话", workspaceRoot: "/tmp/ws" },
    { sessionId: "tauri-kept-row", title: "保留会话", workspaceRoot: "/tmp/ws" },
  ];
  (globalThis as unknown as { __tauriHistoryMessages: unknown[] }).__tauriHistoryMessages = [
    { role: "assistant", content: "```js\nconst answer = 42;\n```" },
  ];

  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");

  const root = createRoot(document.getElementById("root")!);
  await act(async () => {
    root.render(React.createElement(TauriSessionApp));
  });
  await act(async () => {
    await settle();
    await settle();
  });

  console.log("\ntauri chat workspace — workspace, delete, and recovery flows");
  ok(text().includes("待删除会话"), "the recent-session list renders the stored title");
  eq(document.querySelectorAll(".tauri-session-row__delete").length, 2, "each row has a delete control");

  // Step 1: the row-level delete control opens an inline confirmation.
  const opened = clickByClass("tauri-session-row__delete");
  ok(opened, "the delete control is present and clickable");
  await act(async () => {
    await settle();
  });
  ok(text().includes("删除“待删除会话”？"), "clicking delete asks for confirmation in place");
  eq(document.querySelectorAll(".tauri-session-delete__confirm").length, 1, "the confirmation offers a confirm button");

  // Step 2: cancelling must not touch the bridge.
  clickByClass("tauri-session-delete__cancel");
  await act(async () => {
    await settle();
  });
  eq(bridgeCalls().filter(call => call.name === "bridge_delete_session").length, 0, "cancelling never reaches the bridge");
  ok(text().includes("待删除会话"), "cancelling restores the row");

  // Keep another session open while deleting an inactive row. The bridge owns
  // one controller, so the delete flow must restore the previously open row.
  (globalThis as unknown as { __holdStreamReady?: boolean }).__holdStreamReady = true;
  ok(clickSession("保留会话"), "the other conversation can be opened first");
  await act(async () => {
    await settle();
    await settle();
  });
  ok(bridgeCalls().some(call => call.name === "bridge_start_events"), "opening a conversation starts the event subscription");
  ok(document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "subscription command completion alone does not enable sending");
  await act(async () => {
    (globalThis as unknown as { __emitBridgeRestored?: () => void }).__emitBridgeRestored?.();
    await settle();
  });
  ok(!document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "confirmed initial event connection enables sending");
  (globalThis as unknown as { __holdStreamReady?: boolean }).__holdStreamReady = false;
  ok(text().includes("const answer = 42"), "the Tauri entry renders historical Markdown without a localization crash");

  const workspaceStub = globalThis as unknown as {
    __workspaceHandler?: (sessionId: string, path: string) => Promise<object>;
    __workspaceFileHandler?: (sessionId: string, path: string) => Promise<object>;
    __workspaceChangesHandler?: (sessionId: string) => Promise<object>;
    __workspaceDetailHandler?: (sessionId: string, path: string) => Promise<object>;
  };
  const keptFiles = { path: "", truncated: false, entries: [
    { path: "a.txt", name: "a.txt", isDir: false },
    { path: "b.txt", name: "b.txt", isDir: false },
  ] };
  const otherFiles = { path: "", truncated: false, entries: [{ path: "other.txt", name: "other.txt", isDir: false }] };
  workspaceStub.__workspaceHandler = async sessionId => sessionId === "tauri-kept-row" ? keptFiles : otherFiles;
  await act(async () => { clickByClass("tauri-workspace-tree-button"); await settle(); });
  ok(text().includes("a.txt") && text().includes("b.txt"), "workspace drawer lists the current session's files");

  let releaseOldPreview: ((value: object) => void) | undefined;
  const oldPreview = new Promise<object>(resolve => { releaseOldPreview = resolve; });
  workspaceStub.__workspaceFileHandler = async (_sessionId, path) => path === "a.txt"
    ? oldPreview
    : { path, body: "new preview", size: 11, binary: false, truncated: false };
  const doubleClickFile = (path: string) => document.querySelector<HTMLElement>(`.tauri-workspace-entry[title*="${path}"]`)
    ?.dispatchEvent(new dom.window.MouseEvent("dblclick", { bubbles: true, cancelable: true }));
  await act(async () => { doubleClickFile("a.txt"); await settle(); });
  await act(async () => { doubleClickFile("b.txt"); await settle(); });
  await act(async () => {
    releaseOldPreview?.({ path: "a.txt", body: "stale preview", size: 13, binary: false, truncated: false });
    await settle();
  });
  ok(text().includes("new preview") && !text().includes("stale preview"), "late preview of the first file cannot replace the latest file");

  workspaceStub.__workspaceChangesHandler = async () => ({ gitAvailable: true, files: [
    { path: "a.txt", gitStatus: "M", sources: ["git"] },
    { path: "b.txt", gitStatus: "M", sources: ["git"] },
  ] });
  let releaseOldDetail: ((value: object) => void) | undefined;
  const oldDetail = new Promise<object>(resolve => { releaseOldDetail = resolve; });
  workspaceStub.__workspaceDetailHandler = async (_sessionId, path) => path === "a.txt"
    ? oldDetail
    : { source: "git", diff: "new diff", binary: false, added: 1, removed: 0, truncated: false };
  await act(async () => { clickButton("变更"); await settle(); });
  const clickChange = (path: string) => document.querySelector<HTMLElement>(`.tauri-workspace-change-entry[title*="${path}"]`)
    ?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  await act(async () => { clickChange("a.txt"); await settle(); });
  await act(async () => { clickChange("b.txt"); await settle(); });
  await act(async () => {
    releaseOldDetail?.({ source: "git", diff: "stale diff", binary: false, added: 1, removed: 0, truncated: false });
    await settle();
  });
  ok(text().includes("new diff") && !text().includes("stale diff"), "late diff of the first file cannot replace the latest diff");
  await act(async () => { clickButton("文件"); await settle(); });
  let rejectOldPreview: ((reason: Error) => void) | undefined;
  const failingPreview = new Promise<object>((_resolve, reject) => { rejectOldPreview = reject; });
  workspaceStub.__workspaceFileHandler = async () => failingPreview;
  await act(async () => { doubleClickFile("a.txt"); await settle(); });
  await act(async () => { clickButton("变更"); await settle(); });
  await act(async () => { rejectOldPreview?.(new Error("stale preview failure")); await settle(); });
  ok(!text().includes("stale preview failure"), "file preview failure cannot leak into the changes view");
  await act(async () => { clickButton("文件"); await settle(); });

  let releaseOldListing: ((value: object) => void) | undefined;
  const oldListing = new Promise<object>(resolve => { releaseOldListing = resolve; });
  workspaceStub.__workspaceHandler = async sessionId => sessionId === "tauri-kept-row" ? oldListing : otherFiles;
  await act(async () => { clickButton("刷新"); await settle(); });
  ok(clickSession("待删除会话"), "a session switch can start while a workspace request is pending");
  await act(async () => { await settle(); await settle(); });
  await act(async () => { clickByClass("tauri-workspace-tree-button"); await settle(); });
  ok(text().includes("other.txt"), "new session workspace appears before the old request finishes");
  await act(async () => { releaseOldListing?.(keptFiles); await settle(); });
  ok(text().includes("other.txt") && !text().includes("a.txt"), "late listing from the previous session cannot replace the active workspace");
  workspaceStub.__workspaceHandler = async sessionId => sessionId === "tauri-kept-row" ? keptFiles : otherFiles;
  workspaceStub.__workspaceFileHandler = undefined;
  workspaceStub.__workspaceChangesHandler = undefined;
  workspaceStub.__workspaceDetailHandler = undefined;
  await act(async () => { clickSession("保留会话"); await settle(); await settle(); });

  // Step 3: confirming switches to the target, deletes it, removes the host
  // catalog entry, then returns to the previously open conversation.
  ok(clickDeleteFor("待删除会话"), "the intended inactive row can be armed for deletion");
  await act(async () => {
    await settle();
  });
  clickByClass("tauri-session-delete__confirm");
  await act(async () => {
    await settle();
    await settle();
  });
  const calls = bridgeCalls().map(call => call.name);
  ok(calls.includes("bridge_switch_session"), "deleting an inactive session switches to it first");
  const deleted = bridgeCalls().find(call => call.name === "bridge_delete_session");
  ok(deleted !== undefined, "confirming reaches the delete adapter");
  eq((deleted?.args as { sessionId?: string } | undefined)?.sessionId, "tauri-doomed-row", "the delete targets the confirmed row");
  const forgotten = bridgeCalls().find(call => call.name === "forget_workbench_session");
  ok(forgotten !== undefined, "successful core deletion removes the persistent host catalog entry");
  eq((forgotten?.args as { sessionId?: string } | undefined)?.sessionId, "tauri-doomed-row", "the catalog cleanup targets the deleted row");
  const switches = bridgeCalls().filter(call => call.name === "bridge_switch_session");
  eq((switches.at(-1)?.args as { sessionId?: string } | undefined)?.sessionId, "tauri-kept-row", "deleting an inactive row restores the previously open conversation");
  ok(!text().includes("待删除会话"), "the deleted row leaves the list");
  ok(text().includes("保留会话"), "the other row survives");
  eq(document.querySelectorAll(".tauri-session-row__delete").length, 1, "one recent conversation remains");

  // Step 4: a bridge failure must surface instead of silently restoring.
  (globalThis as unknown as { __deleteFailure?: string }).__deleteFailure = "desktop bridge already owns a different session";
  clickByClass("tauri-session-row__delete");
  await act(async () => {
    await settle();
  });
  clickByClass("tauri-session-delete__confirm");
  await act(async () => {
    await settle();
    await settle();
  });
  ok(text().includes("already owns a different session"), "a rejected delete shows the bridge error");
  ok(text().includes("保留会话"), "a rejected delete keeps the row");

  // A stale UI state must not let restart interrupt a turn that the bridge
  // reports as running. Once idle, the same control can apply a new key.
  ok(clickButton("运行状态"), "runtime panel can be opened");
  await act(async () => { await settle(); });
  (globalThis as unknown as { __snapshotState?: string }).__snapshotState = "running";
  ok(clickButton("重启桥接服务并恢复当前会话"), "restart control is present");
  await act(async () => { await settle(); });
  eq(bridgeCalls().filter(call => call.name === "restart_bridge").length, 0, "running turn cannot be interrupted by restart");
  ok(text().includes("请等待当前回合结束"), "running turn shows an actionable explanation");

  (globalThis as unknown as { __snapshotState?: string }).__snapshotState = "idle";
  clickButton("重启桥接服务并恢复当前会话");
  await act(async () => { await settle(); await settle(); });
  eq(bridgeCalls().filter(call => call.name === "restart_bridge").length, 1, "idle session can restart to apply its key");

  const streamCallbacks = globalThis as unknown as {
    __emitBridgeEvent?: (event: { sessionId: string; sequence: number; eventKind: string; payload: object }) => void;
    __emitBridgeError?: (message: string) => void;
    __emitBridgeRestored?: () => void;
    __emitBridgeResync?: () => void;
  };
  await act(async () => { streamCallbacks.__emitBridgeError?.("temporarily unavailable"); });
  ok(document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "event stream outage disables sending");
  await act(async () => { streamCallbacks.__emitBridgeRestored?.(); await settle(); });
  ok(!document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "reconnected event stream re-enables sending");

  const snapshotsBeforeResync = bridgeCalls().filter(call => call.name === "bridge_session_snapshot").length;
  const subscriptionsBeforeResync = bridgeCalls().filter(call => call.name === "bridge_start_events").length;
  (globalThis as unknown as { __outageOnStart?: boolean }).__outageOnStart = true;
  await act(async () => { streamCallbacks.__emitBridgeResync?.(); await settle(); await settle(); });
  eq(bridgeCalls().filter(call => call.name === "bridge_session_snapshot").length, snapshotsBeforeResync + 1, "expired replay window fetches a fresh snapshot");
  eq(bridgeCalls().filter(call => call.name === "bridge_start_events").length, subscriptionsBeforeResync + 1, "resync starts a new event subscription");
  ok(document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "outage during subscription setup cannot re-enable sending");
  (globalThis as unknown as { __outageOnStart?: boolean }).__outageOnStart = false;
  await act(async () => { streamCallbacks.__emitBridgeRestored?.(); await settle(); });
  ok(!document.querySelector<HTMLTextAreaElement>("textarea")?.disabled, "recovery after setup outage re-enables sending");

  const replayed = { sessionId: "tauri-kept-row", sequence: 1, eventKind: "text", payload: { kind: "text", text: "hello" } };
  await act(async () => { streamCallbacks.__emitBridgeEvent?.(replayed); streamCallbacks.__emitBridgeEvent?.(replayed); });
  eq(document.querySelectorAll(".tauri-event-details li").length, 1, "replayed event sequence is applied only once");

  (globalThis as unknown as { __snapshotState?: string }).__snapshotState = "running";
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 2, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await new Promise(resolve => setTimeout(resolve, 650));
  });
  ok(Boolean(document.querySelector('button[aria-label="停止生成"]')), "turn_done does not invent idle state while snapshots remain running");
  ok(text().includes("会话尚未空闲"), "unconfirmed turn completion asks for an authoritative refresh");

  (globalThis as unknown as { __snapshotState?: string }).__snapshotState = "idle";
  await act(async () => { clickButton("刷新当前对话状态与记录"); await settle(); });
  ok(!document.querySelector('button[aria-label="停止生成"]'), "manual refresh restores idle state from the bridge");

  (globalThis as unknown as { __snapshotStates?: string[] }).__snapshotStates = ["running", "idle"];
  const snapshotsBeforeCompletion = bridgeCalls().filter(call => call.name === "bridge_session_snapshot").length;
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 3, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await new Promise(resolve => setTimeout(resolve, 180));
  });
  eq(bridgeCalls().filter(call => call.name === "bridge_session_snapshot").length, snapshotsBeforeCompletion + 2, "finishing snapshot is retried until idle");
  ok(!text().includes("会话尚未空闲"), "confirmed idle state clears the stale completion warning");

  (globalThis as unknown as { __snapshotError?: boolean }).__snapshotError = true;
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 4, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await new Promise(resolve => setTimeout(resolve, 650));
  });
  ok(Boolean(document.querySelector('button[aria-label="停止生成"]')), "failed snapshots keep turn admission closed");
  ok(text().includes("无法确认当前会话状态"), "snapshot failure remains visible instead of claiming completion");
  (globalThis as unknown as { __snapshotError?: boolean }).__snapshotError = false;
  await act(async () => { clickButton("刷新当前对话状态与记录"); await settle(); });
  ok(!document.querySelector('button[aria-label="停止生成"]'), "manual refresh recovers from snapshot failure");

  let releaseStaleSnapshot: (() => void) | undefined;
  (globalThis as unknown as { __snapshotGate?: Promise<void> }).__snapshotGate = new Promise(resolve => { releaseStaleSnapshot = resolve; });
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 5, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await settle();
  });
  (globalThis as unknown as { __snapshotGate?: Promise<void> }).__snapshotGate = undefined;
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 6, eventKind: "turn_started", payload: { kind: "turn_started" } });
    releaseStaleSnapshot?.();
    await settle();
  });
  ok(Boolean(document.querySelector('button[aria-label="停止生成"]')), "late snapshot from an older completed turn cannot mark a new turn idle");

  await act(async () => { clickButton("刷新当前对话状态与记录"); await settle(); });
  const composer = document.querySelector<HTMLTextAreaElement>(".tauri-composer textarea");
  ok(Boolean(composer), "composer is available for submit guard test");
  await act(async () => {
    Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(composer, "one request");
    composer?.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  });
  let releaseSubmit: (() => void) | undefined;
  (globalThis as unknown as { __submitGate?: Promise<void> }).__submitGate = new Promise(resolve => { releaseSubmit = resolve; });
  const submitsBefore = bridgeCalls().filter(call => call.name === "bridge_submit").length;
  await act(async () => {
    const send = document.querySelector<HTMLButtonElement>('button[aria-label="发送消息"]');
    send?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    send?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
  eq(bridgeCalls().filter(call => call.name === "bridge_submit").length, submitsBefore + 1, "two immediate sends start one bridge request");
  (globalThis as unknown as { __submitGate?: Promise<void> }).__submitGate = undefined;
  await act(async () => { releaseSubmit?.(); await new Promise(resolve => setTimeout(resolve, 130)); });

  let releaseStaleHistory: (() => void) | undefined;
  (globalThis as unknown as { __tauriHistoryMessages?: object[] }).__tauriHistoryMessages = [{ role: "assistant", content: "stale completion marker" }];
  (globalThis as unknown as { __historyGate?: Promise<void> }).__historyGate = new Promise(resolve => { releaseStaleHistory = resolve; });
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 7, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await settle();
  });
  (globalThis as unknown as { __historyGate?: Promise<void> }).__historyGate = undefined;
  (globalThis as unknown as { __tauriHistoryMessages?: object[] }).__tauriHistoryMessages = [];
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 8, eventKind: "turn_started", payload: { kind: "turn_started" } });
    releaseStaleHistory?.();
    await settle();
  });
  ok(!text().includes("stale completion marker"), "late history from an older turn cannot replace the new turn transcript");

  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 9, eventKind: "mcp_interaction", payload: { mcpInteraction: { id: "unsafe-link", server: "demo", mode: "url", url: "javascript:alert(1)" } } });
    await settle();
  });
  ok(text().includes("链接不可安全打开"), "unsafe MCP link gives a visible configuration warning");
  eq(document.querySelectorAll(".tauri-prompt-card__link").length, 0, "unsafe MCP link is never rendered as a clickable anchor");
  await act(async () => {
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 10, eventKind: "prompt_answered", payload: { promptId: "unsafe-link" } });
    streamCallbacks.__emitBridgeEvent?.({ sessionId: "tauri-kept-row", sequence: 11, eventKind: "mcp_interaction", payload: { mcpInteraction: { id: "safe-link", server: "demo", mode: "url", url: "https://docs.example.test/authorize" } } });
    await settle();
  });
  const safeLink = document.querySelector<HTMLAnchorElement>(".tauri-prompt-card__link");
  eq(safeLink?.getAttribute("href"), "https://docs.example.test/authorize", "safe MCP link keeps its authorization path");
  ok(safeLink?.textContent?.includes("docs.example.test"), "MCP link shows the actual destination host");

  await act(async () => {
    root.unmount();
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

await main();
