import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { useTauriWorkspaceLayout } from "../tauri/useTauriWorkspaceLayout";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window), cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window), IS_REACT_ACT_ENVIRONMENT: true });
let viewportWidth = 1440, renders = 0, layouts = 0;
Object.defineProperty(dom.window.HTMLElement.prototype, "getBoundingClientRect", { value() {
  return { width: this.classList.contains("tauri-sidebar") ? 254 : viewportWidth };
} });
function Fixture({ open = true, sidebar = true }: { open?: boolean; sidebar?: boolean }) {
  renders++;
  const layout = useTauriWorkspaceLayout(open, sidebar, "default", () => { layouts++; });
  return <main ref={layout.shellRef} style={layout.style} data-docked={String(layout.docked)}>
    <aside className="tauri-sidebar" />{layout.docked && <button type="button" {...layout.separatorProps} />}
  </main>;
}
const root = createRoot(document.getElementById("root")!);
const render = async (open = true, sidebar = true) => { await act(async () => root.render(<Fixture open={open} sidebar={sidebar} />)); };
const shell = () => document.querySelector("main")!;
const separator = () => document.querySelector<HTMLButtonElement>("button")!;
async function pointer(type: string, x: number, id = 1, target: EventTarget = window) {
  const event = new dom.window.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, button: 0 });
  Object.defineProperty(event, "pointerId", { value: id });
  await act(async () => target.dispatchEvent(event));
}
try {
  localStorage.setItem("reasonix.tauri.workspace-width.v1", "invalid");
  await render();
  assert.equal(shell().dataset.docked, "true");
  assert.equal(separator().getAttribute("aria-valuenow"), "520");
  await pointer("pointerdown", 920, 1, separator());
  const before = renders;
  await pointer("pointerdown", 700, 2, separator());
  await pointer("pointermove", 800, 2);
  await pointer("pointermove", 800);
  await new Promise(resolve => requestAnimationFrame(resolve));
  assert.equal(shell().style.getPropertyValue("--tauri-workspace-width"), "640px");
  assert.equal(renders, before, "dragging applies frame-batched CSS without rendering the conversation per move");
  await pointer("pointerup", 800);
  assert.equal(localStorage.getItem("reasonix.tauri.workspace-width.v1"), "640");
  assert.equal(shell().dataset.workspaceResizing, undefined);
  await render(false); await render();
  assert.equal(separator().getAttribute("aria-valuenow"), "640", "reopening retains width");
  await act(async () => separator().dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "End", bubbles: true, cancelable: true })));
  assert.equal(separator().getAttribute("aria-valuenow"), "706", "chat retains its minimum available width");
  await act(async () => separator().dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Home", bubbles: true, cancelable: true })));
  assert.equal(separator().getAttribute("aria-valuenow"), "320");
  await act(async () => separator().dispatchEvent(new dom.window.MouseEvent("dblclick", { bubbles: true })));
  assert.equal(separator().getAttribute("aria-valuenow"), "520");
  await pointer("pointerdown", 920, 1, separator()); await pointer("pointermove", 850);
  await act(async () => window.dispatchEvent(new dom.window.Event("blur")));
  assert.equal(shell().dataset.workspaceResizing, undefined, "blur releases the resize gesture");
  assert.equal(separator().getAttribute("aria-valuenow"), "590");
  viewportWidth = 1000;
  await act(async () => window.dispatchEvent(new dom.window.Event("resize")));
  assert.equal(shell().dataset.docked, "false", "narrow windows fall back to the drawer");
  await render(true, false);
  assert.equal(shell().dataset.docked, "true", "hiding the sidebar can recover enough room for docking");
  assert.equal(separator().getAttribute("aria-valuenow"), "520");
  viewportWidth = 1440;
  await act(async () => window.dispatchEvent(new dom.window.Event("resize")));
  assert.equal(separator().getAttribute("aria-valuenow"), "590", "temporary clamping preserves the preferred width");
  assert(layouts > 0, "layout changes notify the transcript geometry owner");
  await pointer("pointerdown", 900, 1, separator()); await pointer("pointermove", 800);
  await act(async () => root.unmount());
  await pointer("pointermove", 500);
  assert.equal(document.querySelector('[data-workspace-resizing="true"]'), null);
  console.log("PASS Tauri workspace layout: dock/overlay, persistence, frame-batched drag, pointer ownership, keyboard/reset, blur/unmount cleanup, responsive clamping and geometry notification");
} finally { dom.window.close(); }
