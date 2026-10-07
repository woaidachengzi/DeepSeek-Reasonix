import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { applyTauriAppearance, initTauriAppearance, readTauriAppearance } from "../tauri/tauriAppearance";

const dom = new JSDOM("<!doctype html><html><head></head><body></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage });

assert.deepEqual(readTauriAppearance(), { mode: "auto", style: "graphite" });
applyTauriAppearance({ mode: "dark", style: "aurora" });
assert.equal(document.documentElement.getAttribute("data-theme"), "dark");
assert.equal(document.documentElement.getAttribute("data-theme-style"), "aurora");
assert.equal(localStorage.getItem("tauri-theme"), "dark");

document.documentElement.setAttribute("data-theme", "light");
document.documentElement.setAttribute("data-theme-style", "graphite");
initTauriAppearance();
assert.equal(document.documentElement.getAttribute("data-theme"), "dark", "startup restores saved mode");
assert.equal(document.documentElement.getAttribute("data-theme-style"), "aurora", "startup restores saved style");

applyTauriAppearance({ mode: "auto", style: "amber" });
assert.equal(document.documentElement.hasAttribute("data-theme"), false, "system mode follows the OS media query");
assert.equal(document.documentElement.getAttribute("data-theme-style"), "amber");

localStorage.setItem("tauri-theme", "system");
localStorage.setItem("tauri-theme-style", "unknown");
assert.deepEqual(readTauriAppearance(), { mode: "auto", style: "graphite" }, "old system mode and invalid style resolve safely");

const settingsCSS = readFileSync(new URL("../tauri/tauriChatWorkspace.css", import.meta.url), "utf8");
const modelSectionSelector = ":root .tauri-settings-content .tauri-model-settings > .settings-section.tauri-model-settings__section";
function cssRule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = settingsCSS.match(new RegExp(`${escaped} \\{([^}]+)\\}`));
  assert.ok(match, `settings surface rule exists: ${selector}`);
  return match[1];
}
for (const selector of [modelSectionSelector, ".tauri-settings-model-card", ".tauri-provider-editor", ".tauri-provider-connect", ".tauri-provider-advanced"]) {
  assert.match(cssRule(selector), /background:\s*transparent;/, `${selector} shares the settings page background in every theme`);
}
assert.match(cssRule(modelSectionSelector), /box-shadow:\s*none;/, "model groups do not inherit a theme card shadow");
assert.match(cssRule(`${modelSectionSelector} + .settings-section.tauri-model-settings__section`), /border-top:\s*1px solid var\(--border-soft\);/, "model groups keep their section divider despite the stronger surface selector");
assert.match(cssRule(".tauri-provider-editor-form input:not([type=\"checkbox\"]), .tauri-provider-editor-form select, .tauri-provider-editor-form textarea"), /background:\s*var\(--surface\);/, "editable controls retain their distinct surface");

console.log("tauri appearance tests passed");
