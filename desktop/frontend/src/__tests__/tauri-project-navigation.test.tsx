// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
(globalThis as unknown as { __workbenchSessions: unknown[]; __previewFirstUsers: Record<string, string>; __workbenchPages: unknown[] }).__workbenchSessions = [
  { sessionId: "old-alpha", workspaceRoot: "/work/alpha" },
  { sessionId: "recent-beta", workspaceRoot: "/work/beta", title: "Beta task" },
];
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [
    { sessionId: "old-alpha", workspaceRoot: "/work/alpha" },
    { sessionId: "recent-beta", workspaceRoot: "/work/beta", title: "Beta task" },
  ], nextCursor: { position: 2, id: "recent-beta" }, total: 3, source: "identity" },
  { sessions: [{ sessionId: "older-gamma", workspaceRoot: "/work/gamma", title: "Gamma task" }], nextCursor: null, total: 3, source: "identity" },
];
(globalThis as unknown as { __previewFirstUsers: Record<string, string> }).__previewFirstUsers = { "old-alpha": "整理报告并加测试" };

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.equal(document.querySelectorAll(".tauri-project-group").length, 2);
assert.match(document.body.textContent ?? "", /整理报告并加测试/);
const calls = (globalThis as unknown as { __tauriBridgeCalls: Array<{ name: string; args: Record<string, string> }> }).__tauriBridgeCalls;
assert.ok(calls.some(call => call.name === "bridge_session_previews"));
assert.ok(calls.some(call => call.name === "backfill_workbench_titles"));
const loadMore = document.querySelector<HTMLButtonElement>('[aria-label="加载更多会话"]');
assert.ok(loadMore);
await act(async () => { loadMore.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /Gamma task/);
assert.equal(document.querySelector('[aria-label="加载更多会话"]'), null);

const select = document.querySelector<HTMLButtonElement>('[aria-label="切换到项目 beta"]');
assert.ok(select);
await act(async () => { select.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.some(call => call.name === "bridge_open_session" && call.args.sessionId === "recent-beta" && call.args.workspaceRoot === "/work/beta"));

const create = document.querySelector<HTMLButtonElement>('[aria-label="在 beta 中新建对话"]');
assert.ok(create);
await act(async () => { create.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.some(call => call.name === "bridge_switch_session" && call.args.workspaceRoot === "/work/beta"));
const composer = document.querySelector<HTMLTextAreaElement>(".tauri-composer textarea");
assert.ok(composer);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(composer, "修复侧栏显示问题");
  composer.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => {
  document.querySelector<HTMLButtonElement>('[aria-label="发送消息"]')?.click();
  await new Promise(resolve => setTimeout(resolve, 130));
});
assert.ok(calls.some(call => call.name === "backfill_workbench_titles" && JSON.stringify(call.args).includes("修复侧栏显示问题")));
assert.match(document.body.textContent ?? "", /修复侧栏显示问题/);
await act(async () => { root.unmount(); });
console.log("tauri project navigation and legacy title backfill: OK");
