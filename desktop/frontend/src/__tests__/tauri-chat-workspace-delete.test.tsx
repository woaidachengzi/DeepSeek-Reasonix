// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-chat-workspace-delete.test.tsx
//
// Mounts the Tauri chat workspace in jsdom and clicks the real controls. The
// delete button was reported as doing nothing, so this test drives the actual
// DOM instead of re-implementing the flow: click the row's delete control, then
// the confirm button, and assert the adapter was reached and the row left the
// catalog.

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

  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { TauriSessionPreview } = await import("../tauri/TauriChatWorkspace");

  const root = createRoot(document.getElementById("root")!);
  await act(async () => {
    root.render(React.createElement(TauriSessionPreview));
  });
  await act(async () => {
    await settle();
    await settle();
  });

  console.log("\ntauri chat workspace — delete flow");
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
  ok(clickSession("保留会话"), "the other conversation can be opened first");
  await act(async () => {
    await settle();
    await settle();
  });

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

  await act(async () => {
    root.unmount();
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

await main();
