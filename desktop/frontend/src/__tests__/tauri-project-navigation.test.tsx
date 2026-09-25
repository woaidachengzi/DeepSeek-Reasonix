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
    { sessionId: "missing-alpha", title: "丢失文件会话", workspaceRoot: "/work/alpha", state: "missing", missing: true },
    { sessionId: "old-alpha", workspaceRoot: "/work/alpha" },
    { sessionId: "recent-beta", workspaceRoot: "/work/beta", title: "Beta task" },
  ], nextCursor: { position: 3, id: "recent-beta", snapshotId: "a".repeat(64) }, total: 4, source: "identity" },
  { sessions: [{ sessionId: "older-gamma", workspaceRoot: "/work/gamma", title: "Gamma task" }], nextCursor: null, total: 3, source: "identity" },
];
(globalThis as unknown as { __previewFirstUsers: Record<string, string> }).__previewFirstUsers = { "old-alpha": "整理报告并加测试" };
(globalThis as unknown as { __unavailableWorkspaceRoots: string[] }).__unavailableWorkspaceRoots = ["/work/alpha"];

(globalThis as typeof globalThis & { __savedProjectFolders: unknown[] }).__savedProjectFolders = [
  { root: "/work/empty", title: "Saved empty project" },
];

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
let root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.equal(document.querySelectorAll(".tauri-project-group").length, 3);
assert.ok(document.querySelector('[aria-label="切换到项目 Saved empty project"]'));
assert.ok(document.querySelector('[aria-label="在 Saved empty project 中新建对话"]'));
assert.match(document.body.textContent ?? "", /整理报告并加测试/);
const calls = (globalThis as unknown as { __tauriBridgeCalls: Array<{ name: string; args: Record<string, string> }> }).__tauriBridgeCalls;
assert.ok(calls.some(call => call.name === "workbench_project_folders"));
assert.ok(calls.some(call => call.name === "bridge_session_previews"));
assert.ok(calls.some(call => call.name === "backfill_workbench_titles"));
const loadMore = document.querySelector<HTMLButtonElement>('[aria-label="加载更多会话"]');
assert.ok(loadMore);
await act(async () => { loadMore.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /Gamma task/);
assert.equal(document.querySelector('[aria-label="加载更多会话"]'), null);
const continuationCall = calls.find(call => call.name === "workbench_session_page" && Boolean(call.args.cursor));
assert.ok(continuationCall, "continuation requests preserve the server cursor");
assert.equal((continuationCall.args.cursor as unknown as { snapshotId: string }).snapshotId, "a".repeat(64));

const missingRow = document.querySelector<HTMLButtonElement>('[aria-label="丢失文件会话（文件缺失）"]');
assert.ok(missingRow?.disabled);
assert.match(missingRow?.textContent ?? "", /文件缺失/);
assert.ok(document.querySelector<HTMLButtonElement>('[aria-label="删除对话 丢失文件会话"]'), "missing sessions remain explicitly deletable");
await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /工作区不可用/);
assert.ok(document.querySelector<HTMLButtonElement>('[aria-label="在 alpha 中新建对话"]')?.disabled);
assert.ok(!document.querySelector<HTMLButtonElement>('[aria-label="在 beta 中新建对话"]')?.disabled);

