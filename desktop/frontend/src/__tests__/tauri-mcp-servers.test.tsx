// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-mcp-servers.test.tsx
//
// Drives the MCP settings tab through the real DOM: add a server, verify the
// credential value never comes back, edit it with the credential field left
// empty, and delete it.

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
Object.defineProperty(dom.window.navigator, "language", { value: "zh-CN", configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
dom.window.confirm = () => true;

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

/** Buttons inside the MCP settings tab whose label matches. */
function mcpButtons(label: string): HTMLButtonElement[] {
  const section = document.querySelector<HTMLElement>(".tauri-mcp-settings");
  if (!section) return [];
  return [...section.querySelectorAll<HTMLButtonElement>("button")].filter(
    button => (button.textContent ?? "").trim() === label,
  );
}

function click(button: HTMLElement | undefined) {
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
  const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
  const { TauriMCPSettings } = await import("../tauri/TauriMCPSettings");
  const { mcpDraftFromMarketplace, mcpDraftToInput } = await import("../tauri/tauriMCPServers");
  const packageDraft = mcpDraftFromMarketplace({name:"io.example/package",suggestedName:"package",installable:true,transport:"stdio",command:"npx",args:["--yes","name with spaces"]}, [], "global");
  eq(JSON.stringify(mcpDraftToInput(packageDraft).args), JSON.stringify(["--yes","name with spaces"]), "Registry package argument boundaries survive the reviewed form");

  const root = createRoot(document.getElementById("root")!);
  await act(async () => {
    root.render(React.createElement(TauriSessionApp));
  });
  await act(async () => {
    await settle();
    await settle();
  });

  const openSettings = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    button => ["设置", "Settings"].includes((button.textContent ?? "").trim()),
  );
  await act(async () => { ok(click(openSettings), "the settings panel can be opened"); });
  await act(async () => { ok(click([...document.querySelectorAll<HTMLButtonElement>(".tauri-settings-nav-item")].find(button => ["MCP 与工具", "MCP & Tools"].includes(button.textContent?.trim() ?? ""))), "the MCP tab can be opened"); });
  await act(async () => {
    await settle();
    await settle();
  });

  console.log("\ntauri MCP servers — panel");
  ok(text().includes("MCP 服务器"), "the settings panel has an MCP servers tab");
  ok(text().includes("尚未配置服务器"), "an empty profile says so");

  // Add: a stdio server with a credential value.
  await act(async () => {
    ok(click(mcpButtons("添加")[0]), "the add button is present");
    await settle();
  });
  await act(async () => {
    typeInto(field("名称") as HTMLInputElement, "time");
    typeInto(field("命令") as HTMLInputElement, "uvx");
    typeInto(field("参数") as HTMLInputElement, "mcp-server-time");
    typeInto(field("环境变量") as HTMLTextAreaElement, `TIMEZONE=Asia/Shanghai\nAPI_TOKEN=${SECRET}`);
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

  // Activation must be an independent mutation, including a visible state
  // change, without rewriting the server or any write-only credential.
  await act(async () => {
    ok(click(mcpButtons("已启用")[0]), "the activation switch is present");
    await settle();
  });
  const activation = bridgeCalls().find(call => call.name === "set_mcp_server_enabled");
  eq((activation?.args as { name?: string; enabled?: boolean })?.enabled, false, "the switch sends disabled to the bridge");
  ok(text().includes("已停用"), "the effective disabled state is visible");
  ok(!text().includes(SECRET), "activation does not expose the credential");

  // Edit: the credential field starts empty and an omitted credential keeps the
  // stored one at the bridge.
  await act(async () => {
    ok(click(mcpButtons("编辑")[0]), "the edit button is present");
    await settle();
  });
  const envField = field("环境变量") as HTMLTextAreaElement | null;
  eq(envField?.value, "", "editing never prefills a credential");
  const nameField = field("名称") as HTMLInputElement | null;
  eq(nameField?.value, "time", "editing prefills the server name");
  ok(nameField?.disabled, "the name is fixed while editing");

  await act(async () => {
    typeInto(field("参数") as HTMLInputElement, "mcp-server-time --local-timezone=Asia/Tokyo");
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
  ok(text().includes("已停用"), "editing keeps the activation state");

  // Delete.
  await act(async () => {
    ok(click(mcpButtons("删除")[0]), "the delete button is present");
    await settle();
    await settle();
  });
  ok(bridgeCalls().some(call => call.name === "delete_mcp_server"), "the delete reached the delete command");
  ok(text().includes("尚未配置服务器"), "the list is empty again");

  await act(async () => {
    ok(click(mcpButtons("浏览目录")[0]), "the official directory can be opened");
    await settle();
  });
  ok(text().includes("Example remote server"), "a Registry result is shown before installation");
  await act(async () => {
    ok(click(mcpButtons("配置")[0]), "an installable Registry entry can be selected");
    await settle();
  });
  ok(bridgeCalls().some(call => call.name === "resolve_mcp_marketplace"), "the entry is re-resolved before preparing a draft");
  eq((field("名称") as HTMLInputElement | null)?.value, "remote", "the new draft uses the suggested local name");
  eq((field("URL") as HTMLInputElement | null)?.value, "https://mcp.example.test/mcp", "the new draft contains the resolved URL");
  await act(async () => {
    ok(click(mcpButtons("添加服务器")[0]), "the reviewed Registry draft can be saved");
    await settle();
  });
  ok(text().includes("remote"), "the Registry server appears in the installed list");

  (globalThis as unknown as { __mcpServers: Array<Record<string, unknown>> }).__mcpServers =
    (globalThis as unknown as { __mcpServers: Array<Record<string, unknown>> }).__mcpServers.map(server => server.name === "remote"
      ? { ...server, runtimeStatus: "connected", toolCount: 1, toolList: [{ name: "search", description: "Search remote data" }] }
      : server);
  await act(async () => {
    click(mcpButtons("刷新")[0]);
    await settle();
  });
  ok(text().includes("本会话已连接 · 1 项工具"), "the active session Host status is displayed");
  await act(async () => {
    click(document.querySelector<HTMLElement>(".tauri-mcp-list__tools summary") ?? undefined);
  });
  ok(Boolean(document.querySelector<HTMLDetailsElement>(".tauri-mcp-list__tools")?.open), "the Host tool details expand");
  ok(text().includes("Search remote data"), "the current Host tool description is available in details");

  await act(async () => {
    root.unmount();
  });

  const { LocaleProvider, useI18n } = await import("../lib/i18n");
  function ForceEnglish() {
    const { setPref } = useI18n();
    React.useEffect(() => setPref("en"), [setPref]);
    return null;
  }
  (globalThis as unknown as { __mcpServers: unknown[] }).__mcpServers = [];
  const englishRoot = createRoot(document.getElementById("root")!);
  await act(async () => {
    englishRoot.render(React.createElement(LocaleProvider, null,
      React.createElement(React.Fragment, null, React.createElement(ForceEnglish), React.createElement(TauriMCPSettings)),
    ));
    await settle();
    await settle();
  });
  ok(text().includes("MCP servers") && text().includes("No servers configured yet."), "the complete MCP settings page follows the selected English locale");
  await act(async () => { englishRoot.unmount(); });

  (globalThis as unknown as { __mcpServers: Array<Record<string, unknown>> }).__mcpServers = [{
    name: "remote", enabled: true, type: "http", source: "user_config", scope: "global",
    configPath: "/tmp/config.toml", url: "https://mcp.example.test/mcp", runtimeStatus: "failed", errorKind: "connection", nativeOAuthEligible: true, authenticationSaved: true,
  }];
  const activeRoot = createRoot(document.getElementById("root")!);
  await act(async () => {
    activeRoot.render(React.createElement(LocaleProvider, null,
      React.createElement(React.Fragment, null, React.createElement(ForceEnglish), React.createElement(TauriMCPSettings, { sessionId: "active-session", currentSessionState: "idle" })),
    ));
    await settle();
  });
  await act(async () => {
    ok(click(mcpButtons("Authorize in browser")[0]), "eligible Streamable HTTP servers offer browser authorization");
    await settle();
  });
  const authStart = bridgeCalls().find(call => call.name === "start_mcp_oauth");
  const authRequest = (authStart?.args as { request?: { sessionId?: string; name?: string } } | undefined)?.request;
  eq(authRequest?.sessionId, "active-session", "OAuth start is scoped to the displayed session ID");
  eq(authRequest?.name, "remote", "OAuth start identifies the selected server");
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 1250)); await settle(); });
  ok(text().includes("Authorization saved."), "completed browser authorization refreshes settings without auto-connecting");
  await act(async () => {
    ok(click(mcpButtons("Clear saved auth")[0]), "saved static or OAuth credentials can be cleared explicitly");
    await settle();
    await settle();
  });
  const authClear = bridgeCalls().find(call => call.name === "clear_mcp_authentication");
  const clearRequest = (authClear?.args as { request?: { sessionId?: string; name?: string } } | undefined)?.request;
  eq(clearRequest?.sessionId, "active-session", "credential clearing is scoped to the displayed session ID");
  eq(clearRequest?.name, "remote", "credential clearing targets only the selected server");
  await act(async () => {
    ok(click(mcpButtons("Connect to current session")[0]), "a failed enabled server can be connected to the active session");
    await settle();
    await settle();
  });
  const runtimeAction = bridgeCalls().find(call => call.name === "mcp_runtime_action");
  const runtimeRequest = (runtimeAction?.args as { request?: { sessionId?: string; name?: string; action?: string } } | undefined)?.request;
  eq(runtimeRequest?.sessionId, "active-session", "the runtime operation is scoped to the displayed session ID");
  eq(runtimeRequest?.action, "connect", "connect is separate from the persisted activation toggle");
  ok(text().includes("Connected to this session · 2 tools"), "connecting refreshes the active session status and tool count");
  await act(async () => {
    ok(click(mcpButtons("Disconnect this session")[0]), "a connected MCP can be disconnected from only this session");
    await settle();
    await settle();
  });
  const runtimeActions = bridgeCalls().filter(call => call.name === "mcp_runtime_action");
  eq((runtimeActions[1]?.args as { request?: { action?: string } } | undefined)?.request?.action, "disconnect", "disconnect is an explicit runtime action");
  ok(!text().includes("Connected to this session · 2 tools"), "disconnect refreshes the current session status");
  await act(async () => { activeRoot.unmount(); });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

await main();
