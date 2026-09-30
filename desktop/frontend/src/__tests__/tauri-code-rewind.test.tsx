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
  __conversationRewindPreviewHandler?: () => Promise<object>;
  __conversationRewindCommitHandler?: () => Promise<object>;
  __conversationRewindUndoHandler?: () => Promise<object>;
  __sessionHeadsHandler?: () => Promise<object[]>;
  __sessionHeadSwitchHandler?: (_sessionId: string, headId: string) => Promise<void>;
  __combinedRewindPreviewHandler?: () => Promise<object>;
  __combinedRewindCommitHandler?: () => Promise<object>;
  __tauriHistoryMessages?: Array<{ role: string; content: string }>;
  __historyGate?: Promise<void>;
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
state.__tauriHistoryMessages = [
  { role: "system", content: "system" }, { role: "user", content: "start" },
  { role: "assistant", content: "first answer" }, { role: "user", content: "edit two files" },
  { role: "assistant", content: "later answer" },
];
state.__conversationRewindPreviewHandler = async () => ({ planId: "plan-conversation", turn: 1, canConversation: true });
state.__conversationRewindCommitHandler = async () => {
  state.__tauriHistoryMessages = state.__tauriHistoryMessages?.slice(0, 3);
  return { ok: true, conversationForked: true, headId: "rewind-head" };
};
state.__conversationRewindUndoHandler = async () => {
  state.__tauriHistoryMessages = [
    { role: "system", content: "system" }, { role: "user", content: "start" },
    { role: "assistant", content: "first answer" }, { role: "user", content: "edit two files" },
    { role: "assistant", content: "later answer" },
  ];
  return { ok: true, conversationForked: false };
};

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
state.__conversationRewindPreviewHandler = async () => ({ turn: 1, canConversation: false, disabledReason: "legacy session format" });
await act(async () => { click(button("Rewind conversation")); await settle(); });
check(document.body.textContent?.includes("This conversation cannot be rewound here"), "unsupported session should explain the disabled action");
check(!button("Create conversation version"), "unsupported session must not offer commit");
state.__conversationRewindPreviewHandler = async () => ({ planId: "plan-conversation", turn: 1, canConversation: true });
await act(async () => { click(button("Rewind conversation")); await settle(); });
check(document.body.textContent?.includes("Workspace files stay as they are"), "conversation preview should explain file scope");
check(!state.__tauriBridgeCalls?.some(call => call.name === "bridge_conversation_rewind_commit"), "conversation preview must not commit");
await act(async () => { click(button("Create conversation version")); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_conversation_rewind_commit" && call.args.planId === "plan-conversation"), "conversation commit should use previewed plan");
check(!document.body.textContent?.includes("later answer"), "conversation rewind should refresh the visible history");
await act(async () => { click(button("Return to previous conversation")); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_conversation_rewind_undo" && call.args.headId === "rewind-head"), "conversation undo should target the committed head");
check(document.body.textContent?.includes("later answer"), "conversation undo should refresh the original history");
let selectedHead = "main";
state.__sessionHeadsHandler = async () => [
  { id: "main", kind: "main", preview: "later answer", messageCount: 5, selected: selectedHead === "main" },
  { id: "rewind-head", kind: "rewind", preview: "continued version", messageCount: 4, selected: selectedHead === "rewind-head" },
];
state.__sessionHeadSwitchHandler = async (_sessionId, headId) => {
  selectedHead = headId;
  state.__tauriHistoryMessages = headId === "main"
    ? [{ role: "system", content: "system" }, { role: "user", content: "start" }, { role: "assistant", content: "first answer" }, { role: "user", content: "edit two files" }, { role: "assistant", content: "later answer" }]
    : [{ role: "system", content: "system" }, { role: "user", content: "start" }, { role: "assistant", content: "first answer" }, { role: "user", content: "continued version" }];
};
await act(async () => { click(button("刷新")); await settle(); });
check(document.body.textContent?.includes("continued version"), "version list should show saved heads");
await act(async () => { click(document.querySelector('[aria-label="Conversation versions"] li:nth-child(2) button')); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_session_head_switch" && call.args.headId === "rewind-head"), "version switch should target a head ID");
check(!document.querySelector(".tauri-transcript")?.textContent?.includes("later answer"), "version switch should replace the visible transcript");
await act(async () => { click(document.querySelector('[aria-label="Conversation versions"] li:first-child button')); await settle(); await settle(); });
check(document.body.textContent?.includes("later answer"), "switching back should restore the original transcript");
state.__combinedRewindPreviewHandler = async () => ({ planId: "plan-both", turn: 1, canFiles: true, canConversation: true, fileCount: 2, files: ["a.txt", "b.txt"], filesTruncated: false, coverageGaps: ["bash_side_effect"], requiresCoverageConfirmation: true, conflicts: [] });
state.__combinedRewindCommitHandler = async () => {
  state.__tauriHistoryMessages = state.__tauriHistoryMessages?.slice(0, 3);
  selectedHead = "combined-head";
  return { ok: true, partial: false, conversationForked: true, filesRestored: true, headId: "combined-head", transactionId: "tx-both", undoAvailable: true, writtenCount: 2, deletedCount: 0, conflicts: [] };
};
await act(async () => { click(button("Rewind both")); await settle(); });
check(button("Rewind files and conversation")?.disabled, "combined rewind requires coverage confirmation");
await act(async () => { document.querySelector<HTMLInputElement>('[aria-label="Review files and conversation"] input[type="checkbox"]')?.click(); await settle(); });
await act(async () => { click(button("Rewind files and conversation")); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_combined_rewind_commit" && call.args.planId === "plan-both" && call.args.confirmPartialCoverage === true), "combined commit should use confirmed plan");
check(!document.querySelector(".tauri-transcript")?.textContent?.includes("later answer"), "combined rewind should refresh conversation");
check(Boolean(button("Undo combined rewind")), "combined result should offer its own undo");
state.__workspaceFileRevertUndoHandler = async () => {
  selectedHead = "main";
  state.__tauriHistoryMessages = [{ role: "system", content: "system" }, { role: "user", content: "start" }, { role: "assistant", content: "first answer" }, { role: "user", content: "edit two files" }, { role: "assistant", content: "later answer" }];
  return { ok: true, undoAvailable: false, writtenCount: 2, deletedCount: 0 };
};
await act(async () => { click(button("Undo combined rewind")); await settle(); await settle(); });
check(state.__tauriBridgeCalls?.some(call => call.name === "bridge_workspace_file_revert_undo" && call.args.transactionId === "tx-both"), "combined undo should target its file transaction");
check(document.querySelector(".tauri-transcript")?.textContent?.includes("later answer"), "combined undo should refresh original history");
state.__combinedRewindCommitHandler = async () => {
  state.__tauriHistoryMessages = state.__tauriHistoryMessages?.slice(0, 3);
  return { ok: true, partial: true, conversationForked: true, filesRestored: false, headId: "partial-head", undoAvailable: false, writtenCount: 0, deletedCount: 0, conflicts: ["file_changed"], error: "file conflicts detected" };
};
await act(async () => { click(button("Rewind both")); await settle(); });
await act(async () => { document.querySelector<HTMLInputElement>('[aria-label="Review files and conversation"] input[type="checkbox"]')?.click(); await settle(); });
await act(async () => { click(button("Rewind files and conversation")); await settle(); await settle(); });
check(document.body.textContent?.includes("Conversation version created, but file restore did not complete"), "partial commit should explain the uncertain file result");
check(!button("Undo combined rewind"), "partial commit must not offer a file transaction undo");
state.__historyGate = Promise.reject(new Error("history unavailable"));
void state.__historyGate.catch(() => {});
await act(async () => { click(button("Rewind conversation")); await settle(); });
await act(async () => { click(button("Create conversation version")); await settle(); await settle(); });
check(!document.querySelector(".tauri-transcript")?.textContent?.includes("later answer"), "failed history refresh must not keep the old conversation visible");
check(document.body.textContent?.includes("history unavailable"), "failed history refresh should explain the missing transcript");
state.__historyGate = undefined;
await act(async () => { root.unmount(); });
process.stdout.write("tauri code rewind UI: passed\n");
