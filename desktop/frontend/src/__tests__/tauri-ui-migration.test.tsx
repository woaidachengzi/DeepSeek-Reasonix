import assert from "node:assert/strict";
import { managementDom } from "../test-support/managementDom";
const dom = managementDom();
const migration = await import("../tauri/legacyUiMigration");
const workspace = "reasonix.tauri.default-workspace.v1";
const progress = "tauri-progress-mode";
const journal = "reasonix.tauri.ui-preference-migration.v1";
const snapshot = { source: "tauri://localhost", values: { [workspace]: "/old/workspace", [progress]: "deep" } };
localStorage.setItem(workspace, "/current/workspace");
localStorage.setItem("credential-canary", "unrelated");
assert.throws(() => migration.importUiPreferences(snapshot, false), /确认/);
assert.equal(localStorage.getItem(workspace), "/current/workspace");
for (const input of [
  { ...snapshot, source: "https://foreign" },
  { source: snapshot.source, values: { apiKey: "secret" } },
  { source: snapshot.source, values: { [workspace]: "relative" } },
  { source: snapshot.source, values: { "reasonix.tauri.shortcuts.v1": "invalid-json" } },
  { source: snapshot.source, values: { [progress]: "x".repeat(65_537) } },
]) assert.throws(() => migration.importUiPreferences(input, true));
assert.equal(migration.importUiPreferences(snapshot, true), 2);
assert(migration.hasUiPreferenceMigration());
assert.equal(localStorage.getItem(workspace), "/old/workspace");
assert.throws(() => migration.importUiPreferences(snapshot, true), /撤回/);
localStorage.setItem(progress, "standard"); // Later user edit must survive undo.
assert.deepEqual(migration.rollbackUiPreferences(), { restored: 1, conflicts: 1 });
assert.equal(localStorage.getItem(workspace), "/current/workspace");
assert.equal(localStorage.getItem(progress), "standard");
assert(migration.hasUiPreferenceMigration());
localStorage.setItem(progress, "deep");
assert.deepEqual(migration.rollbackUiPreferences(), { restored: 1, conflicts: 0 });
assert.equal(localStorage.getItem(progress), null);
assert.equal(localStorage.getItem("credential-canary"), "unrelated");
assert(!migration.hasUiPreferenceMigration());

// Quota/disk failures after a saved journal and a partial import retain the
// record when rollback also fails. A restarted reader can finish the rollback.
const memory = new Map<string, string>();
let writes = 0;
let fail = true;
const storage: Storage = {
  get length() { return memory.size; }, key: index => [...memory.keys()][index] ?? null,
  clear: () => memory.clear(), getItem: key => memory.get(key) ?? null,
  setItem(key, value) { writes++; if (fail && writes >= 3) throw new Error("quota"); memory.set(key, value); },
  removeItem(key) { if (fail) throw new Error("locked"); memory.delete(key); },
};
assert.throws(() => migration.importUiPreferences(snapshot, true, storage), /写入失败/);
assert(memory.has(journal));
assert.equal(memory.get(workspace), "/old/workspace");
assert(!memory.has(progress));
fail = false;
assert.deepEqual(migration.rollbackUiPreferences(storage), { restored: 1, conflicts: 0 });
assert.equal(memory.size, 0);
const blocked: Storage = { ...storage, setItem() { throw new Error("quota"); } };
assert.throws(() => migration.importUiPreferences(snapshot, true, blocked));
assert.equal(memory.size, 0, "failure to write journal performs zero preference writes");
localStorage.setItem(journal, JSON.stringify({ version: 1, source: snapshot.source, before: { apiKey: null }, after: { apiKey: "forbidden" } }));
assert.throws(() => migration.rollbackUiPreferences());
assert.equal(localStorage.getItem("apiKey"), null);
localStorage.removeItem(journal);

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
const { TauriLegacyUiPreferences } = await import("../tauri/TauriLegacyUiPreferences");
const state = globalThis as typeof globalThis & { __legacyUiSnapshot?: unknown; __legacyUiReadError?: boolean; __tauriBridgeCalls: { name: string }[] };
state.__legacyUiSnapshot = snapshot;
const root = createRoot(document.getElementById("root")!);
const settle = () => new Promise<void>(resolve => setTimeout(resolve, 0));
const button = (text: string) => [...document.querySelectorAll("button")].find(button => button.textContent === text)!;
await act(async () => { root.render(React.createElement(LocaleProvider, null, React.createElement(TauriLegacyUiPreferences))); });
assert.equal(state.__tauriBridgeCalls.length, 0, "mount never reads the old shared store implicitly");
await act(async () => { button("Preview old UI preferences").click(); await settle(); });
assert.equal(state.__tauriBridgeCalls.at(-1)?.name, "legacy_ui_preferences");
assert(document.body.textContent?.includes("/old/workspace"));
assert(button("Import with rollback record").disabled);
await act(async () => { document.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click(); });
await act(async () => { button("Import with rollback record").click(); await settle(); });
assert.equal(localStorage.getItem(workspace), "/old/workspace");
assert(document.body.textContent?.includes("Restart the app"));
assert(button("Import with rollback record").disabled);
await act(async () => { root.unmount(); });
const restoredRoot = createRoot(document.getElementById("root")!);
await act(async () => { restoredRoot.render(React.createElement(LocaleProvider, null, React.createElement(TauriLegacyUiPreferences))); });
assert(button("Undo UI preference migration"), "rollback survives component restart without rereading legacy data");
await act(async () => { button("Undo UI preference migration").click(); await settle(); });
assert.equal(localStorage.getItem(workspace), "/current/workspace");
assert(!migration.hasUiPreferenceMigration());
state.__legacyUiReadError = true;
await act(async () => { button("Preview old UI preferences").click(); await settle(); });
assert(document.querySelector('[role="alert"]')?.textContent?.includes("retry"));
assert.equal(document.querySelector('input[type="checkbox"]'), null, "failed preview clears stale confirmation");
await act(async () => { restoredRoot.unmount(); });
dom.window.close();
console.log("UI preference migration: explicit preview/confirmation, bounded scope, rollback, user edits and storage failures OK");
