import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<html><body></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const { createElement } = await import("react");
const { renderToStaticMarkup } = await import("react-dom/server");
const { LocaleProvider } = await import("../lib/i18n");
const { TauriRemoteServe } = await import("../tauri/TauriRemoteServe");
function firstPaint(workspace: string) {
  document.body.innerHTML = renderToStaticMarkup(createElement(LocaleProvider, { children:
    createElement(TauriRemoteServe, { name: "owned-image", workspace, credentialMode: "remote" }) }));
}
// No passive effects have run: the status response does not exist yet.
firstPaint("/owned/workspace");
const entry = document.querySelector<HTMLButtonElement>("button[aria-expanded]");
assert.ok(entry);
assert.equal(entry.disabled, true, "first paint must not advertise a ready history entry before status initialization");
assert.ok([...document.querySelectorAll<HTMLButtonElement>("button")].every(button => button.disabled), "all initial actions await authoritative status");
firstPaint("");
assert.equal(document.querySelector("button"), null, "missing workspace has no actionable Serve controls");
dom.window.close();
console.log("Remote Serve: first-paint status admission and missing-workspace boundary passed");
