// Run: node --import tsx src/__tests__/tauri-image-preview.test.tsx
import assert from "node:assert/strict";
import { register } from "node:module";
import { JSDOM } from "jsdom";

register(new URL("../../scripts/tauri-bridge-stub-loader.mjs", import.meta.url));

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement,
  localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window),
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const { createElement, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { MarkdownImage } = await import("../components/MarkdownImage");
const { TauriImageScope } = await import("../tauri/TauriImageScope");
const { LocaleProvider } = await import("../lib/i18n");
await import("../components/ImageViewer");
const state = globalThis as typeof globalThis & {
  __imageGate?: (session: string, source: string) => Promise<{ url: string; errorCode?: string }>;
  __tauriBridgeCalls: { name: string }[];
};
state.__imageGate = async (_session, source) => source === "bad.png" ? { url: "", errorCode: "not-found" } : { url: `data:image/png;base64,${source}` };
const root = createRoot(document.getElementById("root")!);
const render = (session: string, source = "first.png") => root.render(createElement(LocaleProvider, { children: createElement(TauriImageScope, { sessionId: session, children: createElement(MarkdownImage, { src: source, alt: source }) }) }));
const thumbnail = () => document.querySelector<HTMLImageElement>("#root img")!;
const dialog = () => document.querySelector<HTMLElement>("[role=dialog]");
const open = async (key?: string) => {
  await act(async () => {
    thumbnail().dispatchEvent(key ? new dom.window.KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }) : new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  });
};
await act(async () => render("first"));
assert.equal(thumbnail().getAttribute("role"), "button");
assert.equal(thumbnail().tabIndex, 0);
const calls = state.__tauriBridgeCalls.length;
await open();
assert.equal(dialog()?.querySelector("img")?.getAttribute("src"), "data:image/png;base64,first.png", "preview uses the already resolved image");
assert.equal(dialog()?.getAttribute("aria-label"), "first.png");
assert.equal(state.__tauriBridgeCalls.length, calls, "click does not reread a raw local path");
await act(async () => dialog()!.querySelector<HTMLImageElement>("img")!.click());
assert.ok(dialog(), "clicking the full image does not close it");
await act(async () => document.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
assert.equal(dialog(), null);
assert.equal(document.body.style.overflow, "");
await open("Enter");
assert.ok(dialog(), "Enter opens the image");
await act(async () => dialog()!.querySelector<HTMLButtonElement>("button")!.click());
assert.equal(dialog(), null, "close button works");
await open(" ");
assert.ok(dialog(), "Space opens the image");
await act(async () => dialog()!.click());
assert.equal(dialog(), null, "backdrop closes preview");
await open();
await act(async () => render("second", "second.png"));
assert.equal(dialog(), null, "session switching closes the old image");
await act(async () => render("first"));
assert.equal(dialog(), null, "returning to a session does not reopen the old preview");

let linkActivations = 0;
await act(async () => root.render(createElement(LocaleProvider, { children: createElement(TauriImageScope, {
  sessionId: "first", children: createElement("a", { href: "https://example.org/linked-image", onClick: () => { linkActivations++; } }, createElement(MarkdownImage, { src: "first.png", alt: "first.png" })),
}) })));
const linkedClick = new dom.window.MouseEvent("click", { bubbles: true, cancelable: true });
await act(async () => thumbnail().dispatchEvent(linkedClick));
assert.ok(dialog(), "linked images still open the preview");
assert.equal(linkedClick.defaultPrevented, true, "preview clicks cancel the parent link's navigation");
assert.equal(linkActivations, 0, "opening the preview does not also activate the parent link");
await act(async () => dialog()!.click());
await open();
await act(async () => root.unmount());
assert.equal(dialog(), null);
assert.equal(document.body.style.overflow, "", "unmount cleans up the preview scroll lock");
dom.window.close();
console.log("Tauri image preview: click, keyboard, close actions, session isolation and cleanup OK");
