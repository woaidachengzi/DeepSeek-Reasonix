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
const calls: string[] = [];
const summary = () => ({
  protocolVersion: 1,
  defaultModel: "",
  providers: [{ name: "demo", displayName: "Demo", kind: "openai", modelCount: 1, models: ["m"], requiresKey: true, configured: keyPresent || envCredentialPresent }],
});

(dom.window as unknown as { __TAURI_INTERNALS__: { invoke: (command: string) => Promise<unknown> } }).__TAURI_INTERNALS__ = {
  async invoke(command: string) {
    calls.push(command);
    switch (command) {
      case "preview_runtime_info": return { previewVersion: "1", stableVersion: "1", tauriVersion: "2", bridgeProtocolVersion: 1, previewBuild: "test" };
      case "provider_summary":
        if (failSummary) { failSummary = false; throw new Error("summary unavailable"); }
        return summary();
      case "platform_info": return "darwin";
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

renderToString(<TauriSettings onClose={() => {}} />);
assert.equal(calls.length, 0, "rendering settings does not start bridge work");

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<TauriSettings onClose={() => {}} onProviderSummaryChange={value => { parentConfigured = value.providers[0]?.configured; }} />); });
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

await act(async () => { click("模型"); });
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

await act(async () => { root.unmount(); });
console.log("tauri settings API key flow passed");
