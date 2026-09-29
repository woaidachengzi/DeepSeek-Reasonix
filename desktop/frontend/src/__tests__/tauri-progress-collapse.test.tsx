// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window), cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window), localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
(globalThis as unknown as { __workbenchSessions: unknown[] }).__workbenchSessions = [
  { sessionId: "progress-session", title: "检查提交" },
  { sessionId: "second-session", title: "第二个会话" },
];
(globalThis as unknown as { __tauriHistoryMessages: unknown[] }).__tauriHistoryMessages = [
  { role: "user", content: "检查提交" },
  { role: "assistant", content: "先查看工作区", workDurationMs: 5_000 },
  { role: "assistant", content: "发现两个提交", workDurationMs: 70_000 },
  { role: "assistant", content: "结论：需要测试", workDurationMs: 960_000 },
];

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 0)); });
const shell = document.querySelector<HTMLElement>(".tauri-shell");
assert.ok(shell);
assert.equal(shell.hasAttribute("data-sidebar-hidden"), false, "sidebar is visible by default");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "b", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(shell.getAttribute("data-sidebar-hidden"), "true", "the stable sidebar shortcut hides the navigation");
assert.equal(localStorage.getItem("tauri-sidebar-visible"), "off", "sidebar visibility persists");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "b", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(shell.hasAttribute("data-sidebar-hidden"), false, "the stable sidebar shortcut restores the navigation");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.ok(document.querySelector('[role="dialog"]'), `Ctrl+K opens the command palette (${document.body.textContent?.slice(-500) ?? ""})`);
const paletteAppearance = [...document.querySelectorAll<HTMLButtonElement>(".palette__chip")].find(button => /外观|Appearance/.test(button.textContent ?? ""));
assert.ok(paletteAppearance, "the command palette offers direct appearance settings navigation");
await act(async () => { paletteAppearance.click(); });
assert.match(document.querySelector('.tauri-settings-nav-item[aria-current="page"]')?.textContent?.trim() ?? "", /外观|Appearance/, "the palette opens the requested settings page");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); });
const paletteDiagnostics = [...document.querySelectorAll<HTMLButtonElement>(".palette__chip")].find(button => /运行状态|Diagnostics/.test(button.textContent ?? ""));
assert.ok(paletteDiagnostics);
await act(async () => { paletteDiagnostics.click(); });
assert.equal(document.querySelector(".tauri-settings-overlay"), null, "opening diagnostics from the palette closes settings so the drawer is visible");
assert.ok(document.querySelector(".tauri-diagnostics"));
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-diagnostics__header [aria-label="关闭"]')?.click(); });
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "?", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.ok(document.querySelector(".shortcuts-cheatsheet"), "the stable shortcut-help chord opens the Preview-specific shortcut sheet");
assert.equal(document.querySelectorAll(".shortcuts-cheatsheet__row").length, 42, "the help sheet lists exactly the configurable Preview actions");
await act(async () => { document.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true })); });
assert.equal(document.querySelector(".shortcuts-cheatsheet"), null, "Escape closes keyboard help");
const sidebarToggle = document.querySelector<HTMLButtonElement>(".tauri-sidebar-toggle");
assert.ok(sidebarToggle, "the top bar keeps a control available to restore a hidden sidebar");
await act(async () => { sidebarToggle.click(); });
assert.equal(shell.getAttribute("data-sidebar-hidden"), "true", "the top-bar control hides the sidebar");
assert.equal(sidebarToggle.getAttribute("aria-pressed"), "true", "the top-bar control exposes its current state");
await act(async () => { sidebarToggle.click(); });
assert.equal(shell.hasAttribute("data-sidebar-hidden"), false, "the top-bar control restores the sidebar");
const sessionButton = document.querySelector<HTMLButtonElement>(".tauri-sidebar__session");
assert.ok(sessionButton);
assert.equal(sessionButton.disabled, false, document.body.textContent ?? "");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "=", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(document.documentElement.getAttribute("data-text-size"), "large", "the stable increase-text shortcut applies immediately");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "-", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(document.documentElement.hasAttribute("data-text-size"), false, "the stable decrease-text shortcut applies immediately");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "=", ctrlKey: true, bubbles: true, cancelable: true })); });
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "0", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(document.documentElement.hasAttribute("data-text-size"), false, "the stable reset-text shortcut restores the default");
await act(async () => {
  sessionButton.click();
  await new Promise(resolve => setTimeout(resolve, 0));
});
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "2", ctrlKey: true, bubbles: true, cancelable: true })); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.ok((globalThis as unknown as { __tauriBridgeCalls: Array<{ name: string; args?: { sessionId?: string } }> }).__tauriBridgeCalls
  .some(call => call.name === "bridge_switch_session" && call.args?.sessionId === "second-session"), "Ctrl+2 opens the second loaded conversation through the host bridge");

