import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import { clearMocks, mockIPC } from "@tauri-apps/api/mocks";
import { emit } from "@tauri-apps/api/event";

const dom = new JSDOM("<body></body>", { url: "http://localhost/" });
const previousWindow = globalThis.window;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
const request = {controllerId:"AAAAAAAAAAAAAAAAAAAAAA",sessionPath:"/owned.jsonl",surfaceId:"surface",generation:1};
const identity = {...request,protocolVersion:1,subscriptionId:"BBBBBBBBBBBBBBBBBBBBBA",sidecarInstanceId:"owner"};
const calls: {command:string;args:unknown}[] = [];
let fallback = 0;
let refuse = false;
Object.assign(window,{go:{main:{App:{SubscribeRemoteSession:()=>{fallback++;}}}}});
mockIPC((command,args) => {
  calls.push({command,args});
  if (refuse) throw new Error("native refusal");
  if (command === "bridge_remote_controller_subscribe") return identity;
  if (command === "bridge_remote_controller_unsubscribe") return;
  throw new Error(`unexpected command: ${command}`);
},{shouldMockEvents:true});
try {
  const { nativeRemoteSessionTransport } = await import("../lib/nativeRemoteSessionSubscription");
  const received: unknown[] = [];
  const unlisten = await nativeRemoteSessionTransport.listen("bridge:remote-session-state",payload=>received.push(payload));
  assert.deepEqual(await nativeRemoteSessionTransport.subscribe(request),identity);
  const notice = {protocolVersion:1,subscription:identity,state:"ready"};
  await emit("bridge:remote-session-state",notice);
  assert.deepEqual(received,[notice],"adapter forwards the event payload, without the native event wrapper");
  await unlisten();
  await nativeRemoteSessionTransport.unsubscribe(identity.subscriptionId);
  assert.deepEqual(calls,[
    {command:"bridge_remote_controller_subscribe",args:{request}},
    {command:"bridge_remote_controller_unsubscribe",args:{request:{subscriptionId:identity.subscriptionId}}},
  ],"native commands use narrow request envelopes");
  refuse = true;
  await assert.rejects(nativeRemoteSessionTransport.subscribe(request),/native refusal/);
  await assert.rejects(nativeRemoteSessionTransport.unsubscribe(identity.subscriptionId),/native refusal/);
  assert.equal(fallback,0,"native refusal must not fall back to Wails");
  console.log("Native remote subscription bindings: typed invoke envelopes, payload forwarding, listener release and no fallback passed");
} finally {
  clearMocks();
  if (previousWindow) globalThis.window = previousWindow;
  else delete (globalThis as {window?:unknown}).window;
  dom.window.close();
}
