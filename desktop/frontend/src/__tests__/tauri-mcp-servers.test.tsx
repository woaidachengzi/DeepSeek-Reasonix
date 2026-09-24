// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-mcp-servers.test.tsx
//
// Drives the MCP server panel through the real DOM: add a server, verify the
// credential value never comes back, edit it with the credential field left
// empty, and delete it. The panel is reachable because the diagnostics overlay
// stays mounted.

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

function text(): string {
  return document.body.textContent ?? "";
}

/** Buttons inside the MCP card whose label matches. */
function mcpButtons(label: string): HTMLButtonElement[] {
  const cards = [...document.querySelectorAll<HTMLElement>(".tauri-diagnostic-card")];
  const card = cards.find(entry => entry.querySelector("h3")?.textContent === "MCP 服务器");
  if (!card) return [];
  return [...card.querySelectorAll<HTMLButtonElement>("button")].filter(
    button => (button.textContent ?? "").trim() === label,
  );
}

function click(button: HTMLButtonElement | undefined) {
  if (!button) return false;
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return true;
}

/** The panel form input identified by its aria-label. */
function field(label: string): HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null {
  const match = document.querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(
    `[aria-label="${label}"]`,
  );
  return match ?? null;
}

function typeInto(element: HTMLInputElement | HTMLTextAreaElement | null, value: string) {
  if (!element) return false;
  const setter = Object.getOwnPropertyDescriptor(
    element instanceof dom.window.HTMLTextAreaElement
      ? dom.window.HTMLTextAreaElement.prototype
      : dom.window.HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(element, value);
  element.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  return true;
}

const bridgeCalls = (): Array<{ name: string; args: unknown }> =>
  (globalThis as unknown as { __tauriBridgeCalls: Array<{ name: string; args: unknown }> }).__tauriBridgeCalls;

const SECRET = "renderer-must-never-see-this";

async function main() {
  (globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [];
  (globalThis as unknown as { __mcpServers: unknown[] }).__mcpServers = [];

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

  // The diagnostics overlay only mounts while it is open.
  const openDiagnostics = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    button => (button.textContent ?? "").includes("运行状态"),
  );
  ok(click(openDiagnostics), "the diagnostics panel can be opened");
  await act(async () => {
    await settle();
    await settle();
  });

  console.log("\ntauri MCP servers — panel");
  ok(text().includes("MCP 服务器"), "the diagnostics panel has an MCP servers card");
  ok(text().includes("还没有配置 MCP 服务器"), "an empty profile says so");

  // Add: a stdio server with a credential value.
  ok(click(mcpButtons("添加")[0]), "the add button is present");
  await act(async () => {
    await settle();
  });
  await act(async () => {
    typeInto(field("MCP 服务器名称") as HTMLInputElement, "time");
    typeInto(field("MCP 服务器命令") as HTMLInputElement, "uvx");
    typeInto(field("MCP 服务器参数") as HTMLInputElement, "mcp-server-time");
    typeInto(field("MCP 服务器环境变量") as HTMLTextAreaElement, `TIMEZONE=Asia/Shanghai\nAPI_TOKEN=${SECRET}`);
  });
  await act(async () => {
    click(mcpButtons("添加服务器")[0]);
    await settle();
    await settle();
  });

  const saved = bridgeCalls().find(call => call.name === "save_mcp_server");
  ok(saved !== undefined, "the form reached the save command");
  const savedServer = (saved?.args as { server?: { name?: string; env?: Record<string, string>; scope?: string } } | undefined)?.server;
  eq(savedServer?.name, "time", "the save carries the server name");
  eq(savedServer?.env?.API_TOKEN, SECRET, "the credential the user typed is submitted once");
  ok(!text().includes(SECRET), "the credential is never rendered back");
  ok(text().includes("time"), "the saved server appears in the list");
  ok(text().includes("API_TOKEN"), "the list names the credential key it expects");

  // Edit: the credential field starts empty and an omitted credential keeps the
  // stored one at the bridge.
  ok(click(mcpButtons("编辑")[0]), "the edit button is present");
  await act(async () => {
    await settle();
  });
  const envField = field("MCP 服务器环境变量") as HTMLTextAreaElement | null;
  eq(envField?.value, "", "editing never prefills a credential");
  const nameField = field("MCP 服务器名称") as HTMLInputElement | null;
  eq(nameField?.value, "time", "editing prefills the server name");
  ok(nameField?.disabled, "the name is fixed while editing");

  await act(async () => {
    typeInto(field("MCP 服务器参数") as HTMLInputElement, "mcp-server-time --local-timezone=Asia/Tokyo");
  });
  await act(async () => {
    click(mcpButtons("保存修改")[0]);
    await settle();
    await settle();
  });
  const edits = bridgeCalls().filter(call => call.name === "save_mcp_server");
  eq(edits.length, 2, "the edit reached the save command");
  const editServer = (edits[1]?.args as { server?: { env?: Record<string, string>; args?: string[] } } | undefined)?.server;
  eq(editServer?.env, undefined, "an untouched credential field is omitted, so the stored value stays");
  ok((editServer?.args ?? []).some(arg => arg.includes("Asia/Tokyo")), "the edited arguments are submitted");

  // Delete.
  ok(click(mcpButtons("删除")[0]), "the delete button is present");
  await act(async () => {
    await settle();
    await settle();
  });
  ok(bridgeCalls().some(call => call.name === "delete_mcp_server"), "the delete reached the delete command");
  ok(text().includes("还没有配置 MCP 服务器"), "the list is empty again");

  await act(async () => {
    root.unmount();
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

await main();
