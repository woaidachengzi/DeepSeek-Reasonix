import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { TauriBotSettings } from "../lib/tauriBridge";

async function main() {
  const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/" });
  Object.assign(globalThis, { window: dom.window, document: dom.window.document,
    IS_REACT_ACT_ENVIRONMENT: true, isTauri: true });
  Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
  const pending: { resolve(value: unknown): void; reject(error: Error): void }[] = [];
  Object.assign(dom.window, { __TAURI_INTERNALS__: { invoke: (name: string) => {
    assert.equal(name, "bot_connection_diagnostics");
    return new Promise((resolve, reject) => pending.push({ resolve, reject }));
  } } });
  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { LocaleProvider } = await import("../lib/i18n");
  const { TauriBotDiagnostics } = await import("../tauri/TauriBotDiagnostics");
  const root = createRoot(document.getElementById("root")!);
  const settings = {} as TauriBotSettings;
  const render = (disabled: boolean) => root.render(React.createElement(LocaleProvider, null,
    React.createElement(TauriBotDiagnostics, { settings, disabled })));
  const fixture = (id: string) => ({ protocolVersion: 1, runtimeObservationOnly: true,
    connections: [{ id, configStatus: "configured", runtimeStatus: "not_observed" }] });
  await act(async () => { render(false); });
  assert.equal(pending.length, 1);
  assert.ok(document.querySelector("[role=status]"));
  assert.equal(document.querySelector<HTMLButtonElement>("button")!.disabled, true);
  await act(async () => { render(true); });
  await act(async () => { render(false); });
  assert.equal(pending.length, 2);
  await act(async () => { pending[0].resolve(fixture("retired-private-id")); });
  assert.ok(!document.body.textContent!.includes("retired-private-id"));
  assert.ok(document.querySelector("[role=status]"), "old finally does not clear current loading");
  await act(async () => { pending[1].resolve(fixture("current-owned-id")); });
  assert.ok(document.body.textContent!.includes("current-owned-id"));
  assert.ok(document.body.textContent!.includes("Not observed"));
  await act(async () => { document.querySelector<HTMLButtonElement>("button")!.click(); });
  assert.ok(!document.body.textContent!.includes("current-owned-id"), "refresh clears stale observation");
  await act(async () => { pending[2].reject(new Error("private-sdk-secret-canary")); });
  assert.ok(document.querySelector("[role=alert]")!.textContent!.includes("refresh to retry"));
  assert.ok(!document.body.textContent!.includes("private-sdk-secret-canary"));
  await act(async () => { document.querySelector<HTMLButtonElement>("button")!.click(); });
  await act(async () => { pending[3].resolve({ protocolVersion: 1, runtimeObservationOnly: true, connections: [] }); });
  assert.ok(document.body.textContent!.includes("No connections to inspect"));
  await act(async () => { document.querySelector<HTMLButtonElement>("button")!.click(); });
  await act(async () => { root.unmount(); pending[4].resolve(fixture("after-unmount")); });
  assert.equal(document.getElementById("root")!.textContent, "");
  dom.window.close();
  console.log("PASS bot diagnostics panel: loading, retry, fixed error, empty, saving epoch retirement and unmount");
}
void main().catch(error => { console.error(error); process.exitCode = 1; });
