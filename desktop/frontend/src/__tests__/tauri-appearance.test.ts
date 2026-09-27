import assert from "node:assert/strict";
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

console.log("tauri appearance tests passed");
