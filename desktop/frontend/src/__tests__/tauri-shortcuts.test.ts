import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

const { defaultTauriShortcut, getTauriShortcut, isValidTauriShortcut, matchesTauriShortcut, resetTauriShortcuts, setTauriShortcut, tauriShortcutConflict } = await import("../tauri/tauriKeyboardShortcuts");

const key = (value: string, ctrl = false, shift = false) => new dom.window.KeyboardEvent("keydown", { key: value, ctrlKey: ctrl, shiftKey: shift });
assert.deepEqual(defaultTauriShortcut("settings", "darwin"), { key: ",", meta: true });
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), true);
assert.equal(setTauriShortcut("settings", { key: "o", ctrl: true, shift: true }, "linux"), true);
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), false, "old binding stops firing immediately");
assert.equal(matchesTauriShortcut(key("o", true, true), "settings", "linux"), true, "new binding works immediately");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "o", "Preview shortcut is saved separately");
assert.equal(localStorage.getItem("reasonix.customShortcuts"), null, "stable shortcut preferences are untouched");
assert.equal(tauriShortcutConflict("new_session", { key: "o", ctrl: true, shift: true }, "linux"), "settings");
assert.equal(setTauriShortcut("new_session", { key: "o", ctrl: true, shift: true }, "linux"), false, "conflicting shortcuts cannot be saved");
assert.equal(isValidTauriShortcut("send_message", { key: "o", ctrl: true }), false, "send stays on Enter");
assert.equal(isValidTauriShortcut("settings", { key: "o" }), false, "plain typing cannot become a global shortcut");

localStorage.setItem("reasonix.tauri.shortcuts.v1", JSON.stringify({ settings: { key: "p", ctrl: true } }));
dom.window.dispatchEvent(new dom.window.StorageEvent("storage", { key: "reasonix.tauri.shortcuts.v1" }));
assert.equal(getTauriShortcut("settings", "linux").key, "p", "another window's saved preference is observed");
resetTauriShortcuts();
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), true, "reset restores the default");

console.log("tauri shortcuts: OK");