const select = document.querySelector<HTMLButtonElement>('[aria-label="切换到项目 beta"]');
assert.ok(select);
const alphaSelect = document.querySelector<HTMLButtonElement>('[aria-label="切换到项目 alpha（工作区不可用）"]');
assert.ok(alphaSelect);
await act(async () => { alphaSelect.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.some(call => call.name === "bridge_open_session" && call.args.sessionId === "old-alpha"), "project switching skips the newest missing session");
await act(async () => { select.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.some(call => call.name === "bridge_switch_session" && call.args.sessionId === "recent-beta" && call.args.workspaceRoot === "/work/beta"));

const create = document.querySelector<HTMLButtonElement>('[aria-label="在 beta 中新建对话"]');
assert.ok(create);
await act(async () => { create.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.some(call => call.name === "bridge_switch_session" && call.args.workspaceRoot === "/work/beta"));
await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok(calls.filter(call => call.name === "workbench_session_page").length >= 4, "catalog mutations refresh the already-loaded identity pages");
assert.ok([...document.querySelectorAll<HTMLButtonElement>(".tauri-sidebar__session")].some(button => button.textContent?.trim() === "新对话"), "a newly created session appears in the identity-backed sidebar");
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

(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "stale-first-page", title: "旧快照" }], nextCursor: { position: 1, id: "stale-first-page", snapshotId: "b".repeat(64) }, total: 2, source: "identity" },
  new Error("session catalog changed while paging; restart the session list"),
  { sessions: [{ sessionId: "fresh-first-page", title: "新快照" }], nextCursor: null, total: 1, source: "legacy" },
];
root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /旧快照/);
const staleLoadMore = document.querySelector<HTMLButtonElement>('[aria-label="加载更多会话"]');
assert.ok(staleLoadMore);
await act(async () => { staleLoadMore.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /新快照/);
assert.doesNotMatch(document.body.textContent ?? "", /旧快照/);
assert.equal(document.querySelector('[aria-label="加载更多会话"]'), null);
await act(async () => { root.unmount(); });

(globalThis as unknown as { __failSessionCatalogShadow: boolean }).__failSessionCatalogShadow = true;
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "identity-before-audit", title: "审计前身份页" }], nextCursor: null, total: 1, source: "identity" },
  { sessions: [{ sessionId: "legacy-after-audit-failure", title: "审计失败后的 JSON 回退" }], nextCursor: null, total: 1, source: "legacy" },
  { sessions: [{ sessionId: "verified-after-retry", title: "重查后身份目录" }], nextCursor: null, total: 1, source: "identity" },
];
root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(React.createElement(TauriSessionApp));
  await new Promise(resolve => setTimeout(resolve, 0));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.match(document.body.textContent ?? "", /审计失败后的 JSON 回退/);
assert.doesNotMatch(document.body.textContent ?? "", /审计前身份页/);
assert.match(document.body.textContent ?? "", /最多 50 条/);
const retryDirectory = document.querySelector<HTMLButtonElement>('[aria-label="重新检查会话目录"]');
assert.ok(retryDirectory, "legacy fallback offers an explicit recheck");
await act(async () => { retryDirectory.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /重查后身份目录/);
assert.doesNotMatch(document.body.textContent ?? "", /审计失败后的 JSON 回退/);
await act(async () => { root.unmount(); });

(globalThis as unknown as { __failSessionCatalogShadow: boolean }).__failSessionCatalogShadow = true;
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "unverified-audit-page", title: "审计失效后残留的身份页" }], nextCursor: null, total: 1, source: "identity" },
  new Error("legacy catalog unavailable"),
];
root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(React.createElement(TauriSessionApp));
  await new Promise(resolve => setTimeout(resolve, 0));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.doesNotMatch(document.body.textContent ?? "", /审计失效后残留的身份页/);
assert.match(document.body.textContent ?? "", /legacy catalog unavailable/);
assert.equal(document.querySelector('[aria-label="加载更多会话"]'), null);
await act(async () => { root.unmount(); });

