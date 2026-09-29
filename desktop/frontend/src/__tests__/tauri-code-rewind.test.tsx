// Run: node --import ./scripts/svg-stub-register.mjs --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-code-rewind.test.tsx
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
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };

const state = globalThis as typeof globalThis & {
  __workbenchSessions?: Array<{ sessionId: string; title: string; workspaceRoot: string }>;
  __workspaceCheckpointsHandler?: () => Promise<object[]>;
  __codeRewindPreviewHandler?: () => Promise<object>;
  __codeRewindCommitHandler?: () => Promise<object>;
  __workspaceFileRevertUndoHandler?: () => Promise<object>;
  __tauriBridgeCalls?: Array<{ name: string; args: Record<string, unknown> }>;
};
state.__workbenchSessions = [{ sessionId: "tauri-code-rewind", title: "回滚测试", workspaceRoot: "/tmp/ws" }];
state.__workspaceCheckpointsHandler = async () => [{ turn: 1, prompt: "edit two files", time: 1, turnFileCount: 2 }];
state.__codeRewindPreviewHandler = async () => ({
  planId: "plan-code", turn: 1, canFiles: true, fileCount: 2, files: ["a.txt", "b.txt"], filesTruncated: false,
  coverage: "partial", coverageGaps: ["bash_side_effect"], requiresCoverageConfirmation: true, conflicts: [],
});
state.__codeRewindCommitHandler = async () => ({ ok: true, transactionId: "tx-code", undoAvailable: true, writtenCount: 2, deletedCount: 0 });
state.__workspaceFileRevertUndoHandler = async () => ({ ok: true, undoAvailable: false, writtenCount: 2, deletedCount: 0 });

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const settle = () => new Promise<void>(resolve => setTimeout(resolve, 0));
const click = (element: Element | null | undefined) => element?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
const button = (label: string) => [...document.querySelectorAll<HTMLButtonElement>("button")].find(item => item.textContent?.trim() === label);
function check(condition: unknown, message: string) {
  if (!condition) throw new Error(message);
}

await act(async () => { root.render(React.createElement(TauriSessionApp)); await settle(); await settle(); });
await act(async () => { click([...document.querySelectorAll(".tauri-session-row")].find(row => row.textContent?.includes("回滚测试"))?.querySelector(".tauri-sidebar__session")); await settle(); await settle(); });
await act(async () => { click(document.querySelector('.tauri-workspace-tree-button')); await settle(); });
await act(async () => { click(button("Checkpoints")); await settle(); });
check(document.body.textContent?.includes("edit two files"), "checkpoint list should show the prompt preview");
await act(async () => { click([...document.querySelectorAll(".tauri-workspace-change-entry")].find(row => row.textContent?.includes("edit two files"))); await settle(); });
check(document.body.textContent?.includes("a.txt") && document.body.textContent?.includes("b.txt"), "preview should list affected files");
check(button("Restore listed files")?.disabled, "partial coverage requires an explicit checkbox");
check(!state.__tauriBridgeCalls?.some(call => call.name === "bridge_code_rewind_commit"), "preview must not commit");
await act(async () => {
  const checkbox = document.querySelector<HTMLInputElement>('.tauri-workspace-revert__confirmation input');
  check(checkbox, "coverage confirmation should be shown");
  checkbox!.click();
  await settle();
});
check(!button("Restore listed files")?.disabled, "confirmation should enable commit");
await act(async () => { click(button("Restore listed files")); await settle(); await settle(); });
const commit = state.__tauriBridgeCalls?.find(call => call.name === "bridge_code_rewind_commit");
check(commit?.args.confirmPartialCoverage === true && commit.args.planId === "plan-code", "commit should send the confirmed plan");
check(document.body.textContent?.includes("Workspace files restored"), "success should be visible");
await act(async () => { click(button("Undo workspace restore")); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_workspace_file_revert_undo" && call.args.transactionId === "tx-code"), "undo should target the committed transaction");
await act(async () => { root.unmount(); });
process.stdout.write("tauri code rewind UI: passed\n");
