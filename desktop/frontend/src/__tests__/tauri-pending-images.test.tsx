// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { TauriBridgeEvent } from "../lib/tauriBridge";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true, isTauri: true,
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.assign(dom.window, {
  __TAURI_INTERNALS__: { invoke: async (command: string) => command === "workspace_external_openers" ? [] : null, metadata: { currentWindow: { label: "main" }, currentWebview: { label: "main" } }, transformCallback: () => 1 },
  __TAURI_EVENT_PLUGIN_INTERNALS__: { unregisterListener: () => {} },
});
const fixture = globalThis as typeof globalThis & {
  __workbenchSessions: { sessionId: string; title: string }[];
  __historyBySession: Record<string, { role: string; content: string }[]>;
  __parallelSessionStates: Record<string, string>;
  __workspaceImage: { url: string };
  __submitGate?: Promise<void>;
  __historyGate?: Promise<void>;
  __emitBridgeEvent?: (event: TauriBridgeEvent) => void;
  __tauriBridgeCalls: { name: string; args: Record<string, unknown> }[];
};
fixture.__workbenchSessions = [{ sessionId: "images", title: "图片回归" }];
fixture.__historyBySession = { images: [] };
fixture.__parallelSessionStates = { images: "idle" };
fixture.__workspaceImage = { url: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=" };
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const settle = async () => { for (let i = 0; i < 3; i++) await new Promise(resolve => setTimeout(resolve, 0)); };
let sequence = 0;
const emit = (kind: string, payload = {}) => fixture.__emitBridgeEvent?.({
  protocolVersion: 1, sessionId: "images", sequence: ++sequence, eventKind: kind,
  payload: { kind, ...payload },
} as TauriBridgeEvent);
const pending = () => document.querySelector('[data-transcript-block-key="pending-question"]');
const thumbnail = () => pending()?.querySelector<HTMLImageElement>(".tauri-message__attachments img");

try {
  await act(async () => { root.render(React.createElement(TauriSessionApp)); await settle(); });
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-sidebar__session")!.click(); await settle(); });
  for (const text of ["看看截图", ""]) {
    await act(async () => {
      const input = document.querySelector("textarea")!;
      Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(input, text);
      input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
      const paste = new dom.window.Event("paste", { bubbles: true, cancelable: true });
      // Native-only screenshot staging uses the same saved attachment as
      // WebView FileReader staging, without accessing the real clipboard.
      Object.defineProperty(paste, "clipboardData", { value: { files: [], items: [], getData: () => "" } });
      input.dispatchEvent(paste);
      await settle();
    });
    assert(document.querySelector(".tauri-composer__attachment img"), "pasted image is visible in the draft");
    let accept!: () => void;
    let publish!: () => void;
    fixture.__submitGate = new Promise<void>(resolve => { accept = resolve; });
    fixture.__historyGate = new Promise<void>(resolve => { publish = resolve; });
    await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="发送消息"]')!.click(); await settle(); });
    const input = fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_submit").at(-1)!.args.input as string;
    assert(input.includes("@.reasonix/attachments/image.png"));
    assert.equal(thumbnail()?.getAttribute("src"), fixture.__workspaceImage.url, "submitted thumbnail is visible before admission finishes, including image-only messages");
    fixture.__historyBySession.images = [...fixture.__historyBySession.images, { role: "user", content: input }];
    await act(async () => { accept(); emit("turn_started"); emit("reasoning", { text: "thinking" }); await settle(); });
    assert(thumbnail(), "image stays visible while the model thinks");
    await act(async () => { emit("text", { text: "截图回答" }); await settle(); });
    assert(thumbnail(), "image stays visible during the streamed answer");
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 120)); publish(); await settle(); });
    assert(thumbnail(), "the active turn keeps its image until final history is ready");
    const expectedImages = text ? 1 : 2;
    delete fixture.__historyGate;
    delete fixture.__submitGate;
    fixture.__parallelSessionStates.images = "idle";
    fixture.__historyBySession.images.push({ role: "assistant", content: "截图回答" });
    await act(async () => { emit("turn_done"); await settle(); });
    assert.equal(Boolean(pending()), false, "authoritative history replaces the optimistic bubble");
    assert.equal(document.querySelectorAll(".tauri-message.is-user .tauri-message__attachments img").length, expectedImages, "completion keeps the image visible without duplicates");
  }
  console.log("PASS pasted images during submit, reasoning, streaming and completed history (text + image and image only)");
} finally {
  await act(async () => root.unmount());
  dom.window.close();
}
