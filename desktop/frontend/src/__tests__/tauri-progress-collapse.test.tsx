// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
(globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [{ sessionId: "progress-session", title: "检查提交" }];
(globalThis as unknown as { __tauriHistoryMessages: unknown[] }).__tauriHistoryMessages = [
  { role: "user", content: "检查提交" },
  { role: "assistant", content: "先查看工作区", workDurationMs: 5_000 },
  { role: "assistant", content: "发现两个提交", workDurationMs: 70_000 },
  { role: "assistant", content: "结论：需要测试", workDurationMs: 960_000 },
];

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
const sessionButton = document.querySelector<HTMLButtonElement>(".tauri-sidebar__session");
assert.ok(sessionButton);
assert.equal(sessionButton.disabled, false, document.body.textContent ?? "");
await act(async () => {
  sessionButton.click();
  await new Promise(resolve => setTimeout(resolve, 0));
});

const progress = document.querySelector<HTMLDetailsElement>(".tauri-progress");
assert.ok(progress, "intermediate updates render inside a progress disclosure");
assert.equal(progress.open, false, "progress is collapsed by default");
assert.match(progress.querySelector("summary")?.textContent ?? "", /用时 16 分钟.*2 条过程更新/);
assert.match(progress.textContent ?? "", /先查看工作区.*发现两个提交/);
assert.ok([...document.querySelectorAll(".tauri-message.is-assistant")].some(message =>
  !progress.contains(message) && message.textContent?.includes("结论：需要测试")), "the final answer stays outside the disclosure");
await act(async () => { progress.querySelector("summary")?.click(); });
assert.equal(progress.open, true, "the user can expand the progress disclosure");
await act(async () => { root.unmount(); });
console.log("tauri progress disclosure: OK");
