import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true, isTauri: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
let fallbackCalls = 0, denied = false, copied = "", deferred = false;
let complete: (() => void) | undefined;
Object.defineProperty(navigator, "clipboard", { value: { writeText: async () => { fallbackCalls++; throw new Error("WebView denied"); } }, configurable: true });
(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke: async (command: string, args: { text: string }) => {
  assert.equal(command, "plugin:clipboard-manager|write_text");
  if (denied) throw new Error("native busy: private detail");
  copied = args.text;
  if (deferred) await new Promise<void>(resolve => { complete = resolve; });
} };
const { DiagnosticsSettingsPage } = await import("../components/DiagnosticsSettingsPage");
const { LocaleProvider, t } = await import("../lib/i18n");
const report = { schema_version: 1, root: "<workspace>", live: false,
  summary: { errors: 0, warnings: 0, infos: 0, instructions: 0, skills: 0, commands: 0, hooks: 0, plugins: 0, mcp_servers: 0 },
  instructions: { docs: [] }, skills: { roots: [], entries: [], winners: 0, shadowed: 0 },
  commands: { roots: [], entries: [], winners: 0, shadowed: 0 }, hooks: { trusted_project: false, project_defines_hooks: false, sources: [], entries: [] },
  plugins: { packages: [] }, mcp: { servers: [] }, issues: [],
};
let failLoad = false;
const provider = async () => { if (failLoad) throw new Error("diagnostic load failed"); return report; };
const root = createRoot(document.getElementById("root")!);
await act(async () => root.render(<LocaleProvider><DiagnosticsSettingsPage capabilityDiagnostics={provider} runtimeDoctorProvider={null} showFrontendRecording={false} /></LocaleProvider>));
const button = (key: string) => [...document.querySelectorAll<HTMLButtonElement>(".diag-page__actions button")].find(b => b.textContent === t(key))!;
await act(async () => button("diag.copyJson").click());
assert.equal(JSON.parse(copied).root, "<workspace>", "diagnostic report uses native text write");
assert.ok(button("diag.copied"));
denied = true;
await act(async () => button("diag.copied").click());
assert.ok(button("diag.copyJson"), "failed copy clears prior success");
assert.ok(document.querySelector('[role="alert"]')?.textContent?.includes(t("diag.copyFailed")));
assert.ok(!document.body.textContent?.includes("private detail"));
denied = false; deferred = true;
await act(async () => button("diag.copyJson").click());
assert.equal(button("diag.copyJson").disabled, true, "pending write prevents duplicate copying");
assert.ok(complete);
await act(async () => button("diag.refresh").click());
await act(async () => complete!());
assert.ok(button("diag.copyJson"), "old copy completion cannot claim the refreshed report was copied");
assert.equal(button("diag.copyJson").disabled, false);
deferred = false; failLoad = true;
await act(async () => button("diag.refresh").click());
assert.match(document.querySelector('[role="alert"]')?.textContent ?? "", /diagnostic load failed/);
assert.equal(button("diag.copyJson").disabled, true);
assert.equal(fallbackCalls, 0);
await act(async () => root.unmount());
console.log("Diagnostic native copy, denial feedback, refreshed-report fencing and independent load error: OK");
