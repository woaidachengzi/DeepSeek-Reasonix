// Run: tsx src/__tests__/tauri-settings-api-key.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
  isTauri: true,
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

let keyPresent = false;
const envCredentialPresent = true;
let saveGate: Promise<void> | null = null;
let releaseSave: (() => void) | undefined;
let failSummary = false;
let failSave = false;
let failRuntime = false;
const calls: string[] = [];
let openedURL = "";
let closeBehavior = "keep_running";
let defaultModel = "";
const summary = () => ({
  protocolVersion: 1,
  defaultModel,
  providers: [{ name: "demo", displayName: "Demo", kind: "openai", modelCount: 1, models: ["m"], requiresKey: true, configured: keyPresent || envCredentialPresent }],
});

(dom.window as unknown as { __TAURI_INTERNALS__: { invoke: (command: string) => Promise<unknown> } }).__TAURI_INTERNALS__ = {
  async invoke(command: string, args?: { url?: string; behavior?: string; request?: { model?: string } }) {
    calls.push(command);
    switch (command) {
      case "preview_runtime_info":
        if (failRuntime) { failRuntime = false; throw new Error("runtime unavailable"); }
        return { previewVersion: "1", previewCommit: "unknown", previewDirty: false, stableVersion: "1", stableCommit: "test", tauriVersion: "2", bridgeProtocolVersion: 1, previewBuild: "test" };
      case "provider_summary":
        if (failSummary) { failSummary = false; throw new Error("summary unavailable"); }
        return summary();
      case "set_default_model": defaultModel = args?.request?.model ?? ""; return summary();
      case "platform_info": return "darwin";
      case "get_close_behavior": return closeBehavior;
      case "set_close_behavior": closeBehavior = args?.behavior ?? closeBehavior; return closeBehavior;
      case "open_external_url": openedURL = args?.url ?? ""; return;
      case "keychain_save":
        if (failSave) throw new Error("secret-in-error-message");
        if (saveGate) await saveGate;
        keyPresent = true;
        return;
      case "keychain_delete": {
        const deleted = keyPresent;
        keyPresent = false;
        return deleted;
      }
      default: throw new Error(`unexpected command: ${command}`);
    }
  },
};

const React = await import("react");
const { act } = React;
const { renderToString } = await import("react-dom/server");
const { createRoot } = await import("react-dom/client");
const { TauriSettings } = await import("../tauri/TauriSettings");
let parentConfigured: boolean | undefined;
let applyCalls = 0;
let profileImportCalls = 0;
let restartCalls = 0;
let auditRefreshCalls = 0;

renderToString(<TauriSettings onClose={() => {}} />);
assert.equal(calls.length, 0, "rendering settings does not start bridge work");

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<TauriSettings initialTab="appearance" onClose={() => {}} onProviderSummaryChange={value => { parentConfigured = value.providers[0]?.configured; }} currentSessionState="idle" onApplyToCurrentSession={async () => { applyCalls += 1; return true; }} bridgeStatus={{ running: true, protocolVersion: 1 }} catalogAudit={{ legacyCount: 3, directoryCount: 3, matchedCount: 3, directoryOnlyCount: 0, missingFromDirectory: 0, retiredLegacyCount: 0, titleMismatches: 0, workspaceMismatches: 0, orderMismatches: 0, missingTranscripts: 0, physicalStateMismatches: 0, unclaimedTranscripts: 0, inventoryErrors: 0, legacyMatchesDirectory: true }} sessionPageSource="identity" onRestartBridge={async () => { restartCalls += 1; return true; }} onRefreshCatalogAudit={async () => { auditRefreshCalls += 1; }} profile={{ previewHome: "/preview/home", previewConfigExists: false, stableConfigExists: true, importAvailable: true, managedProfile: true }} onImportStableProfile={async () => { profileImportCalls += 1; return "已备份并导入配置"; }} />); });
assert.equal(calls.filter(call => call === "provider_summary").length, 1, "settings load once after mount");
assert.equal(parentConfigured, true, "the model picker outside settings receives the initial summary");

