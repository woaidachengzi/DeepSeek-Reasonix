// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
const nativeCalls: Array<{ command: string; args: Record<string, unknown> }> = [];
let openable = true;
(dom.window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
  invoke: (command: string, args: Record<string, unknown>) => {
    nativeCalls.push({ command, args });
    if (command === "workspace_external_openers") return Promise.resolve(openable ? { openers: [{ id: "finder", name: "Finder", kind: "file-manager" }], preferred: "finder", workspaceOpenable: true } : { openers: [], preferred: "", workspaceOpenable: false });
    return Promise.resolve(null);
  },
};
const fixture = globalThis as typeof globalThis & { __workbenchSessions: unknown[]; __tauriBridgeCalls: Array<{ name: string; args: Record<string, unknown> }> };
fixture.__workbenchSessions = [{ sessionId: "global-conversation", title: "Global task" }, { sessionId: "unavailable-conversation", title: "Unavailable task" }];
const React = await import("react"); const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const root = createRoot(document.getElementById("root")!);
const settle = async () => { for (let index = 0; index < 4; index += 1) await new Promise(resolve => setTimeout(resolve, 0)); };
await act(async () => { root.render(React.createElement(TauriSessionApp)); await settle(); });
assert.equal(document.querySelector('.external-opener__primary'), null, "a draft has no session workspace capability");
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-sidebar__session[title="global-conversation"]')!.click(); await settle(); });
assert.ok(fixture.__tauriBridgeCalls.some(call => call.name === "bridge_switch_session" && call.args.sessionId === "global-conversation" && call.args.workspaceRoot === undefined), "Global navigation has no guessed project root");
assert.ok(nativeCalls.some(call => call.command === "workspace_external_openers" && call.args.sessionId === "global-conversation"), "a rootless Global conversation asks host for the real capability");
const primary = document.querySelector<HTMLButtonElement>('.external-opener__primary'); assert.ok(primary);
await act(async () => { primary.click(); await settle(); });
assert.deepEqual(nativeCalls.filter(call => call.command === "open_workspace_external"), [{ command: "open_workspace_external", args: { sessionId: "global-conversation", id: "finder" } }], "only session and installed-app identity cross IPC");
openable = false;
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-sidebar__session[title="unavailable-conversation"]')!.click(); await settle(); });
assert.equal(document.querySelector('.external-opener__primary'), null, "an unavailable workspace cannot keep the previous session's launch control");
assert.equal(nativeCalls.filter(call => call.command === "open_workspace_external").length, 1);
await act(async () => { root.unmount(); });
console.log("Tauri Global workspace opener passed");
