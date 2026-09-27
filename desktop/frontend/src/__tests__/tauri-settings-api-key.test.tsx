// Run: tsx src/__tests__/tauri-settings-api-key.test.tsx
import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { JSDOM } from "jsdom";

registerHooks({ resolve(specifier, context, nextResolve) {
  if (specifier.endsWith(".css")) return nextResolve("./asset-stub-for-tests.ts", { ...context, parentURL: import.meta.url });
  return nextResolve(specifier, context);
} });

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
class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
globalThis.ResizeObserver = TestResizeObserver as unknown as typeof ResizeObserver;

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
let approvalMode = "auto";
let defaultModel = "";
const permissions = { protocolVersion: 1, mode: "ask", allow: [] as string[], ask: [] as string[], deny: [] as string[] };
let sandbox = { protocolVersion: 1, bash: "enforce", network: true, workspaceRoot: "", allowWrite: [] as string[], platform: "darwin" };
let network = { protocolVersion: 1, proxyMode: "auto", noProxy: "", proxyType: "socks5", proxyServer: "", proxyPort: 0, proxyUsername: "", proxyUrlSet: false, proxyPasswordSet: false };
let skills = { protocolVersion: 1, allowImplicitInvocation: true, skills: [{ name: "demo", description: "A test skill", invocation: "/demo", scope: "global", sourcePath: "/tmp/demo/SKILL.md", runAs: "inline", enabled: true }], sources: [] as { path: string; scope: string; status: string; enabled: boolean; configured: boolean }[] };
let subagents = { protocolVersion: 1, defaultModel: "demo/m", subagentModel: "", subagentEffort: "", maxDepth: 2, maxConcurrency: 6, maxParallelWriters: 3, modelRefs: ["demo/m"], modelEfforts: { "demo/m": ["auto", "low", "high"] }, profiles: [{ name: "reviewer", description: "Reviews code", scope: "global", invocation: "/reviewer", configuredModel: "", configuredEffort: "" }] };
let hooks = { protocolVersion: 1, scope: "global", path: "/preview/settings.json", projectRoot: "", revision: "r1", hooks: {} as Record<string, unknown>, events: ["PreToolUse", "Stop"] };
let memory = { protocolVersion: 1, workspaceRoot: "/preview/project", storeDir: "/preview/memory", globalStoreDir: "/preview/global-memory", docs: [{ path: "/preview/project/AGENTS.md", scope: "project", body: "Old instruction.\n", revision: "r1" }], facts: [] as unknown[], archives: [] as unknown[], diagnostics: [] as string[] };
let memoryNote = "";
let savedProviderInput: { name: string; displayName: string; kind: string; baseUrl: string; models: string[]; default: string; useApiKey: boolean } | undefined;
const summary = () => ({
  protocolVersion: 1,
  defaultModel,
  providers: [{ name: "demo", displayName: "Demo", kind: "openai", modelCount: 1, models: ["m"], requiresKey: true, configured: keyPresent || envCredentialPresent }],
});

