import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { localStorage: dom.window.localStorage });

const { getTauriDefaultWorkspace, setTauriDefaultWorkspace } = await import("../tauri/tauriDefaultWorkspace");
assert.equal(getTauriDefaultWorkspace(), "", "Preview starts without an implicit workspace");
assert.equal(setTauriDefaultWorkspace("/work/preview"), "/work/preview");
assert.equal(getTauriDefaultWorkspace(), "/work/preview", "the selected workspace survives a fresh read");
assert.equal(localStorage.getItem("reasonix.tauri.default-workspace.v1"), "/work/preview");
assert.throws(() => setTauriDefaultWorkspace("relative/path"), /绝对路径/, "a relative workspace cannot be saved");
assert.equal(getTauriDefaultWorkspace(), "/work/preview", "a rejected change preserves the saved workspace");
localStorage.setItem("reasonix.tauri.default-workspace.v1", "../stale");
assert.equal(getTauriDefaultWorkspace(), "", "a malformed stored path is ignored");
setTauriDefaultWorkspace("C:\\work\\preview");
assert.equal(getTauriDefaultWorkspace(), "C:\\work\\preview", "Windows absolute paths remain valid");
setTauriDefaultWorkspace("");
assert.equal(localStorage.getItem("reasonix.tauri.default-workspace.v1"), null, "clearing removes the saved preference");

console.log("tauri default workspace: OK");
