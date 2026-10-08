import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerImage } from "../lib/bridgeProtocol.generated";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement,
  localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window),
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const { createElement, act, useContext } = await import("react");
const { createRoot } = await import("react-dom/client");
const { MarkdownImage } = await import("../components/MarkdownImage");
const { MarkdownImageResolverContext, MarkdownImagePreviewContext } = await import("../components/MarkdownImageContext");
const { TauriRemoteImageScope } = await import("../tauri/TauriRemoteImageScope");
const { LocaleProvider } = await import("../lib/i18n");
await import("../components/ImageViewer");
const pixels = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jk1kAAAAASUVORK5CYII=";
const gates: { path: string; source: string; resolve: (image: BridgeRemoteControllerImage) => void; reject: (error: unknown) => void }[] = [];
const lease: RemoteControllerLease = {
  ready: Promise.resolve({ id: "owned", name: "owned", workspace: "/remote", readOnly: true }),
  release: () => {}, sessions: async () => [], sessionView: async () => { throw new Error("unused"); },
  sessionImage: (path, source) => new Promise((resolve, reject) => gates.push({ path, source, resolve, reject })),
};
let resolver: React.ContextType<typeof MarkdownImageResolverContext>;
let preview: React.ContextType<typeof MarkdownImagePreviewContext>;
function Probe() {
  resolver = useContext(MarkdownImageResolverContext);
  preview = useContext(MarkdownImagePreviewContext);
  return createElement(MarkdownImage, { src: "remote.png", alt: "remote screenshot" });
}
const root = createRoot(document.getElementById("root")!);
const tick = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const render = async (path: string, owner = lease) => act(async () => {
  root.render(createElement(LocaleProvider, { children: createElement(TauriRemoteImageScope, {
    lease: owner, sessionPath: path, surface: path, children: createElement(Probe),
  }) }));
  await tick();
});
const dialog = () => document.querySelector("[role=dialog]");
const image = () => document.querySelector<HTMLImageElement>("#root img");
await render("/A");
assert.equal(gates.length, 1, "layout ownership is live before child passive image resolution");
assert.deepEqual({ path: gates[0].path, source: gates[0].source }, { path: "/A", source: "remote.png" });
const oldResolver = resolver!;
const oldPreview = preview!;
await render("/B");
assert.equal(image(), null);
assert.deepEqual(await oldResolver("late.png"), { url: "", errorCode: "remote-preview-unavailable" });
assert.equal(gates.length, 2, "stale resolver dispatches zero requests");
await act(async () => { gates[0].resolve({ url: pixels, mime: "image/png" }); await tick(); });
assert.equal(image(), null, "old completion cannot paint in B");
await act(async () => { gates[1].resolve({ url: pixels, mime: "image/png" }); await tick(); });
assert.equal(image()?.getAttribute("src"), pixels);
const before = gates.length;
await act(async () => { image()!.click(); await tick(); });
assert.equal(dialog()?.querySelector("img")?.getAttribute("src"), pixels);
assert.equal(gates.length, before, "preview uses resolved pixels without rereading files");
await render("/B");
assert.equal(gates.length, before, "same-scope refresh keeps resolver and cached pixels stable");
assert.ok(dialog(), "same-scope refresh keeps preview");
await render("/A");
assert.equal(dialog(), null, "switch closes the preview immediately");
await act(async () => { oldPreview({ url: pixels }); await tick(); });
assert.equal(dialog(), null, "A -> B -> A does not revive the old scope or old click");
await act(async () => { gates[2].reject(new Error("PRIVATE FAILURE")); await tick(); });
assert.equal(image(), null);
assert.equal(document.querySelector(".md-image-fallback")?.getAttribute("title"), "remote-preview-unavailable");
assert.ok(!document.body.textContent?.includes("PRIVATE"));
await act(async () => { preview!({ url: "https://private.invalid/image.png" }); await tick(); });
assert.equal(dialog(), null, "direct network URL is not a preview grant");
const replaced = { ...lease };
await render("/A", replaced);
assert.equal(gates.length, 4, "replaced lease obtains a fresh image scope even for same path");
await act(async () => { gates[3].resolve({ url: pixels, mime: "image/png" }); await tick(); });
await act(async () => { image()!.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Enter", bubbles: true })); await tick(); });
assert.ok(dialog(), "keyboard opens preview");
let parentEscapes = 0;
const parentEscape = (event: KeyboardEvent) => { if (event.key === "Escape") ++parentEscapes; };
window.addEventListener("keydown", parentEscape);
const escape = new dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
await act(async () => document.dispatchEvent(escape));
assert.equal(dialog(), null);
assert.equal(escape.defaultPrevented, true);
assert.equal(parentEscapes, 0, "preview Escape must not also close its underlying settings/workspace");
window.removeEventListener("keydown", parentEscape);
await act(async () => { image()!.click(); await tick(); });
const lastResolver = resolver!;
await act(async () => root.unmount());
assert.equal(dialog(), null);
assert.equal(document.body.style.overflow, "", "unmount restores viewer scroll lock");
assert.deepEqual(await lastResolver("after-unmount.png"), { url: "", errorCode: "remote-preview-unavailable" });
assert.equal(gates.length, 4);
dom.window.close();
console.log("Remote images: committed scope, stale dispatch/results, ABA, same-scope refresh, lease replacement, pixel-only preview, keyboard and cleanup passed");