(globalThis as unknown as { __failSessionCatalogShadow: boolean }).__failSessionCatalogShadow = false;
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "unverified-continuation-page", title: "续页回读失败后残留的身份页" }], nextCursor: { position: 1, id: "unverified-continuation-page", snapshotId: "c".repeat(64) }, total: 2, source: "identity" },
  new Error("session directory changed while paging"),
  new Error("session catalog shadow unavailable"),
];
root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
const failedRestartLoadMore = document.querySelector<HTMLButtonElement>('[aria-label="加载更多会话"]');
assert.ok(failedRestartLoadMore);
await act(async () => { failedRestartLoadMore.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.doesNotMatch(document.body.textContent ?? "", /续页回读失败后残留的身份页/);
assert.match(document.body.textContent ?? "", /session catalog shadow unavailable/);
assert.equal(document.querySelector('[aria-label="加载更多会话"]'), null);
await act(async () => { root.unmount(); });

// Deletion recovery is sourced from SQLite even when the ordinary sidebar
// catalog cannot be loaded at all.
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [new Error("recent catalog unavailable")];
(globalThis as unknown as { __pendingSessionDeletes: unknown[] }).__pendingSessionDeletes = [
  { id: "orphan-pending-delete", title: "独立恢复记录" },
];
root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(React.createElement(TauriSessionApp));
  await new Promise(resolve => setTimeout(resolve, 0));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.match(document.body.textContent ?? "", /recent catalog unavailable/);
assert.match(document.body.textContent ?? "", /待完成删除/);
assert.ok(document.querySelector<HTMLButtonElement>('[aria-label="继续删除 独立恢复记录"]'));
await act(async () => { root.unmount(); });

// The standalone shadow check can be clean just before the guarded first-page
// command observes a new drift. A legacy page must stay labeled as legacy.
(globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [
  { sessionId: "before-race", title: "审计时的身份页" },
];
(globalThis as unknown as { __pendingSessionDeletes: unknown[] }).__pendingSessionDeletes = [];
(globalThis as unknown as { __failSessionCatalogShadow: boolean }).__failSessionCatalogShadow = false;
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "before-race", title: "审计时的身份页" }], nextCursor: null, total: 1, source: "identity" },
  { sessions: [{ sessionId: "after-race", title: "漂移后的兼容页" }], nextCursor: null, total: 1, source: "legacy" },
];
root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
await act(async () => {
  document.querySelector<HTMLButtonElement>(".tauri-sidebar__new")?.click();
  await new Promise(resolve => setTimeout(resolve, 0));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.match(document.body.textContent ?? "", /漂移后的兼容页/);
assert.doesNotMatch(document.body.textContent ?? "", /审计时的身份页/);
assert.match(document.body.textContent ?? "", /最多 50 条/);
assert.ok(document.querySelector<HTMLButtonElement>('[aria-label="重新检查会话目录"]'));
await act(async () => { root.unmount(); });

let rejectAudit!: (reason: Error) => void;
let resolveOldContinuation!: (page: unknown) => void;
const delayedAudit = new Promise<unknown>((_, reject) => { rejectAudit = reject; });
const delayedContinuation = new Promise<unknown>(resolve => { resolveOldContinuation = resolve; });
(globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [
  { sessionId: "old-first", title: "旧身份首屏" },
];
(globalThis as unknown as { __sessionCatalogShadowResponses: unknown[] }).__sessionCatalogShadowResponses = [delayedAudit];
(globalThis as unknown as { __workbenchPages: unknown[] }).__workbenchPages = [
  { sessions: [{ sessionId: "old-first", title: "旧身份首屏" }], nextCursor: { position: 0, id: "old-first", snapshotId: "d".repeat(64) }, total: 2, source: "identity" },
  delayedContinuation,
  { sessions: [{ sessionId: "fallback-first", title: "审计失败后的兼容首屏" }], nextCursor: null, total: 1, source: "legacy" },
];
root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
await act(async () => {
  document.querySelector<HTMLButtonElement>('[aria-label="加载更多会话"]')?.click();
  await new Promise(resolve => setTimeout(resolve, 0));
});
await act(async () => { rejectAudit(new Error("shadow changed during continuation")); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.match(document.body.textContent ?? "", /审计失败后的兼容首屏/);
assert.match(document.body.textContent ?? "", /最多 50 条/);
await act(async () => {
  resolveOldContinuation({ sessions: [{ sessionId: "stale-second", title: "过时的身份续页" }], nextCursor: null, total: 2, source: "identity" });
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.doesNotMatch(document.body.textContent ?? "", /过时的身份续页/);
assert.match(document.body.textContent ?? "", /最多 50 条/);
await act(async () => { root.unmount(); });
console.log("tauri project navigation and legacy title backfill: OK");