const progress = document.querySelector<HTMLDetailsElement>(".tauri-progress");
assert.ok(progress, "intermediate updates render inside a progress disclosure");
assert.equal(progress.open, false, "progress is collapsed by default");
assert.match(progress.querySelector("summary")?.textContent ?? "", /用时 16 分钟.*2 条过程更新/);
assert.match(progress.textContent ?? "", /先查看工作区.*发现两个提交/);
assert.ok([...document.querySelectorAll(".tauri-message.is-assistant")].some(message =>
  !progress.contains(message) && message.textContent?.includes("结论：需要测试")), "the final answer stays outside the disclosure");
await act(async () => { progress.querySelector("summary")?.click(); });
assert.equal(progress.open, true, "the user can expand the progress disclosure");
const openSettings = [...document.querySelectorAll<HTMLButtonElement>(".tauri-sidebar__diagnostics")].find(button => /设置|Settings/.test(button.textContent ?? ""));
assert.ok(openSettings);
await act(async () => { openSettings.click(); });
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); });
const paletteFiles = [...document.querySelectorAll<HTMLButtonElement>(".palette__chip")].find(button => /文件|Files/.test(button.textContent ?? ""));
assert.ok(paletteFiles);
await act(async () => { paletteFiles.click(); });
assert.equal(Boolean(document.querySelector(".tauri-settings-overlay")), false, "opening files from the palette closes settings so the drawer is visible");
assert.ok(document.querySelector(".tauri-workspace-drawer"));
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-workspace-drawer__header [aria-label="关闭"]')?.click(); });
await act(async () => { openSettings.click(); });
const progressOptions = () => document.querySelector<HTMLElement>('[aria-label="Session experience"]') ?? document.querySelector<HTMLElement>('[aria-label="会话体验"]');
assert.ok(progressOptions(), "settings expose the conversation experience preference");
await act(async () => { progressOptions()?.querySelectorAll<HTMLButtonElement>("button")[1]?.click(); });
assert.equal(localStorage.getItem("tauri-progress-mode"), "deep", "deep mode persists");
assert.equal(document.querySelector<HTMLDetailsElement>(".tauri-progress")?.open, true, "deep mode expands the current process group");
await act(async () => { document.querySelector<HTMLDetailsElement>(".tauri-progress")?.querySelector("summary")?.click(); });
assert.equal(document.querySelector<HTMLDetailsElement>(".tauri-progress")?.open, false, "deep mode still permits manual folding");
await act(async () => { progressOptions()?.querySelectorAll<HTMLButtonElement>("button")[0]?.click(); });
assert.equal(document.querySelector<HTMLDetailsElement>(".tauri-progress")?.open, false, "standard mode returns to folded process groups");
const statusBar = () => document.querySelector<HTMLElement>(".tauri-statusbar");
assert.ok(statusBar(), "Preview displays its live workspace status bar");
assert.match(statusBar()?.textContent ?? "", /本地服务|Local service/);
const statusStyle = document.querySelector<HTMLElement>('[aria-label="Bottom status bar style"]') ?? document.querySelector<HTMLElement>('[aria-label="底部信息栏样式"]');
assert.ok(statusStyle, "General settings control the visible status bar");
await act(async () => { statusStyle.querySelectorAll<HTMLButtonElement>("button")[0]?.click(); });
assert.ok(statusBar()?.classList.contains("is-icon"), "style updates the workspace immediately");
const statusItemsToggle = document.querySelector<HTMLButtonElement>(".status-bar-items-editor__toggle");
assert.ok(statusItemsToggle);
assert.equal(statusItemsToggle.getAttribute("aria-expanded"), "false", "item editing starts folded like the stable settings page");
await act(async () => { statusItemsToggle.click(); });
const bridgeItem = document.querySelector<HTMLInputElement>('[data-statusbar-setting-item="bridge"] input');
assert.ok(bridgeItem);
await act(async () => { bridgeItem.click(); });
assert.equal(statusBar()?.querySelector('[title^="本地服务："], [title^="Local service:"]'), null, "hiding an item removes its real status value");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.status-bar.v1") ?? "null")?.style, "icon", "status bar preference persists");
const shortcutsNav = [...document.querySelectorAll<HTMLButtonElement>(".tauri-settings-nav-item")].find(button => /快捷键|Shortcuts/.test(button.textContent ?? ""));
assert.ok(shortcutsNav);
await act(async () => { shortcutsNav.click(); });
const settingsShortcut = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="settings"]');
assert.ok(settingsShortcut);
const commandPaletteShortcut = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="command_palette"]');
assert.ok(commandPaletteShortcut, "the command palette shortcut is configurable from settings");
assert.match(commandPaletteShortcut.textContent ?? "", /Ctrl\+K/, "the command palette shows the stable Ctrl+K default on Linux");
await act(async () => { settingsShortcut.click(); });
await act(async () => { settingsShortcut.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "j", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "j");
await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-settings-back")?.click(); });
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: ",", ctrlKey: true, bubbles: true, cancelable: true })); });
assert.equal(document.querySelector(".tauri-settings-overlay"), null, "replaced shortcut no longer opens settings");
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "j", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.ok(document.querySelector(".tauri-settings-overlay"), "new shortcut opens settings in the live workspace");
const shortcutNavAgain = [...document.querySelectorAll<HTMLButtonElement>(".tauri-settings-nav-item")].find(button => /快捷键|Shortcuts/.test(button.textContent ?? ""));
assert.ok(shortcutNavAgain);
await act(async () => { shortcutNavAgain.click(); });
await act(async () => { window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "p", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.match(document.querySelector('.tauri-settings-nav-item[aria-current="page"]')?.textContent?.trim() ?? "", /模型服务|Model services/, "a configurable shortcut opens its matching settings page");
const shortcutsAfterDirectNav = [...document.querySelectorAll<HTMLButtonElement>(".tauri-settings-nav-item")].find(button => /快捷键|Shortcuts/.test(button.textContent ?? ""));
assert.ok(shortcutsAfterDirectNav);
await act(async () => { shortcutsAfterDirectNav.click(); });
const newSessionRecorder = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="new_session"]');
assert.ok(newSessionRecorder);
await act(async () => { newSessionRecorder.click(); });
await act(async () => { newSessionRecorder.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "q", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.match(document.querySelector(".tauri-sidebar__new kbd")?.textContent ?? "", /Ctrl\+Shift\+Q/, "new-session hint follows the saved shortcut");
const sendRecorder = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="send_message"]');
assert.ok(sendRecorder);
await act(async () => { sendRecorder.click(); });
await act(async () => { sendRecorder.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.match(document.querySelector<HTMLTextAreaElement>(".tauri-composer textarea")?.placeholder ?? "", /Ctrl\+Shift\+Enter/, "composer hint follows the saved send shortcut");
await act(async () => { root.unmount(); });
console.log("tauri progress disclosure: OK");
