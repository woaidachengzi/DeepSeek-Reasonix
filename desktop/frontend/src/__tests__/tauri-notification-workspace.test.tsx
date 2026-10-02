// Run with the SVG/CSS/Tauri stub loaders, then --import tsx.
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
(globalThis as typeof globalThis & { isTauri?: boolean }).isTauri = true;
(dom.window as unknown as { __TAURI__?: unknown }).__TAURI__ = { core: { invoke: () => Promise.resolve(null) } };
(dom.window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = { invoke: (command: string) => Promise.resolve(command === "workspace_external_openers" ? [] : null) };
const fixture = globalThis as typeof globalThis & {
  __workbenchSessions: Array<{ sessionId: string; workspaceRoot: string; title: string }>;
  __notificationClicks: Array<{ token: string; sessionId: string }>;
  __notificationTargets: Record<string, { sessionId: string; workspaceRoot: string } | null>;
  __notificationClickListeners: Set<() => void>;
  __tauriBridgeCalls: Array<{ name: string; args: Record<string, unknown> }>;
};
fixture.__workbenchSessions = [{ sessionId: "notification-session", workspaceRoot: "/catalog/project", title: "Notification task" }];
fixture.__notificationClicks = [{ token: "cold-click", sessionId: "notification-session" }];
fixture.__notificationTargets = { "cold-click": { sessionId: "notification-session", workspaceRoot: "/canonical/project" } };

const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { TauriSessionApp } = await import("../tauri/TauriChatWorkspace");
const settle = async () => { for (let index = 0; index < 4; index += 1) await new Promise(resolve => setTimeout(resolve, 0)); };
const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(React.createElement(TauriSessionApp)); await settle(); });
const calls = fixture.__tauriBridgeCalls;
const switches = () => calls.filter(call => call.name === "bridge_switch_session");
assert.equal(switches().length, 1, "a queued cold-start click is opened once after directory readiness");
assert.deepEqual(switches()[0].args, { sessionId: "notification-session", workspaceRoot: "/canonical/project" }, "native canonical root wins over stale catalog metadata");
assert.equal(fixture.__notificationClicks.length, 0, "accepted navigation acknowledges the native click");
assert.equal(fixture.__notificationClickListeners.size, 1);

await act(async () => {
  fixture.__notificationClicks = [{ token: "stale-click", sessionId: "deleted-session" }];
  fixture.__notificationTargets["stale-click"] = null;
  fixture.__notificationClickListeners.forEach(callback => callback());
  await settle();
});
assert.equal(switches().length, 1, "a deleted target does not create or switch a session");
assert.equal(fixture.__notificationClicks.length, 0, "a stale target does not block the queue");

for (const composition of [{ isComposing: true }, { keyCode: 229 }]) {
  const event = new dom.window.KeyboardEvent("keydown", { key: ",", ctrlKey: true, bubbles: true, cancelable: true, ...composition });
  await act(async () => { document.dispatchEvent(event); await settle(); });
  assert.equal(document.querySelector('.tauri-settings-overlay'), null, "IME keys do not open the underlying settings surface");
  assert.equal(event.defaultPrevented, false, "the input method retains the composition key");
}
await act(async () => {
  document.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: ",", ctrlKey: true, bubbles: true }));
  await settle();
});
assert.ok(document.querySelector('.tauri-settings-overlay'), "settings opens before the same-session notification");
for (const composition of [{ isComposing: true }, { keyCode: 229 }]) {
  await act(async () => {
    document.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true, ...composition }));
    await settle();
  });
  assert.ok(document.querySelector('.tauri-settings-overlay'), "IME cancellation does not close settings");
}
await act(async () => {
  fixture.__notificationClicks = [{ token: "same-session", sessionId: "notification-session" }];
  fixture.__notificationTargets["same-session"] = { sessionId: "notification-session", workspaceRoot: "/canonical/project" };
  fixture.__notificationClickListeners.forEach(callback => callback());
  fixture.__notificationClickListeners.forEach(callback => callback());
  await settle();
});
assert.equal(document.querySelector('.tauri-settings-overlay'), null, "same-session clicks return to the conversation");
assert.equal(switches().length, 1, "same-session clicks do not restart the session");
assert.equal(fixture.__notificationClicks.length, 0);
assert.ok(!calls.some(call => /bridge_(approve|answer|submit)/.test(call.name)), "notification clicks never approve, answer, or submit agent actions");
await act(async () => { root.unmount(); });
assert.equal(fixture.__notificationClickListeners.size, 0, "unmount removes the native listener");
fixture.__notificationClicks = [{ token: "after-unmount", sessionId: "notification-session" }];
fixture.__notificationClickListeners.forEach(callback => callback());
assert.equal(fixture.__notificationClicks.length, 1, "clicks remain available for the next mounted workspace");
console.log("Tauri notification workspace navigation passed");