(dom.window as unknown as { __TAURI_INTERNALS__: { invoke: (command: string) => Promise<unknown> } }).__TAURI_INTERNALS__ = {
  async invoke(command: string, args?: { scope?: string; workspaceRoot?: string; url?: string; behavior?: string; mode?: string; request?: { model?: string }; input?: { name: string; displayName: string; kind: string; baseUrl: string; models: string[]; default: string; useApiKey: boolean }; change?: { action?: string; enabled?: boolean; name?: string; path?: string; mode?: string; list?: "allow" | "ask" | "deny"; rule?: string; bash?: string; network?: boolean; workspaceRoot?: string; allowWrite?: string[]; proxyMode?: string; noProxy?: string; proxyType?: string; proxyServer?: string; proxyPort?: number; proxyUsername?: string; proxyUrlAction?: string; proxyPasswordAction?: string; value?: string; number?: number; revision?: string; hooks?: Record<string, unknown>; scope?: string; body?: string } }) {
    calls.push(command);
    switch (command) {
      case "preview_runtime_info":
        if (failRuntime) { failRuntime = false; throw new Error("runtime unavailable"); }
        return { previewVersion: "1", previewCommit: "unknown", previewDirty: false, stableVersion: "1", stableCommit: "test", tauriVersion: "2", bridgeProtocolVersion: 1, previewBuild: "test" };
      case "provider_summary":
        if (failSummary) { failSummary = false; throw new Error("summary unavailable"); }
        return summary();
      case "provider_configs": return { protocolVersion: 1, providers: [{ name: "demo", displayName: "Demo", kind: "openai", models: ["m"], default: "m" }] };
      case "save_provider_config": savedProviderInput = args?.input; return { protocolVersion: 1, providers: [args?.input] };
      case "usage_stats": return { protocolVersion: 1, from: "2026-09-01", to: "2026-09-27", tokens: 42, requests: 1, turns: 1, cacheHit: 0, cacheMiss: 42, activeDays: 1, topModel: "demo/m", topProvider: "demo", daily: [], models: [], providers: [] };
      case "permission_settings": return { ...permissions };
      case "change_permission_settings": {
        const change = args?.change;
        if (change?.action === "mode") permissions.mode = change.mode ?? permissions.mode;
        else if (change?.list && change.rule) {
          if (change.action === "add") permissions[change.list] = [...permissions[change.list], change.rule];
          else permissions[change.list] = permissions[change.list].filter(rule => rule !== change.rule);
        }
        return { ...permissions };
      }
      case "sandbox_settings": return { ...sandbox };
      case "change_sandbox_settings": sandbox = { ...sandbox, ...args?.change }; return { ...sandbox };
      case "network_settings": return { ...network };
      case "change_network_settings": network = { ...network, ...args?.change }; return { ...network };
      case "skills_settings": return { ...skills };
      case "change_skills_settings": {
        if (args?.change?.action === "implicit") skills.allowImplicitInvocation = Boolean(args.change.enabled);
        if (args?.change?.action === "skill" && args.change.name) skills.skills = skills.skills.map(item => item.name === args.change?.name ? { ...item, enabled: Boolean(args.change?.enabled) } : item);
        return { ...skills };
      }
      case "subagent_settings": return { ...subagents };
      case "change_subagent_settings": {
        if (args?.change?.action === "depth") subagents.maxDepth = args.change.number ?? subagents.maxDepth;
        if (args?.change?.action === "model") subagents.subagentModel = args.change.value ?? subagents.subagentModel;
        return { ...subagents };
      }
      case "hooks_settings": return { ...hooks, scope: args?.scope ?? "global" };
      case "change_hooks_settings": hooks = { ...hooks, revision: "r2", hooks: args?.change?.hooks ?? {} }; return { ...hooks };
      case "memory_settings": return { ...memory };
      case "change_memory_settings": {
        if (args?.change?.action === "save_doc") memory = { ...memory, docs: memory.docs.map(doc => doc.path === args.change?.path ? { ...doc, body: args.change?.body ?? "", revision: "r2" } : doc) };
        if (args?.change?.action === "quick_add") memoryNote = args.change.body ?? "";
        return { ...memory };
      }
      case "set_default_model": defaultModel = args?.request?.model ?? ""; return summary();
      case "platform_info": return "darwin";
      case "get_close_behavior": return closeBehavior;
      case "set_close_behavior": closeBehavior = args?.behavior ?? closeBehavior; return closeBehavior;
      case "desktop_preferences": return { protocolVersion: 1, defaultToolApprovalMode: approvalMode };
      case "set_desktop_approval": approvalMode = args?.mode ?? approvalMode; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode };
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
const { LocaleProvider } = await import("../lib/i18n");
const { TauriSettings } = await import("../tauri/TauriSettings");
let parentConfigured: boolean | undefined;
let applyCalls = 0;
let profileImportCalls = 0;
let restartCalls = 0;
let auditRefreshCalls = 0;

renderToString(<LocaleProvider><TauriSettings onClose={() => {}} /></LocaleProvider>);
assert.equal(calls.length, 0, "rendering settings does not start bridge work");

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<LocaleProvider><TauriSettings initialTab="appearance" onClose={() => {}} onProviderSummaryChange={value => { parentConfigured = value.providers[0]?.configured; }} currentSessionState="idle" onApplyToCurrentSession={async () => { applyCalls += 1; return true; }} bridgeStatus={{ running: true, protocolVersion: 1 }} catalogAudit={{ legacyCount: 3, directoryCount: 3, matchedCount: 3, directoryOnlyCount: 0, missingFromDirectory: 0, retiredLegacyCount: 0, titleMismatches: 0, workspaceMismatches: 0, orderMismatches: 0, missingTranscripts: 0, physicalStateMismatches: 0, unclaimedTranscripts: 0, inventoryErrors: 0, legacyMatchesDirectory: true }} sessionPageSource="identity" onRestartBridge={async () => { restartCalls += 1; return true; }} onRefreshCatalogAudit={async () => { auditRefreshCalls += 1; }} profile={{ previewHome: "/preview/home", previewConfigExists: false, stableConfigExists: true, importAvailable: true, managedProfile: true }} onImportStableProfile={async () => { profileImportCalls += 1; return "已备份并导入配置"; }} /></LocaleProvider>); });
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
await act(async () => { click("Yolo"); });
assert.equal(approvalMode, "yolo", "new-session approval default is saved through the bridge");
assert.equal(document.querySelector('[aria-label="新会话默认审批"] [aria-checked="true"]')?.textContent, "Yolo");
const notificationToggle = document.querySelector<HTMLInputElement>(".tauri-settings-toggle input");
assert.ok(notificationToggle, "general settings include desktop notifications");
await act(async () => { notificationToggle.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
assert.equal(localStorage.getItem("tauri-desktop-notifications"), "off", "notification preference is saved");
const soundToggle = document.querySelector<HTMLButtonElement>('.tauri-settings-sound-toggle');
assert.ok(soundToggle);
await act(async () => { soundToggle.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
const successSelect = document.querySelector<HTMLSelectElement>('select[aria-label="回复完成提示音"]');
const attentionSelect = document.querySelector<HTMLSelectElement>('select[aria-label="需要回答提示音"]');
const volumeSlider = document.querySelector<HTMLInputElement>('input[aria-label="通知音量"]');
assert.ok(successSelect && attentionSelect && volumeSlider, "sound controls are available in General");
await act(async () => { successSelect.value = "synth"; successSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => { attentionSelect.value = "positive"; attentionSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(volumeSlider, "42");
  volumeSlider.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  volumeSlider.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(localStorage.getItem("notificationSoundSuccess"), "synth");
assert.equal(localStorage.getItem("notificationSoundAttention"), "positive");
assert.equal(localStorage.getItem("notificationSoundVolume"), "42");

await act(async () => { click("模型偏好"); });
const defaultModelSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-default-model");
assert.ok(defaultModelSelect, "default model has its own settings page");
await act(async () => {
  defaultModelSelect.value = "demo/m";
  defaultModelSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(defaultModel, "demo/m", "default model is persisted through the bridge");
await act(async () => { click("用量统计"); });
assert.ok(document.querySelector(".usage-stats"), "Preview renders the stable usage chart panel");
assert.ok(calls.includes("usage_stats"), "usage statistics read through the Tauri bridge");
await act(async () => { click("权限"); });
assert.match(visibleText(), /默认写入决策/, "permission editor is available in settings");
await act(async () => { click("拒绝"); });
assert.equal(permissions.mode, "deny", "writer fallback mode is persisted through the bridge");
const denyRuleInput = document.querySelector<HTMLInputElement>('input[aria-label="新增拒绝规则"]');
assert.ok(denyRuleInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(denyRuleInput, "Bash(rm:*)");
  denyRuleInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("添加"); });
assert.deepEqual(permissions.deny, ["Bash(rm:*)"], "permission rule is persisted through the bridge");
await act(async () => { click("沙盒"); });
assert.match(visibleText(), /Bash 沙盒/, "sandbox editor is available in settings");
await act(async () => { click("关闭"); });
await act(async () => { click("保存沙盒设置"); });
assert.equal(sandbox.bash, "off", "sandbox mode is persisted through the bridge");
await act(async () => { click("网络"); });
assert.match(visibleText(), /代理模式/, "network editor is available in settings");
await act(async () => { click("直连"); });
await act(async () => { click("保存网络设置"); });
assert.equal(network.proxyMode, "off", "proxy mode is persisted through the bridge");
await act(async () => { click("Agent Skills"); });
assert.match(visibleText(), /A test skill/, "discovered skills appear in settings");
const implicitSwitch = document.querySelector<HTMLInputElement>('input[aria-label="允许自动调用技能"]');
assert.ok(implicitSwitch);
await act(async () => { implicitSwitch.click(); });
assert.equal(skills.allowImplicitInvocation, false, "implicit skill invocation is persisted through the bridge");
await act(async () => { click("子智能体"); });
assert.match(visibleText(), /Reviews code/, "discoverable subagent profiles appear in settings");
await act(async () => { click("1 层"); });
assert.equal(subagents.maxDepth, 1, "subagent delegation depth is persisted through the bridge");
await act(async () => { click("Hooks"); });
assert.match(visibleText(), /配置文件/, "hooks editor displays its source path");
const hooksEditor = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Hooks JSON"]');
assert.ok(hooksEditor);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(hooksEditor, '{"hooks":{"Stop":[{"command":"echo done"}]}}');
  hooksEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存 Hooks"); });
assert.deepEqual(hooks.hooks, { Stop: [{ command: "echo done" }] }, "hooks JSON is saved through the bridge");
await act(async () => { click("模型服务"); });
assert.match(visibleText(), /已就绪/, ".env credential configures the provider");
assert.match(visibleText(), /服务配置/, "provider configuration is editable from settings");
await act(async () => { click("编辑"); });
assert.equal(document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="留空保留现有端点"]')?.value, "", "existing endpoint is not sent back to the WebView");
await act(async () => { click("取消"); });
await act(async () => { click("添加服务"); });
const providerNameInput = document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="例如 my-provider"]');
const providerURLInput = document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="https://example.com/v1"]');
const providerModelsInput = document.querySelector<HTMLTextAreaElement>('.tauri-provider-editor-form textarea');
assert.ok(providerNameInput && providerURLInput && providerModelsInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(providerNameInput, "new-provider");
  providerNameInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(providerURLInput, "https://provider.example/v1");
  providerURLInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(providerModelsInput, "chat-model");
  providerModelsInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存服务"); });
assert.equal(savedProviderInput?.name, "new-provider", "provider creation reaches the native command");
assert.equal(savedProviderInput?.models[0], "chat-model", "model IDs reach the native command");
assert.equal(savedProviderInput?.useApiKey, true, "remote services default to credential support");
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
assert.equal(calls.filter(call => call === "provider_summary").length, 3, "save refreshes the provider summary");
assert.equal(parentConfigured, true, "the model picker outside settings receives the saved status");
assert.match(visibleText(), /应用到当前会话/, "saved key offers an explicit active-session update");
await act(async () => { click("应用到当前会话"); });
assert.equal(applyCalls, 1, "active-session update is user initiated");
assert.doesNotMatch(visibleText(), /应用到当前会话/, "successful update clears the pending action");

await act(async () => { click("删除"); });
assert.match(visibleText(), /钥匙串密钥已删除；其他凭据仍可用/, "delete reports the keychain scope when .env remains");
assert.match(visibleText(), /已就绪/, "refreshed provider remains configured by .env");
assert.equal(calls.filter(call => call === "provider_summary").length, 4, "delete refreshes the provider summary");
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
await act(async () => { retryRoot.render(<LocaleProvider><TauriSettings onClose={() => {}} /></LocaleProvider>); });
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
const memoryRoot = createRoot(document.getElementById("root")!);
await act(async () => { memoryRoot.render(<LocaleProvider><TauriSettings initialTab="memory" workspaceRoot="/preview/project" onClose={() => {}} /></LocaleProvider>); });
assert.match(visibleText(), /Old instruction/, "memory document is read through the bridge");
const memoryEditor = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="记忆文档"]');
assert.ok(memoryEditor);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(memoryEditor, "New instruction.");
  memoryEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存文档"); });
assert.equal(memory.docs[0].body, "New instruction.", "memory document edit is persisted through the bridge");
const noteInput = document.querySelector<HTMLInputElement>('input[aria-label="记忆笔记"]');
assert.ok(noteInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(noteInput, "Remember this.");
  noteInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("追加"); });
assert.equal(memoryNote, "Remember this.", "quick memory note is saved through the bridge");
await act(async () => { memoryRoot.unmount(); });
console.log("tauri settings API key flow passed");
