// Run: node --import tsx src/__tests__/tauri-provider-connect.test.tsx
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true, isTauri: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
let installed = false;
let installs = 0;
let failKey = true;
let failRefresh = false;
let failRefreshAfterKeys = false;
const written = new Set<string>();
const writes: string[] = [];
const config = (name: string) => ({ name, displayName: name, kind: "openai", models: ["chat"], default: "chat", noProxy: false, contextWindow: 0, modelsUrlSet: false, balanceUrlSet: false, responsesMode: "", removable: true, revision: "r1" });
const presets = () => [{ id: "two-routes", label: "Two routes", description: "API defaults", group: "API", recommended: true, status: installed ? "installed" : "available", revision: "r1", routes: ["alpha", "beta"].map(name => ({ name, kind: "openai", baseUrl: "https://example.com/v1", models: ["chat"], default: "chat" })) }, { id: "other", label: "Other", status: "available", revision: "r1", routes: [], description: "", group: "API", recommended: false }];
const view = () => ({ protocolVersion: 1, providers: installed ? [config("alpha"), config("beta")] : [], presets: presets() });
Object.assign(dom.window, { __TAURI_INTERNALS__: { async invoke(command: string, args?: { input?: { presetId?: string }; key?: string; value?: string }) {
  if (command === "provider_configs") return view();
  if (command === "save_provider_config") { installs++; installed = true; return view(); }
  if (command === "provider_summary") {
    if (failRefresh || (failRefreshAfterKeys && written.size === 2)) { failRefresh = false; failRefreshAfterKeys = false; throw new Error("private refresh failure"); }
    return { protocolVersion: 1, providers: ["alpha", "beta"].map(name => ({ name, requiresKey: true, configured: written.has(name) })) };
  }
  if (command === "keychain_save") {
    const name = args!.key!.replace("api_key_", "");
    writes.push(name);
    if (name === "beta" && failKey) throw new Error("private credential failure");
    written.add(name); return;
  }
  throw new Error(`unexpected command ${command}`);
} } });
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
const { TauriProviderEditor } = await import("../tauri/TauriProviderEditor");
let root = createRoot(document.getElementById("root")!);
const click = async (text: string) => act(async () => {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === text);
  assert.ok(button, text); assert.equal(button.disabled, false); button.click();
});
const change = async (element: HTMLInputElement | HTMLSelectElement, value: string) => act(async () => {
  const prototype = element instanceof dom.window.HTMLSelectElement ? dom.window.HTMLSelectElement.prototype : dom.window.HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(element, value);
  element.dispatchEvent(new dom.window.Event(element.tagName === "SELECT" ? "change" : "input", { bubbles: true }));
});
await act(async () => root.render(<LocaleProvider><TauriProviderEditor onSummaryChange={() => {}} /></LocaleProvider>));
assert.equal(document.querySelector(".tauri-provider-connect"), null, "add form starts closed");
await click("+ Add provider");
const select = document.querySelector<HTMLSelectElement>(".tauri-provider-connect select")!;
assert.equal(select.value, "", "no alphabetically first provider is silently selected");
await change(select, "two-routes");
let key = document.querySelector<HTMLInputElement>('.tauri-provider-connect input[type="password"]')!;
await change(key, "catalog-canary");
assert.equal(document.querySelector<HTMLDetailsElement>(".tauri-provider-connect details")!.open, false, "preset details start folded");
await click("Custom model API");
const name = document.querySelector<HTMLInputElement>('input[placeholder="e.g. my-provider"]')!;
assert.ok(name);
await change(name, "draft-provider");
await click("Third-party providers");
assert.equal(key.value, "catalog-canary", "catalog draft survives mode changes");
await click("Custom model API");
assert.equal(name.value, "draft-provider", "custom draft survives mode changes");
await click("Third-party providers");
await click("Save");
assert.equal(installs, 1);
assert.deepEqual(writes, ["alpha", "beta"]);
assert.match(document.body.textContent!, /Connection saved; credentials could not be confirmed/);
assert.doesNotMatch(document.body.textContent!, /private credential failure/);
assert.equal(document.querySelector<HTMLButtonElement>('[role="tab"]')!.disabled, true, "cannot orphan a partial credential retry by switching mode");
failKey = false;
await click("Save to Keychain");
assert.equal(installs, 1, "retry never reinstalls the committed config");
assert.deepEqual(writes, ["alpha", "beta", "beta"], "retry preserves the key already committed for alpha");
assert.equal(document.querySelector(".tauri-provider-key-retry"), null);
assert.equal(document.querySelector(".tauri-provider-connect"), null);
// Exercise the post-write summary failure independently of a key refusal.
await act(async () => root.unmount());
installed = false; written.clear(); failRefreshAfterKeys = true;
root = createRoot(document.getElementById("root")!);
await act(async () => root.render(<LocaleProvider><TauriProviderEditor onSummaryChange={() => {}} /></LocaleProvider>));
await click("+ Add provider");
await change(document.querySelector<HTMLSelectElement>(".tauri-provider-connect select")!, "two-routes");
key = document.querySelector<HTMLInputElement>('.tauri-provider-connect input[type="password"]')!;
await change(key, "refresh-canary");
await click("Save");
assert.ok(document.querySelector(".tauri-provider-key-retry"), "failed state query retains the retry and secret draft");
const beforeRefreshRetryWrites = writes.length;
await click("Save to Keychain");
assert.equal(writes.length, beforeRefreshRetryWrites, "summary-only retry does not rewrite keys already stored");
assert.equal(installs, 2, "refresh retry does not duplicate a committed connection");
await act(async () => root.unmount());
console.log("PASS provider add modes, isolated drafts, folded details, credential refusal and refresh retry");