function click(label: string) {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(candidate => candidate.textContent?.trim() === label);
  assert.ok(button, `missing button: ${label}`);
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
}

function enterKey(value: string) {
  const input = document.querySelector<HTMLInputElement>(".tauri-settings-apikey input");
  assert.ok(input);
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(input, value);
  input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
}

function visibleText() { return document.body.textContent ?? ""; }

assert.match(visibleText(), /配色风格/, "appearance controls are available without opening the model tab");
assert.ok(document.querySelector(".tauri-settings-sidebar .tauri-settings-nav"), "settings use a full-page sidebar");
const settingsSearch = document.querySelector<HTMLInputElement>('input[aria-label="搜索设置"]');
assert.ok(settingsSearch, "the sidebar has settings search");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "快捷键");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
assert.equal(document.querySelectorAll(".tauri-settings-nav-item").length, 1, "search filters navigation items");
assert.match(document.querySelector(".tauri-settings-nav-item")?.textContent ?? "", /快捷键/);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("深色"); });
assert.equal(document.documentElement.getAttribute("data-theme"), "dark");
assert.equal(localStorage.getItem("tauri-theme"), "dark");
await act(async () => { click("跟随系统"); });
assert.equal(document.documentElement.hasAttribute("data-theme"), false);
await act(async () => { click("极光"); });
assert.equal(document.documentElement.getAttribute("data-theme-style"), "aurora");
assert.equal(localStorage.getItem("tauri-theme-style"), "aurora");
await act(async () => { click("宽屏"); });
assert.equal(document.documentElement.getAttribute("data-conversation-width"), "full");
assert.equal(localStorage.getItem("reasonix-conv-width"), "full");
await act(async () => { click("大"); });
assert.equal(document.documentElement.getAttribute("data-text-size"), "large");
assert.equal(localStorage.getItem("reasonix-text-size"), "large");
await act(async () => { click("数据"); });
assert.match(visibleText(), /\/preview\/home/, "data settings show the isolated preview directory");
await act(async () => { click("复制稳定版配置（先备份）"); });
assert.equal(profileImportCalls, 1);
assert.match(visibleText(), /已备份并导入配置/, "data settings surface the import result");
await act(async () => { click("通用"); });
await act(async () => { click("退出 Reasonix"); });
assert.equal(closeBehavior, "quit", "close-window behavior is saved through the native host");
assert.equal(document.querySelector('[aria-label="关闭窗口时"] [aria-checked="true"]')?.textContent, "退出 Reasonix");
const notificationToggle = document.querySelector<HTMLInputElement>(".tauri-settings-toggle input");
assert.ok(notificationToggle, "general settings include desktop notifications");
await act(async () => { notificationToggle.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
assert.equal(localStorage.getItem("tauri-desktop-notifications"), "off", "notification preference is saved");

await act(async () => { click("模型偏好"); });
const defaultModelSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-default-model");
assert.ok(defaultModelSelect, "default model has its own settings page");
await act(async () => {
  defaultModelSelect.value = "demo/m";
  defaultModelSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(defaultModel, "demo/m", "default model is persisted through the bridge");
await act(async () => { click("模型服务"); });
assert.match(visibleText(), /已就绪/, ".env credential configures the provider");
await act(async () => { enterKey("secret-input-value"); });

saveGate = new Promise(resolve => { releaseSave = resolve; });
await act(async () => {
  click("保存到钥匙串");
  click("保存到钥匙串");
  click("删除");
});
assert.equal(calls.filter(call => call === "keychain_save").length, 1, "repeated save starts one operation");
assert.equal(calls.filter(call => call === "keychain_delete").length, 0, "delete cannot race with save");
await act(async () => { releaseSave?.(); });
saveGate = null;
assert.match(visibleText(), /已保存到钥匙串/);
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "", "successful save clears the entered secret");
assert.equal(calls.filter(call => call === "provider_summary").length, 2, "save refreshes the provider summary");
assert.equal(parentConfigured, true, "the model picker outside settings receives the saved status");
assert.match(visibleText(), /应用到当前会话/, "saved key offers an explicit active-session update");
await act(async () => { click("应用到当前会话"); });
assert.equal(applyCalls, 1, "active-session update is user initiated");
assert.doesNotMatch(visibleText(), /应用到当前会话/, "successful update clears the pending action");

await act(async () => { click("删除"); });
assert.match(visibleText(), /钥匙串密钥已删除；其他凭据仍可用/, "delete reports the keychain scope when .env remains");
assert.match(visibleText(), /已就绪/, "refreshed provider remains configured by .env");
assert.equal(calls.filter(call => call === "provider_summary").length, 3, "delete refreshes the provider summary");
assert.equal(parentConfigured, true, "the model picker outside settings keeps the .env readiness");

await act(async () => { click("删除"); });
assert.match(visibleText(), /钥匙串中没有密钥/, "repeated delete does not claim a key was removed");

await act(async () => { enterKey("secret-input-value"); });
failSummary = true;
await act(async () => { click("保存到钥匙串"); });
assert.match(visibleText(), /已保存到钥匙串；配置状态刷新失败/, "refresh failure does not misreport a successful save");
assert.doesNotMatch(visibleText(), /secret-input-value|secret-in-error-message/, "status never includes secrets");

await act(async () => { enterKey("secret-input-value"); });
failSave = true;
await act(async () => { click("保存到钥匙串"); });
assert.match(visibleText(), /保存失败/);
assert.doesNotMatch(visibleText(), /secret-in-error-message/, "error details do not expose secrets");

await act(async () => { click("运行诊断"); });
assert.match(visibleText(), /持久身份目录/, "diagnostics show the actual sidebar data source");
assert.match(visibleText(), /运行中 · 协议 v1/, "diagnostics show bridge status");
await act(async () => { click("重新检查"); });
assert.equal(auditRefreshCalls, 1, "diagnostics refresh the catalog audit");
await act(async () => { click("重启桥接服务"); });
assert.equal(restartCalls, 1, "diagnostics restart the existing bridge");
assert.match(visibleText(), /桥接服务已重启/, "restart outcome is shown in settings");

await act(async () => { click("关于"); });
const githubLink = document.querySelector<HTMLAnchorElement>('.tauri-settings-about a[href^="https://github.com/"]')
  ?? document.querySelector<HTMLAnchorElement>('.tauri-settings-actions a[href^="https://github.com/"]');
assert.ok(githubLink, "About page has a GitHub link");
const githubClick = new dom.window.MouseEvent("click", { bubbles: true, cancelable: true });
await act(async () => { githubLink.dispatchEvent(githubClick); });
assert.equal(githubClick.defaultPrevented, true, "GitHub link does not navigate the WebView");
assert.equal(openedURL, "https://github.com/esengine/DeepSeek-Reasonix", "GitHub link opens through the native host");

await act(async () => { root.unmount(); });
failSummary = true;
const retryRoot = createRoot(document.getElementById("root")!);
await act(async () => { retryRoot.render(<TauriSettings onClose={() => {}} />); });
assert.match(visibleText(), /桌面风格/, "general settings remain available when provider loading fails");
await act(async () => { click("模型服务"); });
assert.match(visibleText(), /读取设置失败/, "failed provider loading is actionable");
await act(async () => { click("重试"); });
assert.match(visibleText(), /已就绪/, "retry recovers the model settings");
await act(async () => { click("关于"); });
failRuntime = true;
await act(async () => { click("刷新"); });
assert.match(visibleText(), /读取设置失败/, "runtime refresh failure is visible");
await act(async () => { click("模型服务"); });
assert.match(visibleText(), /已就绪/, "runtime failure does not block model settings");
await act(async () => { retryRoot.unmount(); });
console.log("tauri settings API key flow passed");
