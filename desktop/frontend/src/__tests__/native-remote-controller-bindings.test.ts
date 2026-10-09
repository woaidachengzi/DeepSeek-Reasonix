import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<body></body>", { url: "http://localhost/" });
const previousWindow = globalThis.window;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
const id = "AAAAAAAAAAAAAAAAAAAAAA";
const view = { id, name: "owned", workspace: "/resolved-owned", readOnly: true };
const sessionPath = "/remote/中文 &+?#.jsonl";
const snapshot = {protocolVersion:1,sessionPath,readOnly:true,ownership:"saved",current:false,modelRef:"",label:"",history:[{id:"backend-user",role:"user",content:"question"}]};
const calls: { command: string; args: Record<string, unknown> }[] = [];
let fallback = 0;
let refuse = false;
Object.assign(window, {
  go: { main: { App: { OpenRemoteController: () => { fallback++; } } } },
  __TAURI_INTERNALS__: {
    async invoke(command: string, args: Record<string, unknown>) {
      calls.push({ command, args });
      if (refuse) throw new Error("native refusal");
      if (command === "bridge_remote_controller_attach") return { protocolVersion: 1, controller: view };
      if (command === "bridge_remote_controller_sessions") return { protocolVersion: 1, controller: view, sessions: [] };
      if (command === "bridge_remote_controller_session_view") return {protocolVersion:1,controller:view,view:snapshot};
      if (command === "bridge_remote_controller_session_image") return {protocolVersion:1,controller:view,view:{protocolVersion:1,sessionPath,workspace:view.workspace,image:{url:"",errorCode:"not-found"}}};
      if(command === "bridge_remote_controller_session_cancel")return {protocolVersion:1,controller:view,receipt:{protocolVersion:1,sessionPath,runtimeEpoch:"instance",turnId:"turn",cancelled:true}};
      if (command === "bridge_remote_controller_close") return { protocolVersion: 1, closed: true };
      throw new Error("unexpected native command");
    },
  },
});
const tick = async () => { for (let i = 0; i < 16; i++) await Promise.resolve(); };
try {
  const { nativeRemoteControllers } = await import("../lib/nativeRemoteControllers");
  const lease = nativeRemoteControllers.acquire("owned", "requested-alias");
  assert.deepEqual(await lease.ready, view);
  assert.deepEqual(await lease.sessions(), []);
  assert.deepEqual(await lease.sessionView(sessionPath),snapshot);
  assert.deepEqual(await lease.sessionImage(sessionPath,"image.png"),{url:"",errorCode:"not-found"});
  assert.deepEqual(await lease.sessionCancel!({sessionPath,runtimeEpoch:"instance",turnId:"turn"}),{protocolVersion:1,sessionPath,runtimeEpoch:"instance",turnId:"turn",cancelled:true});
  lease.release();
  await tick();
  assert.deepEqual(calls, [
    { command: "bridge_remote_controller_attach", args: { request: { name: "owned", workspace: "requested-alias" } } },
    { command: "bridge_remote_controller_sessions", args: { request: { controllerId: id } } },
    { command: "bridge_remote_controller_session_view", args: {request:{controllerId:id,sessionPath}} },
    { command: "bridge_remote_controller_session_image", args: {request:{controllerId:id,sessionPath,source:"image.png"}} },
    {command:"bridge_remote_controller_session_cancel",args:{request:{controllerId:id,sessionPath,runtimeEpoch:"instance",turnId:"turn"}}},
    { command: "bridge_remote_controller_close", args: { request: { controllerId: id } } },
  ], "renderer sends only narrow typed requests, not a URL, token or local session identity");
  refuse = true;
  const denied = nativeRemoteControllers.acquire("owned", "/denied");
  await assert.rejects(denied.ready, /native refusal/);
  denied.release();
  await tick();
  assert.equal(fallback, 0, "native refusal must not fall back to Wails or browser navigation");
  nativeRemoteControllers.invalidateHost("owned");
  console.log("Native remote controller bindings: typed payloads, resolved workspace, release and no fallback passed");
} finally {
  if (previousWindow) globalThis.window = previousWindow;
  else delete (globalThis as { window?: unknown }).window;
  dom.window.close();
}
