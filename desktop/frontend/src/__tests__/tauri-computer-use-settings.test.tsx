import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { ComputerUsePermission, ComputerUseStatus } from "../lib/tauriComputerUse";
import type { TauriSandboxSettings } from "../lib/tauriBridge";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
localStorage.setItem("reasonix-lang", "en");
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriComputerUseSettings } = await import("../tauri/TauriComputerUseSettings");
const { LocaleProvider } = await import("../lib/i18n");
const root = createRoot(document.getElementById("root")!);
const tick = () => new Promise(resolve => setTimeout(resolve, 0));
let native: ComputerUseStatus = { protocolVersion: 1, platform: "darwin", available: true, accessibility: false, screenRecording: true };
const sandbox: TauriSandboxSettings = { protocolVersion: 1, bash: "enforce", network: false, workspaceRoot: "", allowWrite: [], platform: "darwin", shell: "auto", resolvedShell: "zsh", effectiveWriteRoots: [], effectiveRootsError: "" };
const opened: ComputerUsePermission[] = [];
const navigated: string[] = [];
let fail = false;
let oldRequest: ((value: ComputerUseStatus) => void) | undefined;
let heldStatus: Promise<ComputerUseStatus> | undefined;
let permissionGate: Promise<void> | undefined;
const reads: Array<[string | undefined, string | undefined]> = [];
const api = {
  status: async () => { if (fail) throw new Error("native unavailable"); return heldStatus ?? native; },
  sandbox: async (workspaceRoot?: string, sessionId?: string) => { reads.push([workspaceRoot, sessionId]); return sandbox; },
  openPermission: async (permission: ComputerUsePermission) => { opened.push(permission); await permissionGate; },
};
const render = async (workspaceRoot = "/qa/a") => {
  await act(async () => { root.render(<LocaleProvider><TauriComputerUseSettings workspaceRoot={workspaceRoot} sessionId="session-a" onOpenSandbox={() => navigated.push("sandbox")} onOpenPermissions={() => navigated.push("permissions")} onOpenModels={() => navigated.push("model")} api={api} /></LocaleProvider>); await tick(); });
};
const button = (label: string) => [...document.querySelectorAll<HTMLButtonElement>("button")].find(b => (b.getAttribute("aria-label") || b.textContent) === label)!;
const click = async (label: string) => { const b = button(label); assert(b, label); await act(async () => { b.click(); await tick(); }); };

try {
  await render();
  assert.deepEqual(reads, [["/qa/a", "session-a"]]);
  assert.equal(opened.length, 0, "opening the page must not prompt or change system access");
  assert(document.body.textContent?.includes("Enforced isolation currently blocks computer control."));
  assert(document.body.textContent?.includes("Not granted"));
  assert(document.querySelector('[role="note"]')?.textContent?.includes("already enabled"));
  await click("Sandbox settings"); await click("Manage confirmations"); await click("Model settings");
  assert.deepEqual(navigated, ["sandbox", "permissions", "model"]);
  await click("Open Accessibility / device control settings");
  assert.deepEqual(opened, ["accessibility"]);
  assert(document.body.textContent?.includes("Not granted"), "opening OS settings must not imply permission was granted");
  native = { ...native, accessibility: true };
  await click("Refresh status");
  assert(!document.body.textContent?.includes("Not granted"));
  assert.equal(document.querySelector('[role="note"]'), null, "recovery guidance is hidden when native permissions are ready");

  fail = true; await click("Refresh status");
  assert(document.querySelector('[role="alert"]')?.textContent?.includes("native unavailable"));
  assert(!document.body.textContent?.includes("Included with the app"), "failed reads must not retain stale readiness");
  fail = false; native = { ...native, available: false, platform: "linux" };
  await click("Refresh status");
  assert(document.body.textContent?.includes("GNU/Linux build with the Cua runtime"));
  assert(!button("Open Screen recording settings"), "Linux must not show macOS permission controls");
  native = { ...native, available: true, linux: { x11: true, atspi: false, wayland: true, wayland_enabled: false, xsend_event: true } };
  await click("Refresh status");
  assert(document.body.textContent?.includes("X11 / XWayland"));
  assert(document.body.textContent?.includes("AT-SPI"));
  assert(document.body.textContent?.includes("only supported XWayland windows"));
  assert(!document.body.textContent?.includes("Screen recording"));
  assert.equal(opened.length, 1, "Linux readiness must not open OS permission settings");
  native = { ...native, linux: { ...native.linux!, gnome_shell_major: 50 } };
  await click("Refresh status");
  assert(document.body.textContent?.includes("GNOME 50"));
  assert(!document.body.textContent?.includes("Native Wayland support is off"), "unsupported compositor must not suggest simply enabling its unsafe helper");

  heldStatus = new Promise(resolve => { oldRequest = resolve; });
  await render("/qa/old");
  heldStatus = undefined; native = { ...native, available: true, platform: "darwin" };
  await render("/qa/current");
  await act(async () => { oldRequest!({ ...native, available: false }); await tick(); });
  assert(document.body.textContent?.includes("Included with the app"), "late workspace reads must not overwrite the current view");
  let release!: () => void;
  permissionGate = new Promise(resolve => { release = resolve; });
  await click("Open Screen recording settings");
  assert(button("Open Screen recording settings").disabled);
  await render("/qa/next");
  assert(!button("Open Screen recording settings").disabled, "old permission requests must not lock a new workspace");
  await act(async () => { release(); await tick(); });
  assert(!document.body.textContent?.includes("System Settings opened."), "late permission completion must not update another workspace");
  console.log("PASS computer control settings: read-only load, sandbox navigation, native permissions, refresh/recovery, unsupported platform, stale requests");
} finally { await act(async () => root.unmount()); dom.window.close(); }
