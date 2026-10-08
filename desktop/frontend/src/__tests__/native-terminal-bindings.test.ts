import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<body></body>", { url: "http://localhost/" });
const previousWindow = globalThis.window;
const host = globalThis as typeof globalThis & { isTauri?: boolean };
const previousTauri = host.isTauri;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
host.isTauri = true;
const id = "a".repeat(32);
const calls: { command: string; args: Record<string, unknown> }[] = [];
let fallback = 0;
let failInput = false;
let next = 0;
const callbacks = new Map<number, (value: unknown) => void>();
const listeners = new Map<number, { event: string; handler: number }>();
Object.assign(window, {
  go: { main: { App: { WriteTerminalForTab: () => { fallback++; }, TerminalWorkspaceForTab: () => { fallback++; } } } },
  __TAURI_INTERNALS__: {
    transformCallback(cb: (value: unknown) => void) { const key = ++next; callbacks.set(key, cb); return key; },
    unregisterCallback(key: number) { callbacks.delete(key); },
    async invoke(command: string, args: Record<string, unknown>) {
      calls.push({ command, args });
      if (command === "plugin:event|listen") { const key = ++next; listeners.set(key, args as { event: string; handler: number }); return key; }
      if (command === "plugin:event|unlisten") { listeners.delete(args.eventId as number); return; }
      if (command === "bridge_terminal_workspace") return { protocolVersion: 1, workspace: { available: true, readOnly: false, sessions: [{ id, title: "sh", cwd: "/owned", shell: "/bin/sh", createdAt: 1, running: true }], shells: [] } };
      if (command === "bridge_terminal_output") return { protocolVersion: 1, output: { id, start: 0, end: 0, data: "" } };
      if (command === "bridge_terminal_input" && failInput) throw new Error("native refusal");
      return { protocolVersion: 1 };
    },
  },
  __TAURI_EVENT_PLUGIN_INTERNALS__: { unregisterListener() {} },
});
try {
  const { app } = await import("../lib/bridge");
  const { nativeTerminals } = await import("../lib/nativeTerminals");
  const { startTerminalEventBridge, __resetTerminalEventBus } = await import("../lib/terminalEvents");
  const { registerTerminalSink } = await import("../lib/terminalSink");
  __resetTerminalEventBus();
  const stop = nativeTerminals.retain("owned");
  const offFirst = startTerminalEventBridge();
  const offSecond = startTerminalEventBridge();
  await app.TerminalWorkspaceForTab("owned");
  assert.equal([...listeners.values()].filter(item => item.event === "bridge:terminal-event").length, 1);
  const received: number[] = [];
  const sink = registerTerminalSink(id, bytes => received.push(...bytes));
  for (const [key, item] of listeners) if (item.event === "bridge:terminal-event") callbacks.get(item.handler)!({ id: key, payload: { protocolVersion: 1, sessionId: "owned", eventKind: "terminal_output", payload: { id, start: 0, end: 3, data: "5L2g" } } });
  assert.deepEqual(received, [0xe4, 0xbd, 0xa0]);
  await app.WriteTerminalForTab("owned", id, "你\r");
  const request = calls.find(call => call.command === "bridge_terminal_input")!.args.request as Record<string, unknown>;
  assert.deepEqual(Object.keys(request).sort(), ["data", "requestId", "sessionId", "terminalId"]);
  assert.equal(Buffer.from(String(request.data), "base64").toString(), "你\r");
  failInput = true;
  await assert.rejects(app.WriteTerminalForTab("owned", id, "refused"), /native refusal/);
  assert.equal(fallback, 0, "native failure must never reach Wails or browser mock");
  offFirst(); offSecond(); sink.dispose(); stop();
  for (let i = 0; i < 12; i++) await Promise.resolve();
  assert.equal(listeners.size, 0, "all native listeners are released with the owner");
  __resetTerminalEventBus();
  console.log("Native terminal bindings: priority, no mock fallback, typed input, shared raw-byte subscription and cleanup passed");
} finally {
  if (previousWindow) globalThis.window = previousWindow;
  else delete (globalThis as { window?: unknown }).window;
  if (previousTauri === undefined) delete host.isTauri;
  else host.isTauri = previousTauri;
  dom.window.close();
}
