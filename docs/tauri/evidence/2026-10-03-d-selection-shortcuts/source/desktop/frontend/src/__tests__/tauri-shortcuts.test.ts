import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

const { defaultTauriShortcut, getTauriShortcut, isValidTauriShortcut, matchesTauriShortcut, nativeTauriShortcutConflict, resetTauriShortcuts, setTauriShortcut, tauriShortcutConflict, TAURI_SHORTCUT_ACTIONS, TAURI_SHORTCUT_TABS } = await import("../tauri/tauriKeyboardShortcuts");
const { default: nativeShortcuts } = await import("../tauri/macosMenuShortcuts.json");

const key = (value: string, ctrl = false, shift = false) => new dom.window.KeyboardEvent("keydown", { key: value, ctrlKey: ctrl, shiftKey: shift });
assert.deepEqual(defaultTauriShortcut("settings", "darwin"), { key: ",", meta: true });
assert.deepEqual(defaultTauriShortcut("close_panel", "darwin"), { key: "w", meta: true });
assert.deepEqual(defaultTauriShortcut("toggle_sidebar", "darwin"), { key: "b", meta: true }, "sidebar toggle follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("workspace_files", "darwin"), { key: "f", meta: true, shift: true }, "workspace browser moves to avoid the stable sidebar shortcut");
assert.deepEqual(defaultTauriShortcut("goto_session_1", "darwin"), { key: "1", meta: true }, "first session navigation follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("goto_session_9", "linux"), { key: "9", ctrl: true }, "ninth session navigation follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("open_appearance", "darwin"), { key: "a", meta: true, shift: true });
assert.deepEqual(defaultTauriShortcut("open_model_services", "linux"), { key: "p", ctrl: true, shift: true });
assert.deepEqual(defaultTauriShortcut("text_size_increase", "darwin"), { key: "=", meta: true }, "text size increase follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("text_size_decrease", "linux"), { key: "-", ctrl: true }, "text size decrease follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("text_size_reset", "windows"), { key: "0", ctrl: true }, "text size reset follows the stable shortcut");
assert.deepEqual(defaultTauriShortcut("composer_newline", "darwin"), { key: "Enter", shift: true }, "composer newline follows the stable Shift+Enter shortcut");
assert.equal(isValidTauriShortcut("open_usage_stats", { key: "u", ctrl: true, shift: true }), true);
assert.equal(TAURI_SHORTCUT_ACTIONS.length, 44, "all Preview actions, including command palette, keyboard help, sidebar toggle, session navigation, panel close, composer newline, text size and every settings page, can be configured");
assert.equal(Object.keys(TAURI_SHORTCUT_TABS).length, 16, "every remaining settings page has a direct shortcut route");
for (const action of TAURI_SHORTCUT_ACTIONS) {
  const combo = defaultTauriShortcut(action, "darwin");
  assert.equal(isValidTauriShortcut(action, combo), true, `${action} has a valid default binding`);
  assert.equal(tauriShortcutConflict(action, combo, "darwin"), null, `${action} default does not conflict`);
}
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), true);
assert.equal(setTauriShortcut("settings", { key: "q", ctrl: true, shift: true }, "linux"), true);
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), false, "old binding stops firing immediately");
assert.equal(matchesTauriShortcut(key("q", true, true), "settings", "linux"), true, "new binding works immediately");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "q", "Preview shortcut is saved separately");
assert.equal(localStorage.getItem("reasonix.customShortcuts"), null, "stable shortcut preferences are untouched");
assert.equal(tauriShortcutConflict("new_session", { key: "q", ctrl: true, shift: true }, "linux"), "settings");
assert.equal(setTauriShortcut("new_session", { key: "q", ctrl: true, shift: true }, "linux"), false, "conflicting shortcuts cannot be saved");
assert.equal(isValidTauriShortcut("send_message", { key: "o", ctrl: true }), false, "send stays on Enter");
assert.equal(isValidTauriShortcut("composer_newline", { key: "Enter", shift: true }), true, "newline accepts the safe Shift+Enter chord without a global modifier");
assert.equal(isValidTauriShortcut("composer_newline", { key: "o", ctrl: true, shift: true }), false, "newline remains bound to Enter");
assert.equal(isValidTauriShortcut("settings", { key: "o" }), false, "plain typing cannot become a global shortcut");

