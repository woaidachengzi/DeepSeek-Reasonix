import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const React = await import("react"); const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSpeechSettings } = await import("../tauri/TauriSpeechSettings");
const root = createRoot(document.getElementById("root")!);
let view = { protocolVersion: 1, mode: "mimo" as "mimo" | "custom", provider: "old-mimo", protocol: "transcriptions" as const, endpoint: "", model: "", keySet: false, revision: "first" };
const saved: unknown[] = [];
const api = { load: async () => view, providers: async () => [{ name: "old-mimo", label: "已有 MiMo 渠道" }], save: async (input: import("../lib/tauriBridge").TauriSpeechSettingsChange) => {
 saved.push(input); const {apiKey: _key, keyAction: _action, ...fields} = input; void _key; void _action; view = { ...view, ...fields, protocol: "transcriptions", keySet: input.keyAction === "keep" ? view.keySet : input.keyAction === "replace", revision: `save-${saved.length}` }; return view;
} };
const tick = async () => { await new Promise(resolve => setTimeout(resolve, 0)); };
const click = async (label: string) => { await act(async () => { const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(item => item.textContent === label || item.getAttribute("aria-label") === label); assert(button, label); button.click(); await tick(); }); };
const select = async (id: string, value: string) => { await act(async () => { const element = document.getElementById(id) as HTMLSelectElement; element.value = value; element.dispatchEvent(new dom.window.Event("change", { bubbles: true })); await tick(); }); };
const input = async (id: string, value: string) => { await act(async () => { const element = document.getElementById(id) as HTMLInputElement; Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(element, value); element.dispatchEvent(new dom.window.Event("input", { bubbles: true })); await tick(); }); };
try {
 await act(async () => { root.render(<TauriSpeechSettings onOpenProviders={() => {}} api={api} />); await tick(); });
 assert.equal((document.getElementById("speech-provider") as HTMLSelectElement).value, "old-mimo", "existing selection retained");
 assert.equal(saved.length, 0, "opening settings does not rewrite legacy configuration");
 assert(document.body.textContent?.includes("尚未测试"));
 await select("speech-mode", "custom");
 assert(document.querySelector<HTMLButtonElement>('[aria-label="测试录音与识别"]')?.disabled, "unsaved settings cannot be tested");
 await input("speech-endpoint", "http://127.0.0.1:8000/v1/audio/transcriptions"); await input("speech-model", "whisper-local"); await input("speech-key", "private-key");
 await click("保存语音设置");
 assert.equal(saved.length, 1); assert.equal((saved[0] as { apiKey: string }).apiKey, "private-key");
 assert.equal((document.getElementById("speech-key") as HTMLInputElement).value, "", "saved secrets are cleared from the form");
 assert(!document.body.textContent?.includes("private-key"));
 assert.equal(document.querySelector<HTMLButtonElement>('[aria-label="测试录音与识别"]')?.disabled, false);
 await input("speech-endpoint", "http://127.0.0.1:9000/v1/audio/transcriptions");
 assert(document.body.textContent?.includes("服务地址已改变"));
 await click("清除密钥"); assert(document.body.textContent?.includes("保存后将清除原密钥"));
 await click("保存语音设置"); assert.equal((saved[1] as { keyAction: string }).keyAction, "clear"); assert.equal(view.keySet, false);
 await select("speech-mode", "mimo"); await click("保存语音设置");
 assert.equal(view.provider, "old-mimo", "switching back preserves the MiMo channel");
 console.log("PASS speech settings: legacy selection, no automatic writes, saved-only tests, key write-only/clear, endpoint warning, MiMo round trip");
} finally { await act(async () => root.unmount()); dom.window.close(); }
