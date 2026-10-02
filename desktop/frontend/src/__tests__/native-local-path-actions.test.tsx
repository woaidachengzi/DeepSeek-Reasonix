import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "https://reasonix.local/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
const host = globalThis as typeof globalThis & { isTauri?: boolean };
const win = window as unknown as { go?: unknown; __TAURI_INTERNALS__?: { invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown> } };
let wailsCalls = 0;
win.go = { main: { App: new Proxy({}, { get: () => async () => { wailsCalls++; } }) } };
const calls: { command: string; args?: Record<string, unknown> }[] = [];
let failCommand = "";
let failMessage = "cannot access document; check path permissions";
let savedPath = "";
host.isTauri = true;
win.__TAURI_INTERNALS__ = { invoke: async (command, args) => {
  calls.push({ command, args });
  if (command === failCommand) throw new Error(failMessage);
  if (command === "local_path_openers") return { openers: [{ id: "vscode", name: "VS Code", kind: "editor" }], preferred: "" };
  if (command === "save_local_path_as") return savedPath;
} };
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { RichMarkdownLink } = await import("../components/githubLink");
const { ToastProvider } = await import("../lib/toast");
const { app } = await import("../lib/bridge");
const root = createRoot(document.getElementById("root")!);
const source = "/tmp/中文 ' document.md";
await act(async () => { root.render(React.createElement(ToastProvider, null, React.createElement(RichMarkdownLink, { href: new URL(`file://${source}`).href, children: "Document" }))); });
const anchor = document.querySelector("a")!;
await act(async () => { anchor.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true })); });
assert.deepEqual(calls.at(-1), { command: "open_local_path", args: { path: source } });
await act(async () => { anchor.dispatchEvent(new dom.window.MouseEvent("auxclick", { button: 1, bubbles: true, cancelable: true })); });
assert.equal(calls.at(-1)?.command, "open_local_path", "middle click uses the host without WebView navigation");

async function select(label: string) {
  await act(async () => { anchor.dispatchEvent(new dom.window.MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 15, clientY: 15 })); });
  const action = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find(item => item.textContent?.includes(label));
  assert.ok(action, `missing native menu action: ${label}`);
  await act(async () => { action.click(); });
}
await select("VS Code");
assert.deepEqual(calls.at(-1), { command: "open_local_path_with", args: { path: source, id: "vscode" } }, "only an installed-app ID crosses the adapter");
await select("Reveal");
assert.deepEqual(calls.at(-1), { command: "reveal_local_path", args: { path: source } });
await select("Save as");
assert.deepEqual(calls.at(-1), { command: "save_local_path_as", args: { path: source } });
assert.equal(document.querySelector(".toast--info"), null, "canceled native save must not show success");
savedPath = "/tmp/target.md";
await select("Save as");
assert.ok(document.querySelector(".toast--info")?.textContent?.includes(savedPath), "completed native save reports the actual destination");
failCommand = "open_local_path";
await act(async () => { anchor.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true })); });
assert.ok(document.querySelector(".toast--error")?.textContent?.includes("check path permissions"), "default-open failures are visible");
failCommand = "save_local_path_as";
await assert.rejects(app.SaveLocalPathAs(source), /check path permissions/);
for (const [failure, expected] of [
  ["destination is the source file; choose another path", "Choose a different name or location"],
  ["destination is the same as the source", "Choose a different name or location"],
  ["cannot access document: No such file or directory (os error 2); check the path and permissions", "Check that it still exists"],
  ["cannot read source: Permission denied; check file permissions", "have read permission"],
  ['cannot create destination: Permission denied (os error 13) at path "/private/tmp/.tmp-secret"; choose a writable folder', "Choose a folder you have permission"],
  ["cannot save document: Operation not permitted (os error 1); choose a writable destination", "Choose a folder you have permission"],
  ["cannot save document: disk full", "Choose a writable location"],
] as const) {
  failMessage = failure;
  await select("Save as");
  const toast = [...document.querySelectorAll(".toast--error")].at(-1)?.textContent ?? "";
  assert.ok(toast.includes(expected), `Save As reports recovery for ${failure}`);
  assert.ok(!toast.includes(".tmp-secret"), "known write denial hides the internal temporary path");
  assert.ok(!toast.includes("Could not open"), "saving never uses the open-action failure label");
}
assert.equal(wailsCalls, 0, "native errors never fall back to a Wails binding or browser mock");
failCommand = "";
host.isTauri = false;
await app.OpenLocalPath(source);
assert.equal(wailsCalls, 1, "Wails binding still resolves after switching out of Tauri");
await act(async () => { root.unmount(); });
// Exercise the visible workspace chooser against the same native adapter.
host.isTauri = true;
const { ExternalOpener } = await import("../components/ExternalOpener");
const { tauriExternalOpenerBridge } = await import("../tauri/tauriExternalOpener");
let preferred = "finder";
win.__TAURI_INTERNALS__ = { invoke: async (command, args) => {
  calls.push({ command, args });
  if (command === failCommand) throw new Error("cannot save preference; check profile permissions");
  if (command === "workspace_external_openers") return { openers: [{ id: "finder", name: "Finder", kind: "file-manager" }, { id: "vscode", name: "VS Code", kind: "editor", iconDataUrl: "data:image/png;base64,AAAA" }], preferred, workspaceOpenable: true };
  if (command === "set_preferred_external_opener") preferred = args?.id as string;
} };
const container = document.createElement("div");
document.body.append(container);
const chooser = createRoot(container);
await act(async () => { chooser.render(React.createElement(ToastProvider, null, React.createElement(ExternalOpener, { tabId: "session-a", dismissSignal: 0, bridge: tauriExternalOpenerBridge }))); });
assert.deepEqual(calls.at(-1), { command: "workspace_external_openers", args: { sessionId: "session-a" } });
async function chooseApp(label: string) {
  await act(async () => { container.querySelector<HTMLButtonElement>('[aria-haspopup="menu"]')!.click(); });
  const option = [...container.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')].find(option => option.textContent?.includes(label));
  assert.ok(option);
  await act(async () => { option.click(); });
}
await chooseApp("VS Code");
assert.ok(calls.some(call => call.command === "open_workspace_external" && call.args?.sessionId === "session-a" && call.args?.id === "vscode"));
assert.deepEqual(calls.at(-1), { command: "set_preferred_external_opener", args: { id: "vscode" } });
assert.ok(container.querySelector<HTMLButtonElement>(".external-opener__primary")?.ariaLabel?.includes("VS Code"));
assert.ok(container.querySelector(".external-opener__primary img"), "native application icons are rendered");
failCommand = "open_workspace_external";
await chooseApp("Finder");
assert.equal(preferred, "vscode", "failed launch must not persist a default");
failCommand = "set_preferred_external_opener";
await chooseApp("Finder");
assert.equal(preferred, "vscode", "failed persistence keeps the actual preference");
assert.ok(document.querySelector(".toast--error")?.textContent?.includes("check profile permissions"));
assert.equal(wailsCalls, 1, "workspace launch and preference errors never fall back to Wails");
assert.ok(calls.filter(call => call.command === "open_workspace_external" || call.command === "workspace_external_openers").every(call => !Object.prototype.hasOwnProperty.call(call.args ?? {}, "path") && !Object.prototype.hasOwnProperty.call(call.args ?? {}, "workspaceRoot")), "workspace roots and programs stay in the host");
await act(async () => { chooser.unmount(); });
console.log("native local document and workspace openers, icons and preference actions: OK");
