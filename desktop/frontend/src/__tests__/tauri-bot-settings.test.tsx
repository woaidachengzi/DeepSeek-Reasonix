// Run: node --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-bot-settings.test.tsx
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

let passed = 0;
let failed = 0;
function ok(value: unknown, label: string) {
  if (value) { process.stdout.write(`  PASS  ${label}\n`); passed += 1; }
  else { process.stdout.write(`  FAIL  ${label}\n`); failed += 1; }
}
const settle = () => new Promise<void>(resolve => setTimeout(resolve, 0));
function typeInto(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const prototype = input instanceof dom.window.HTMLTextAreaElement ? dom.window.HTMLTextAreaElement.prototype : dom.window.HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
}
function click(label: string) {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(item => item.textContent?.trim() === label);
  button?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return Boolean(button);
}

async function main() {
  (globalThis as typeof globalThis & { __botRuntimeStatus?: unknown }).__botRuntimeStatus = {
    protocolVersion: 1, running: true, status: "degraded", message: "1 bot connection(s) running; 1 failed to start", connections: 1,
    startedAt: "2026-09-28T10:00:00Z", desktopBridgeAvailable: false,
    adapterHealth: [{ id: "feishu-lark", platform: "feishu", domain: "lark", status: "connected", messages: 4, sends: 2, send_errors: 1, last_error: "temporary send failure" }],
  };
  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { LocaleProvider } = await import("../lib/i18n");
  const { tauriBotSettings } = await import("../lib/tauriBridge");
  const { TauriBotSettings } = await import("../tauri/TauriBotSettings");
  await tauriBotSettings();
  const malformedSettings = globalThis as typeof globalThis & { __botSettings?: { allowlist: Record<string, Record<string, unknown>>; channels: { access?: Record<string, unknown> }[] } };
  malformedSettings.__botSettings!.allowlist.qq.users = null;
  malformedSettings.__botSettings!.channels[0].access!.users = null;
  const root = createRoot(document.getElementById("root")!);
  await act(async () => { root.render(React.createElement(LocaleProvider, null, React.createElement(TauriBotSettings))); await settle(); });
  ok((document.getElementById("bot-access-qq.users") as HTMLTextAreaElement | null)?.value === "", "a null legacy allowlist renders as an empty list instead of crashing settings");
  ok((document.getElementById("bot-channel-access-feishu-lark-users") as HTMLTextAreaElement | null)?.value === "", "a null connection-specific allowlist renders as an empty list instead of crashing settings");
  ok(document.body.textContent?.includes("Degraded"), "runtime health is rendered from the bridge status");
  ok(document.body.textContent?.includes("feishu-lark") && document.body.textContent?.includes("4 messages received"), "adapter identity and counters are visible");
  ok(document.body.textContent?.includes("not connected to desktop session control"), "unsupported desktop command bridge is disclosed");
  const checkboxes = [...document.querySelectorAll<HTMLInputElement>("input[type=checkbox]")];
  ok(checkboxes.length === 6 && !checkboxes[0].checked && !checkboxes[1].checked && !checkboxes[2].checked && checkboxes[3].checked && checkboxes[4].checked && !checkboxes[5].checked, "gateway, global and connection access policies, self-message filtering, and channel preferences are shown separately");
  await act(async () => { checkboxes[0].click(); await settle(); });
  const calls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string }[] }).__tauriBridgeCalls;
  ok(calls.some(call => call.name === "bot_runtime_status"), "settings reads live status through the Tauri bridge");
  ok(calls.some(call => call.name === "change_bot_settings"), "gateway toggle persists through the authenticated bridge action");
  const gatewayRuntime = [...document.querySelectorAll("details")].find(details => details.querySelector("summary")?.textContent?.includes("Gateway runtime settings"));
  const maxSteps = gatewayRuntime?.querySelector<HTMLInputElement>('input[type="number"]');
  if (maxSteps) typeInto(maxSteps, "42");
  await act(async () => { maxSteps?.dispatchEvent(new dom.window.FocusEvent("focusout", { bubbles: true })); await settle(); });
  const gatewayRuntimeCalls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: { change?: { action?: string; maxSteps?: number; queueMode?: string; pairingRequestTtlMinutes?: number; platform?: string; values?: string[] } } }[] }).__tauriBridgeCalls;
  ok(gatewayRuntimeCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_gateway_runtime" && call.args.change.maxSteps === 42), "gateway max steps saves through the authenticated Preview config bridge");
  const gatewaySelect = gatewayRuntime?.querySelector<HTMLSelectElement>("select");
  if (gatewaySelect) { gatewaySelect.value = "collect"; gatewaySelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); }
  await act(async () => { await settle(); });
  ok(gatewayRuntimeCalls.some(call => call.args?.change?.action === "set_gateway_runtime" && call.args.change.queueMode === "collect"), "gateway queue mode saves to the live runtime configuration");
  const selfIDs = gatewayRuntime?.querySelector<HTMLTextAreaElement>("#bot-self-id-qq");
  if (selfIDs) typeInto(selfIDs, "self-qq-1\nself-qq-2");
  const selfSave = selfIDs?.closest(".tauri-bot-access-field")?.querySelector<HTMLButtonElement>("button");
  await act(async () => { selfSave?.click(); await settle(); });
  ok(gatewayRuntimeCalls.some(call => call.args?.change?.action === "set_self_user_ids" && call.args.change.platform === "qq" && JSON.stringify(call.args.change.values) === JSON.stringify(["self-qq-1", "self-qq-2"])), "platform self IDs are normalized and saved independently");
  const routesPanel = [...document.querySelectorAll("details")].find(details => details.querySelector("summary")?.textContent?.includes("Bind chats to projects"));
  const addRoute = [...(routesPanel?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find(button => button.textContent?.trim() === "Add route");
  await act(async () => { addRoute?.click(); await settle(); });
  const routeCard = routesPanel?.querySelector<HTMLElement>(".tauri-bot-route");
  const routeSelects = [...(routeCard?.querySelectorAll<HTMLSelectElement>("select") ?? [])];
  const routeInputs = [...(routeCard?.querySelectorAll<HTMLInputElement>("input") ?? [])];
  await act(async () => {
    if (routeSelects[1]) { routeSelects[1].value = "feishu"; routeSelects[1].dispatchEvent(new dom.window.Event("change", { bubbles: true })); }
    if (routeSelects[2]) { routeSelects[2].value = "group"; routeSelects[2].dispatchEvent(new dom.window.Event("change", { bubbles: true })); }
    if (routeInputs[0]) typeInto(routeInputs[0], "group-42");
    if (routeInputs[3]) typeInto(routeInputs[3], "/tmp/route-workspace");
    await settle();
  });
  const saveRoutes = [...(routesPanel?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find(button => button.textContent?.trim() === "Save route changes");
  await act(async () => { saveRoutes?.click(); await settle(); });
  const routeCall = [...gatewayRuntimeCalls].reverse().find(call => call.args?.change?.action === "set_routes");
  const savedRoute = (routeCall?.args?.change as { routes?: { platform: string; chatType: string; chatId: string; workspaceRoot: string }[] } | undefined)?.routes?.[0];
  ok(savedRoute?.platform === "feishu" && savedRoute.chatType === "group" && savedRoute.chatId === "group-42" && savedRoute.workspaceRoot === "/tmp/route-workspace", "chat route match and outputs save together through the authenticated bridge");
  await act(async () => { checkboxes[1].click(); await settle(); });
  const accessCalls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: { change?: { action?: string; platform?: string; channelId?: string; list?: string; values?: string[]; enabled?: boolean } } }[] }).__tauriBridgeCalls;
  ok(accessCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_pairing" && call.args.change.enabled === true), "pairing policy is saved through the authenticated bridge");
  const firstAllowlist = document.getElementById("bot-access-qq.users") as HTMLTextAreaElement | null;
  if (firstAllowlist) typeInto(firstAllowlist, "user-1\nuser-2");
  const firstAllowlistSave = firstAllowlist?.closest(".tauri-bot-access-field")?.querySelector<HTMLButtonElement>("button");
  await act(async () => { firstAllowlistSave?.click(); await settle(); });
  const allowlistCall = [...accessCalls].reverse().find(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_allowlist");
  ok(allowlistCall?.args?.change?.platform === "qq" && allowlistCall.args.change.list === "users" && JSON.stringify(allowlistCall.args.change.values) === JSON.stringify(["user-1", "user-2"]), "allowlist editor saves normalized user IDs");
  const runtimePanel = [...document.querySelectorAll("details")].find(details => details.querySelector("summary")?.textContent?.includes("Runtime settings"));
  runtimePanel?.querySelector("summary")?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  const runtimeSelects = [...(runtimePanel?.querySelectorAll<HTMLSelectElement>("select") ?? [])];
  if (runtimeSelects[1]) { runtimeSelects[1].value = "auto"; runtimeSelects[1].dispatchEvent(new dom.window.Event("change", { bubbles: true })); }
  await act(async () => { await settle(); });
  const runtimeCalls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: { change?: { action?: string; channelId?: string; toolApprovalMode?: string } } }[] }).__tauriBridgeCalls;
  ok(runtimeCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_channel_runtime" && call.args.change.channelId === "feishu-lark" && call.args.change.toolApprovalMode === "auto"), "connection tool approval mode is persisted through the bridge");
  const workspaceInput = runtimePanel?.querySelector<HTMLInputElement>(".tauri-bot-runtime-field input");
  if (workspaceInput) typeInto(workspaceInput, "/tmp/bot-project");
  await act(async () => { workspaceInput?.dispatchEvent(new dom.window.FocusEvent("focusout", { bubbles: true })); await settle(); });
  const workspaceCalls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: { change?: { action?: string; channelId?: string; workspaceRoot?: string } } }[] }).__tauriBridgeCalls;
  ok(workspaceCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_channel_runtime" && call.args.change.workspaceRoot === "/tmp/bot-project"), "connection workspace root saves on blur");
  await act(async () => { checkboxes[5].click(); await settle(); });
  ok(accessCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_channel_pairing" && call.args.change.channelId === "feishu-lark" && call.args.change.enabled), "connection-specific pairing policy is saved");
  const channelUserList = document.querySelector<HTMLTextAreaElement>("#bot-channel-access-feishu-lark-users");
  if (channelUserList) typeInto(channelUserList, "channel-user");
  const channelAccess = channelUserList?.closest("details");
  const channelSave = [...(channelAccess?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find(button => button.textContent?.trim() === "Save list");
  await act(async () => { channelSave?.click(); await settle(); });
  ok(accessCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_channel_allowlist" && call.args.change.channelId === "feishu-lark" && call.args.change.list === "users" && call.args.change.values?.[0] === "channel-user"), "connection-specific access list is saved separately from the global list");
  const identity = document.querySelector<HTMLInputElement>('[aria-label="App ID / account ID · Feishu"]');
  const secret = document.querySelector<HTMLInputElement>('[aria-label="Bot secret / token · Feishu"]');
  if (identity) typeInto(identity, "app-id");
  if (secret) typeInto(secret, "hidden-bot-secret");
  await act(async () => { click("Save credentials"); await settle(); });
  const credentialCall = [...calls].reverse().find(call => call.name === "change_bot_settings") as { args?: { change?: { action?: string; identity?: string; secret?: string } } } | undefined;
  ok(credentialCall?.args?.change?.action === "set_credentials" && credentialCall.args.change.secret === "hidden-bot-secret", "credential form sends the secret only with the save request");
  ok(!document.body.textContent?.includes("hidden-bot-secret") && secret?.value === "", "saved bot credential is cleared and never echoed into the page");
  await act(async () => { root.unmount(); });
  process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exitCode = 1;
}

await main();
