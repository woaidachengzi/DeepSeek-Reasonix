// Run with the Tauri, CSS, and SVG stub loaders used by test:tauri.
// An unfinished rename must remain discoverable when the bounded host catalog
// contains no row for its ID. Opening or confirming deletion is explicit.
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
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };

function settle(): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, 0));
}

async function main() {
  const id = "older-than-recent-catalog";
  const ambiguousID = "ambiguous-older-title";
  const workspaceRoot = "/tmp/old-project";
  const state = globalThis as unknown as {
    __workbenchSessions: unknown[];
    __pendingSessionTitleRecoveries: unknown[];
    __recoveredTitles: Record<string, string>;
    __tauriBridgeCalls: Array<{ name: string; args: unknown }>;
  };
  state.__workbenchSessions = [];
  state.__pendingSessionTitleRecoveries = [
    { id, title: "Before rename", workspaceRoot, state: "ready" },
    { id: ambiguousID, title: "Ambiguous rename", workspaceRoot, state: "ready" },
  ];
  state.__recoveredTitles = { [id]: "After rename" };

  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
  const root = createRoot(document.getElementById("root")!);
  await act(async () => {
    root.render(React.createElement(TauriSessionApp));
    await settle();
    await settle();
  });

  const recovery = document.querySelector<HTMLElement>('section[aria-label="待恢复标题"]');
  if (!recovery?.textContent?.includes("Before rename") ||
      !recovery.textContent.includes("标题待恢复")) {
    throw new Error("pending title recovery is not visible outside the recent catalog");
  }
  const open = recovery.querySelector<HTMLButtonElement>(".tauri-sidebar__session");
  if (!open || open.disabled) throw new Error("pending title recovery cannot be explicitly opened");
  await act(async () => {
    open.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
    await settle();
    await settle();
  });
  const opened = state.__tauriBridgeCalls.find(call => call.name === "bridge_switch_session" &&
    (call.args as { sessionId?: string }).sessionId === id);
  if (!opened || (opened.args as { workspaceRoot?: string }).workspaceRoot !== workspaceRoot) {
    throw new Error("explicit recovery did not open the original ID and workspace");
  }
  const afterOpen = document.querySelector<HTMLElement>('section[aria-label="待恢复标题"]');
  if (!afterOpen || afterOpen.textContent?.includes("Before rename") || !afterOpen.textContent?.includes("Ambiguous rename")) {
    throw new Error("completed title recovery was not removed while the unresolved one stayed visible");
  }
  const deleteButton = afterOpen.querySelector<HTMLButtonElement>('.tauri-session-row__delete');
  if (!deleteButton) throw new Error("ambiguous pending title has no explicit delete action");
  await act(async () => {
    deleteButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
    await settle();
  });
  const confirmation = document.querySelector<HTMLElement>('.tauri-session-delete__question');
  if (!confirmation?.textContent?.includes("放弃未完成的改名")) {
    throw new Error("pending-title deletion does not explain that the rename will be abandoned");
  }
  const confirmDelete = document.querySelector<HTMLButtonElement>('.tauri-session-delete__confirm');
  if (!confirmDelete) throw new Error("pending-title deletion is missing confirmation");
  await act(async () => {
    confirmDelete.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
    await settle();
    await settle();
  });
  if (state.__tauriBridgeCalls.some(call => call.name === "bridge_switch_session" &&
      (call.args as { sessionId?: string }).sessionId === ambiguousID)) {
    throw new Error("ambiguous pending title was reopened before deletion");
  }
  if (!state.__tauriBridgeCalls.some(call => call.name === "bridge_delete_session" &&
      (call.args as { sessionId?: string }).sessionId === ambiguousID)) {
    throw new Error("confirmed pending-title deletion did not reach the bridge");
  }
  if (document.querySelector('section[aria-label="待恢复标题"]')) {
    throw new Error("deleted pending title remains in the independent list");
  }
  await act(async () => root.unmount());
  process.stdout.write("pending title recovery outside the recent catalog passed\n");
}

main().catch(error => {
  process.stderr.write(`${error}\n`);
  process.exitCode = 1;
});