localStorage.setItem("reasonix.tauri.shortcuts.v1", JSON.stringify({ settings: { key: "p", ctrl: true } }));
dom.window.dispatchEvent(new dom.window.StorageEvent("storage", { key: "reasonix.tauri.shortcuts.v1" }));
assert.equal(getTauriShortcut("settings", "linux").key, "p", "another window's saved preference is observed");
resetTauriShortcuts();
assert.equal(matchesTauriShortcut(key(",", true), "settings", "linux"), true, "reset restores the default");

for (const native of [...nativeShortcuts.required, ...nativeShortcuts.optional]) {
  assert.equal(nativeTauriShortcutConflict(native, "darwin"), true, `${native.title} stays native`);
  assert.equal(setTauriShortcut("settings", native, "darwin"), false, `${native.title} cannot be saved as a Preview shortcut`);
  const event = new dom.window.KeyboardEvent("keydown", {
    key: native.key, metaKey: native.meta, ctrlKey: "ctrl" in native && native.ctrl === true,
    altKey: "alt" in native && native.alt === true, shiftKey: "shift" in native && native.shift === true,
  });
  for (const action of TAURI_SHORTCUT_ACTIONS) {
    assert.equal(matchesTauriShortcut(event, action, "darwin"), false, `${native.title} never dispatches ${action}`);
  }
}
assert.equal(nativeTauriShortcutConflict({ key: "C", meta: true }, "darwin"), true, "native chords are case normalized");
assert.equal(nativeTauriShortcutConflict({ key: " ", meta: true }, "darwin"), true, "AppKit's raw space key matches the normalized keyboard event");
assert.equal(nativeTauriShortcutConflict({ key: "c", meta: true, shift: true }, "darwin"), false, "a different modifier combination is not over-reserved");
assert.equal(nativeTauriShortcutConflict({ key: "r", meta: true }, "darwin"), false, "refresh stays configurable after removing native Reload's accelerator");
assert.equal(matchesTauriShortcut(new dom.window.KeyboardEvent("keydown", { key: "r", metaKey: true }), "refresh_session", "darwin"), true);

localStorage.setItem("reasonix.tauri.shortcuts.v1", JSON.stringify({ settings: { key: "q", meta: true } }));
dom.window.dispatchEvent(new dom.window.StorageEvent("storage", { key: "reasonix.tauri.shortcuts.v1" }));
assert.deepEqual(getTauriShortcut("settings", "darwin"), defaultTauriShortcut("settings", "darwin"), "legacy native conflicts fall back to the safe default");
assert.equal(matchesTauriShortcut(new dom.window.KeyboardEvent("keydown", { key: "q", metaKey: true }), "settings", "darwin"), false, "legacy native chords never dispatch a Preview action");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "q", "rehydration does not overwrite legacy preferences");

resetTauriShortcuts();
assert.equal(setTauriShortcut("settings", { key: "q", meta: true, shift: true }, "darwin"), true);
assert.equal(setTauriShortcut("new_session", { key: ",", meta: true }, "darwin"), true);
assert.equal(setTauriShortcut("settings", null, "darwin"), false, "individual reset cannot create a duplicate default binding");
assert.equal(getTauriShortcut("settings", "darwin").key, "q", "failed reset preserves the active binding");
assert.equal(setTauriShortcut("new_session", null, "darwin"), true);
assert.equal(setTauriShortcut("settings", null, "darwin"), true, "reset works once the original chord is free");
resetTauriShortcuts();

for (const action of TAURI_SHORTCUT_ACTIONS) {
  const combo = defaultTauriShortcut(action, "darwin");
  for (const composition of [{ isComposing: true }, { keyCode: 229 }]) {
    const event = new dom.window.KeyboardEvent("keydown", {
      key: combo.key, metaKey: combo.meta, ctrlKey: combo.ctrl,
      altKey: combo.alt, shiftKey: combo.shift, ...composition,
    });
    assert.equal(matchesTauriShortcut(event, action, "darwin"), false, `${action} leaves composition keys to the input method`);
  }
}

console.log("tauri shortcuts: OK");

resetTauriShortcuts();
assert.deepEqual(defaultTauriShortcut("add_selection", "darwin"), { key: "l", meta: true });
assert.equal(setTauriShortcut("add_selection", { key: "r", meta: true }, "darwin"), false, "selection cannot take the refresh binding");
assert.equal(setTauriShortcut("add_selection", { key: "c", meta: true }, "darwin"), false, "selection cannot take native Copy");
