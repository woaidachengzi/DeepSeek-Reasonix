import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true, isTauri: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
let fallbackCalls = 0, denied = false, copied = "";
Object.defineProperty(navigator, "clipboard", { value: { writeText: async () => { fallbackCalls++; } }, configurable: true });
window.open = () => { fallbackCalls++; return null; };
Object.defineProperty(document, "execCommand", { value: () => { fallbackCalls++; return true; } });
const calls: Array<{ command: string; args: unknown }> = [];
(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke: async (command: string, args: { url?: string; text?: string }) => {
  calls.push({ command, args });
  if (denied) throw new Error("native refusal: secret diagnostic");
  if (command === "plugin:clipboard-manager|write_text") copied = args.text ?? "";
  else assert.equal(command, "open_external_link");
} };
const { SearchSourcesPanel } = await import("../components/SearchSourcesPanel");
const { LocaleProvider, t } = await import("../lib/i18n");
const { ToastProvider } = await import("../lib/toast");
const root = createRoot(document.getElementById("root")!);
await act(async () => root.render(<LocaleProvider><ToastProvider><SearchSourcesPanel sources={[
  { title: "Docs", url: "https://example.com/docs?utm_source=test" },
  { title: "Unsafe", url: "javascript:alert(1)" },
]} /></ToastProvider></LocaleProvider>));
await act(async () => document.querySelector<HTMLButtonElement>(".msg-search-sources__toggle")!.click());
assert.equal(document.querySelectorAll(".msg-search-source__link").length, 1);
const link = document.querySelector<HTMLAnchorElement>(".msg-search-source__link")!;
for (const event of [
  new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }),
  new dom.window.MouseEvent("click", { bubbles: true, cancelable: true, metaKey: true }),
  new dom.window.MouseEvent("auxclick", { bubbles: true, cancelable: true, button: 1 }),
]) {
  await act(async () => { link.dispatchEvent(event); });
  assert.equal(event.defaultPrevented, true, "source activation never navigates the WebView");
  assert.deepEqual(calls.at(-1), { command: "open_external_link", args: { url: "https://example.com/docs" } });
}
const copy = document.querySelector<HTMLButtonElement>(".msg-search-source__copy")!;
await act(async () => copy.click());
assert.equal(copied, "https://example.com/docs");
assert.ok(document.body.textContent?.includes(t("richLink.copied")));
denied = true;
await act(async () => link.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true })));
assert.ok(document.body.textContent?.includes(t("settings.about.openLinkFailed")));
const action = document.querySelector<HTMLButtonElement>(".toast__action")!;
assert.ok(action, "opener refusal provides a copy recovery action");
await act(async () => action.click());
assert.ok(document.body.textContent?.includes(t("richLink.copyFailed")));
await act(async () => copy.click());
assert.equal(calls.at(-1)?.command, "plugin:clipboard-manager|write_text");
assert.equal(fallbackCalls, 0, "native refusal never falls through to browser transports");
assert.ok(!document.body.textContent?.includes("secret diagnostic"));
await act(async () => root.unmount());
console.log("Search sources native opener/copy, modified activation and refusal recovery: OK");
