import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<body></body>",{url:"http://localhost/"});
const previous = globalThis.window;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
const calls:{name:string;args:Record<string,unknown>}[]=[];
const callbacks = new Map<number,(value:unknown)=>void>();
let next = 1;
let fallback = 0;
Object.assign(window,{
  go:{main:{App:{SubscribeRemoteSession:()=>{fallback++;}}}},
  __TAURI_EVENT_PLUGIN_INTERNALS__:{unregisterListener:()=>{}},
  __TAURI_INTERNALS__:{
    transformCallback(callback:(value:unknown)=>void) { const id=next++;callbacks.set(id,callback);return id; },
    async invoke(name:string,args:Record<string,unknown>) {
      calls.push({name,args});
      if(name === "plugin:event|listen") return 7;
      if(name === "plugin:event|unlisten") return;
      if(name === "bridge_remote_controller_subscribe") return {subscriptionId:"owned"};
      if(name === "bridge_remote_controller_unsubscribe") return;
      if(name === "bridge_remote_controller_snapshot") return {protocolVersion:1};
      throw new Error("unexpected invoke");
    },
  },
});
try {
  const {nativeRemoteSessionTransport:transport} = await import("../lib/nativeRemoteSessionSubscription");
  const seen:unknown[]=[];
  const off = await transport.listen("bridge:remote-session-event",value=>seen.push(value));
  assert.equal(calls[0].name,"plugin:event|listen");
  assert.deepEqual(calls[0].args.target,{kind:"AnyLabel",label:"main"});
  callbacks.get(calls[0].args.handler as number)!({id:7,event:"bridge:remote-session-event",payload:{owned:true}});
  assert.deepEqual(seen,[{owned:true}]);
  await off();
  const request = {controllerId:"controller",sessionPath:"/owned.jsonl",surfaceId:"surface",generation:2};
  await transport.subscribe({...request,token:"PRIVATE",url:"https://invalid"} as typeof request);
  assert.deepEqual(calls[calls.length-1],{name:"bridge_remote_controller_subscribe",args:{request}});
  await transport.unsubscribe("exact-subscription");
  assert.deepEqual(calls[calls.length-1],{name:"bridge_remote_controller_unsubscribe",args:{request:{subscriptionId:"exact-subscription"}}});
  const before = calls.length;
  await assert.rejects(transport.listen("bridge:global-event",()=>{}),/invalid remote subscription event/);
  assert.equal(calls.length,before);
  assert.equal(fallback,0);
  const {nativeRemoteSnapshotTransport:snapshot} = await import("../lib/nativeRemoteSessionSnapshot");
  await snapshot.snapshot("owned-subscription");
  assert.deepEqual(calls[calls.length-1],{name:"bridge_remote_controller_snapshot",args:{request:{subscriptionId:"owned-subscription"}}});
  await snapshot.snapshot("owned-subscription","owned-continuation");
  assert.deepEqual(calls[calls.length-1],{name:"bridge_remote_controller_snapshot",args:{request:{subscriptionId:"owned-subscription",continuation:"owned-continuation"}}});
  assert.equal(fallback,0);
  console.log("Native remote subscription bindings: main-target listeners, narrow commands and no fallback passed");
} finally {
  if(previous) globalThis.window=previous;
  else delete (globalThis as {window?:unknown}).window;
  dom.window.close();
}
