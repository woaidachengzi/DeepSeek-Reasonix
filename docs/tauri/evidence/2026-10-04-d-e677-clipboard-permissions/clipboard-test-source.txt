import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div><input id='focus' /></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const host = globalThis as typeof globalThis & { isTauri?: boolean };
const win = window as unknown as { __TAURI_INTERNALS__?: { invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown> }; runtime?: { ClipboardSetText?: (text: string) => Promise<boolean>; ClipboardGetText?: () => Promise<string> } };
const { readClipboardText, readClipboardTextOrThrow, writeClipboardText } = await import("../lib/clipboard");
const { CopyButton } = await import("../components/CopyButton");
const { LocaleProvider, t } = await import("../lib/i18n");
const { ToastProvider } = await import("../lib/toast");

let nativeText = "";
let browserCalls = 0;
Object.defineProperty(navigator, "clipboard", { configurable: true, value: {
  writeText: async () => { browserCalls++; throw new Error("denied"); },
  readText: async () => { browserCalls++; throw new Error("denied"); },
} });
host.isTauri = true;
win.__TAURI_INTERNALS__ = { invoke: async (command, args) => {
  if (command === "plugin:clipboard-manager|write_text") { nativeText = args?.text as string; return; }
  if (command === "plugin:clipboard-manager|read_text") return nativeText;
  throw new Error(`unexpected command: ${command}`);
} };
assert.equal(await writeClipboardText("中文\nline two"), true);
assert.equal(await readClipboardText(), "中文\nline two");
assert.equal(browserCalls, 0, "Tauri uses native text access even when the WebView Clipboard API is denied");

let wailsCalls = 0, legacyCopyCalls = 0;
Object.defineProperty(navigator, "clipboard", { configurable: true, value: {
  writeText: async () => { browserCalls++; },
  readText: async () => { browserCalls++; return "browser secret"; },
} });
win.runtime = { ClipboardSetText: async value => { wailsCalls++; nativeText = value; return true; }, ClipboardGetText: async () => { wailsCalls++; return "Wails secret"; } };
Object.defineProperty(document, "execCommand", { configurable: true, value: () => { legacyCopyCalls++; return true; } });
for (const reason of ["clipboard-manager read_text not allowed", "native clipboard busy"]) {
  win.__TAURI_INTERNALS__.invoke = async () => { throw new Error(reason); };
  assert.equal(await writeClipboardText("refused write"), false, "Tauri write refusal is terminal");
  assert.equal(await readClipboardText(), "", "forgiving Tauri read refuses fallback data");
  await assert.rejects(readClipboardTextOrThrow(), new RegExp(reason), "strict Tauri read preserves refusal");
}
assert.equal(browserCalls, 0, "Tauri refusal cannot switch to browser permissions");
assert.equal(wailsCalls, 0, "Tauri refusal cannot reach another host bridge");
assert.equal(legacyCopyCalls, 0, "Tauri refusal cannot use execCommand");
assert.equal(document.querySelector("textarea"), null, "refusal never creates a fallback textarea");

host.isTauri = false;
Object.defineProperty(navigator, "clipboard", { configurable: true, value: {
  writeText: async () => { throw new Error("browser denied"); },
  readText: async () => { throw new Error("browser denied"); },
} });
assert.equal(await writeClipboardText("Wails fallback"), true, "Wails retains its native bridge fallback");
assert.equal(await readClipboardText(), "Wails secret");
assert.equal(wailsCalls, 2);
delete win.runtime;
const focused = document.getElementById("focus") as HTMLInputElement;
focused.focus();
Object.defineProperty(document, "execCommand", { configurable: true, value: () => true });
assert.equal(await writeClipboardText("browser fallback"), true);
assert.equal(document.activeElement, focused, "fallback returns focus to the original input");
assert.equal(document.querySelector("textarea"), null, "fallback removes its temporary editable element");
Object.defineProperty(document, "execCommand", { configurable: true, value: () => { throw new Error("denied"); } });
assert.equal(await writeClipboardText("cannot copy"), false, "all clipboard failures return false");
assert.equal(await readClipboardText(), "", "unreadable clipboard does not overwrite a selected draft with invented text");
await assert.rejects(readClipboardTextOrThrow(), /unavailable/, "editors can distinguish denied access from an empty clipboard");

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<LocaleProvider><CopyButton text="copy result" /></LocaleProvider>); });
const button = document.querySelector("button")!;
await act(async () => { button.click(); });
assert.equal(button.classList.contains("copybtn--copied"), false, "failed writes must not show success");
let completeWrite: (() => void) | undefined;
Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: () => new Promise<void>(resolve => { completeWrite = resolve; }) } });
await act(async () => { button.click(); });
assert.equal(button.classList.contains("copybtn--copied"), false, "pending writes must not show success");
await act(async () => { completeWrite?.(); });
assert.equal(button.classList.contains("copybtn--copied"), true, "completed writes show success");
Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async () => { throw new Error("private-native-error"); } } });
await act(async () => { button.click(); });
assert.equal(button.classList.contains("copybtn--copied"), false, "refused copy clears the preceding success");
assert.equal(button.title, t("msg.copyFailed"), "copy refusal offers a retry instruction");
host.isTauri = true;
let nativeWrites = 0;
let releaseNative!: () => void;
let nativeDenied = true;
win.__TAURI_INTERNALS__ = { invoke: async (command, args) => {
  assert.equal(command, "plugin:clipboard-manager|write_text");
  nativeWrites++;
  if (nativeDenied) throw new Error("private-native-error");
  nativeText = args?.text as string;
  await new Promise<void>(resolve => { releaseNative = resolve; });
} };
const renderCopy = async (text: string, getText?: () => Promise<string>) => {
  await act(async () => root.render(<LocaleProvider><ToastProvider><CopyButton text={text} getText={getText} /></ToastProvider></LocaleProvider>));
  return document.querySelector<HTMLButtonElement>(".copybtn")!;
};
let nativeButton = await renderCopy("native message");
await act(async () => nativeButton.click());
assert.equal(nativeButton.classList.contains("copybtn--copied"), false);
assert.equal(document.querySelector(".toast--error .toast__text")?.textContent, t("msg.copyFailed"), "native refusal produces visible recovery feedback");
assert.ok(!document.body.textContent?.includes("private-native-error"));
nativeDenied = false;
await act(async () => { nativeButton.click(); nativeButton.click(); });
assert.equal(nativeWrites, 2, "a pending copy accepts only one native write");
assert.equal(nativeButton.disabled, true);
nativeButton = await renderCopy("replacement message");
await act(async () => releaseNative());
assert.equal(nativeButton.classList.contains("copybtn--copied"), false, "old write cannot mark replacement content copied");
assert.equal(nativeButton.disabled, false);

let releaseText!: (text: string) => void;
const delayedText = () => new Promise<string>(resolve => { releaseText = resolve; });
nativeButton = await renderCopy("lazy message", delayedText);
await act(async () => nativeButton.click());
nativeButton = await renderCopy("latest message");
await act(async () => releaseText("stale generated content"));
assert.equal(nativeWrites, 2, "replaced asynchronous content never reaches the native clipboard");
assert.equal(nativeButton.classList.contains("copybtn--copied"), false);
assert.equal(browserCalls, 0, "native message copy never falls back to WebView access");
assert.equal(legacyCopyCalls, 0);
await act(async () => { root.unmount(); });
console.log("native/browser/Wails clipboard and copy feedback: OK");
