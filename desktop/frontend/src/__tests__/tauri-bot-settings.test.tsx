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
  const { TauriBotSettings } = await import("../tauri/TauriBotSettings");
  const root = createRoot(document.getElementById("root")!);
  await act(async () => { root.render(React.createElement(LocaleProvider, null, React.createElement(TauriBotSettings))); await settle(); });
  ok(document.body.textContent?.includes("Degraded"), "runtime health is rendered from the bridge status");
  ok(document.body.textContent?.includes("feishu-lark") && document.body.textContent?.includes("4 messages received"), "adapter identity and counters are visible");
  ok(document.body.textContent?.includes("not connected to desktop session control"), "unsupported desktop command bridge is disclosed");
  const checkboxes = [...document.querySelectorAll<HTMLInputElement>("input[type=checkbox]")];
  ok(checkboxes.length === 5 && !checkboxes[0].checked && !checkboxes[1].checked && !checkboxes[2].checked && checkboxes[3].checked && !checkboxes[4].checked, "gateway, global and connection access policies, and channel preferences are shown separately");
  await act(async () => { checkboxes[0].click(); await settle(); });
  const calls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string }[] }).__tauriBridgeCalls;
  ok(calls.some(call => call.name === "bot_runtime_status"), "settings reads live status through the Tauri bridge");
  ok(calls.some(call => call.name === "change_bot_settings"), "gateway toggle persists through the authenticated bridge action");
  await act(async () => { checkboxes[1].click(); await settle(); });
  const accessCalls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: { change?: { action?: string; platform?: string; channelId?: string; list?: string; values?: string[]; enabled?: boolean } } }[] }).__tauriBridgeCalls;
  ok(accessCalls.some(call => call.name === "change_bot_settings" && call.args?.change?.action === "set_pairing" && call.args.change.enabled === true), "pairing policy is saved through the authenticated bridge");
  const firstAllowlist = document.querySelector<HTMLTextAreaElement>("textarea");
  if (firstAllowlist) typeInto(firstAllowlist, "user-1\nuser-2");
  await act(async () => { click("Save list"); await settle(); });
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
  await act(async () => { checkboxes[4].click(); await settle(); });
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
