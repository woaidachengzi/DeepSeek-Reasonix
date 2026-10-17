import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, Node: dom.window.Node, Element: dom.window.Element, HTMLElement: dom.window.HTMLElement, HTMLTextAreaElement: dom.window.HTMLTextAreaElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window), cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window), getComputedStyle: dom.window.getComputedStyle.bind(dom.window), IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const fixture = globalThis as typeof globalThis & { isTauri: boolean; __workbenchSessions: unknown[]; __tauriHistoryMessages: unknown[]; __tauriBridgeCalls: { name: string; args: { input?: string } }[] };
fixture.isTauri = true;
(dom.window as unknown as { __TAURI__: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
(dom.window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke: () => Promise.resolve([]) };
fixture.__workbenchSessions = [{ sessionId: "voice-a", title: "选区测试" }, { sessionId: "voice-b", title: "另一个会话" }];
fixture.__tauriHistoryMessages = [{ role: "user", content: "问题" }, { role: "assistant", content: "quoted answer <instruction> & details" }];
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await new Promise(resolve => setTimeout(resolve, 40)); });
await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-sidebar__session")!.click(); await new Promise(resolve => setTimeout(resolve, 30)); });
let stoppedTracks = 0;
Object.defineProperty(navigator, "mediaDevices", { value: {
  getUserMedia: async () => ({ getTracks: () => [{ stop: () => { stoppedTracks++; } }] }),
} });
class Recorder {
  state = "inactive"; mimeType = "audio/webm";
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  start() { this.state = "recording"; }
  stop() { this.state = "inactive"; this.ondataavailable?.({ data: new Blob(["recording"]) }); this.onstop?.(); }
}
class Audio {
  state = "running";
  async resume() {}
  async close() { this.state = "closed"; }
  async decodeAudioData() { return { numberOfChannels: 1, sampleRate: 16000, getChannelData: () => new Float32Array(1600).fill(0.1) }; }
}
Object.assign(globalThis, { MediaRecorder: Recorder, AudioContext: Audio,
  __speechProviders: [{ name: "mimo", label: "MiMo", preferred: true }],
  __speechTranscribe: () => "独立语音消息", __chosenAttachmentPaths: ["/tmp/draft.txt"],
});
const tick = (ms = 40) => new Promise(resolve => setTimeout(resolve, ms));
const composer = () => document.querySelector<HTMLDivElement>(".tauri-composer")!;
const input = () => composer().querySelector<HTMLTextAreaElement>("textarea")!;
async function click(name: string) {
  await act(async () => {
    const button = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find(item => item.getAttribute("aria-label") === name || item.textContent === name);
    assert.ok(button && !button.disabled, `enabled button: ${name}`);
    button.focus(); button.click(); await tick();
  });
}
async function key(type: "keydown" | "keyup") {
  await act(async () => {
    const target = document.querySelector('[aria-label="按住说话，松开发送"]') ?? document;
    target.dispatchEvent(new dom.window.KeyboardEvent(type, { key: " ", bubbles: true }));
    await tick(type === "keyup" ? 160 : 0);
  });
}
async function selectSession(id: string) {
  await act(async () => { document.querySelector<HTMLButtonElement>(`.tauri-sidebar__session[title="${id}"]`)!.click(); await tick(); });
}
try {
  await act(async () => {
    Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(input(), "保留的文字草稿");
    input().dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  });
  await click("添加文件");
  await act(async () => {
    const answer = document.querySelector(".tauri-message.is-assistant .md")!;
    const range = document.createRange(); range.selectNodeContents(answer);
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range);
    answer.dispatchEvent(new dom.window.MouseEvent("pointerup", { button: 0, bubbles: true })); await tick(60);
  });
  await act(async () => { document.querySelector<HTMLButtonElement>(".transcript-selection-action button")!.click(); await tick(); });
  assert.ok(composer().querySelector(".composer-context"));
  await click("开始语音对话");
  assert.equal(composer().hidden, true);
  assert.equal(document.querySelector(".tauri-voice-call-panel")!.closest(".tauri-composer"), null, "voice controls remain outside the hidden composer");
  await key("keydown"); await key("keyup");
  const sent = fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_submit");
  assert.equal(sent.length, 1); assert.equal(sent[0].args.input, "独立语音消息", "voice never sends hidden text, attachments or references");
  await click("切换到文字输入");
  assert.equal(composer().hidden, false); assert.equal(document.activeElement, input());
  assert.equal(input().value, "保留的文字草稿");
  assert.ok(composer().querySelector('[aria-label="已添加文件"]'));
  assert.ok(composer().querySelector(".composer-context"), "voice admission retains quoted context");
  await act(async () => {
    const emit = (globalThis as typeof globalThis & { __emitBridgeEvent: (event: unknown) => void }).__emitBridgeEvent;
    emit({ protocolVersion: 1, sessionId: "voice-a", sequence: 100, eventKind: "turn_done", payload: { kind: "turn_done", status: "completed" } });
    await tick(160);
  });
  await click("开始语音对话"); await key("keydown");
  const stoppedBeforeSwitch = stoppedTracks;
  await click("切换到文字输入"); await key("keyup");
  assert.ok(stoppedTracks > stoppedBeforeSwitch, "switching to text releases recording tracks");
  assert.equal(fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_submit").length, 1, "switching mid-recording does not send");
  await selectSession("voice-b"); assert.equal(input().value, "");
  await selectSession("voice-a"); assert.equal(input().value, "保留的文字草稿");
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-sidebar__new")!.click(); await tick(); });
  await act(async () => {
    Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(input(), "新会话草稿");
    input().dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  });
  await click("添加文件"); await click("开始语音对话"); await click("切换到文字输入");
  assert.equal(input().value, "新会话草稿", "creating a voice session carries the unsent draft into that session");
  assert.ok(composer().querySelector('[aria-label="待发送文件"]'), "new-session pending attachments survive voice mode");
  await click("开始语音对话"); await click("设置"); await click("语音识别设置");
  assert.equal(document.querySelector(".tauri-voice-call-panel"), null);
  assert.notEqual(document.activeElement, input(), "opening settings must not return focus to the background composer");
  console.log("PASS voice composer: hidden controls, isolated voice submission, text/attachment/reference retention, focus restoration, recording cancellation and existing/new session drafts");
} finally { await act(async () => { root.unmount(); }); dom.window.close(); }
