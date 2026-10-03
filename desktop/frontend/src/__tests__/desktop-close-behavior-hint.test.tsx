import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { DesktopCloseBehaviorHint } from "../components/DesktopCloseBehaviorHint";
import { onDesktopOpenSettings, onDesktopShellStatus, type AppBindings } from "../lib/bridge";
import assert from "node:assert/strict";

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
window.go = {
  main: {
    App: {
      GetDesktopShellStatus: async () => ({
        trayState: "unavailable",
        backgroundCloseAvailable: false,
        reason: "no_host",
      }),
    } as Partial<AppBindings> as AppBindings,
  },
};

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("missing root");
const root = createRoot(rootElement);
await act(async () => {
  root.render(<DesktopCloseBehaviorHint backgroundSelected hint="Keep running after close." unavailableHint="Closing this time will quit." />);
  await Promise.resolve();
});
if (rootElement.textContent !== "Keep running after close.Closing this time will quit.") {
  throw new Error(`unexpected close fallback hint: ${JSON.stringify(rootElement.textContent)}`);
}
await act(async () => root.unmount());

// A native tray transition is newer than the initial async status query.
let resolveStatus!: (value: { trayState: "unavailable"; backgroundCloseAvailable: false }) => void;
const pendingStatus = new Promise<{ trayState: "unavailable"; backgroundCloseAvailable: false }>((resolve) => { resolveStatus = resolve; });
let statusEvent!: (...values: unknown[]) => void;
let unsubscribed = 0;
window.runtime = {
  EventsOn(name: string, listener: (...values: unknown[]) => void) {
    assert.equal(name, "desktop:shell-status");
    statusEvent = listener;
    return () => { unsubscribed++; };
  },
} as NonNullable<Window["runtime"]>;
window.go.main.App.GetDesktopShellStatus = () => pendingStatus;
const nextRoot = createRoot(rootElement);
await act(async () => {
  nextRoot.render(<DesktopCloseBehaviorHint backgroundSelected hint="Keep running after close." unavailableHint="Closing this time will quit." />);
});
await act(async () => {
  statusEvent({ trayState: "ready", backgroundCloseAvailable: true });
  resolveStatus({ trayState: "unavailable", backgroundCloseAvailable: false });
  await pendingStatus;
});
assert.equal(rootElement.textContent, "Keep running after close.", "a delayed snapshot must not overwrite the newer native tray event");
await act(async () => nextRoot.unmount());
assert.equal(unsubscribed, 1, "the host subscription is removed on unmount");

// Native queued delivery must be inert after the settings consumer leaves.
let settingsEvent!: (...values: unknown[]) => void;
let settingsOpened = 0;
let settingsUnsubscribed = 0;
window.runtime.EventsOn = (name, listener) => {
  assert.equal(name, "app:open-settings");
  settingsEvent = listener;
  return () => { settingsUnsubscribed++; };
};
const stopSettings = onDesktopOpenSettings(() => { settingsOpened++; });
settingsEvent();
assert.equal(settingsOpened, 1);
stopSettings();
stopSettings();
settingsEvent();
assert.equal(settingsOpened, 1, "already queued host settings events cannot reopen settings after disposal");
assert.equal(settingsUnsubscribed, 1, "host event disposal is idempotent");
delete window.runtime;
onDesktopOpenSettings(() => { throw new Error("browser preview must not fabricate a settings event"); })();
onDesktopShellStatus(() => { throw new Error("browser preview must not fabricate tray status"); })();
console.log("desktop close behavior hint: ok");
