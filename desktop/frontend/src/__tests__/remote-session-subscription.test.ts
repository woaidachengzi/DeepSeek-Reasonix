import assert from "node:assert/strict";
import { openRemoteSessionSubscription, type RemoteSubscriptionTransport } from "../lib/remoteSessionSubscription";
const id = "AAAAAAAAAAAAAAAAAAAAAA";
const controller = {id,name:"owned",workspace:"/resolved",readOnly:true};
const request = {controllerId:id,sessionPath:"/owned.jsonl",surfaceId:"surface",generation:1};
const identity = {...request,protocolVersion:1,subscriptionId:"BBBBBBBBBBBBBBBBBBBBBA",sidecarInstanceId:"owner"};
const frame = {protocolVersion:1,controller,sessionPath:request.sessionPath,event:{kind:"text",sessionPath:request.sessionPath,text:"answer"}};
const tick = async () => { for (let i=0;i<10;i++) await Promise.resolve(); };
function fixture() {
  const callbacks = new Map<string,(payload:unknown)=>void>();
  const order: string[] = [];
  let resolve!: (value:unknown)=>void;
  const result = new Promise<unknown>(done => { resolve = done; });
  const transport:RemoteSubscriptionTransport = {
    listen:async (name,callback) => { order.push(name); callbacks.set(name,callback); return () => { order.push(`unlisten:${name}`); callbacks.delete(name); }; },
    subscribe:async actual => { assert.deepEqual(actual,request); order.push("subscribe"); return result; },
    unsubscribe:async actual => { order.push(`close:${actual}`); },
  };
  const output: unknown[] = [];
  const subscription = openRemoteSessionSubscription(transport,controller,request,{state:value=>output.push(value),event:value=>output.push(value.event.text)});
  const emit = (state:string) => callbacks.get("bridge:remote-session-state")?.({protocolVersion:1,subscription:identity,state});
  const event = (value:unknown = frame, owner:unknown = identity) => callbacks.get("bridge:remote-session-event")?.({protocolVersion:1,subscription:owner,frame:value});
  return {callbacks,order,resolve,output,subscription,emit,event};
}
{
  const f = fixture(); await tick();
  assert.deepEqual(f.order,["bridge:remote-session-state","bridge:remote-session-event","subscribe"]);
  f.emit("opening"); f.emit("ready"); f.event();
  assert.deepEqual(f.output,[],"native enqueue cannot beat exact invoke receipt");
  f.resolve({...identity,token:"PRIVATE"});
  assert.deepEqual(await f.subscription.receipt,identity,"receipt explicitly drops unknown fields");
  assert.deepEqual(f.output,["opening","ready","answer"]);
  const late = f.callbacks.get("bridge:remote-session-event")!;
  f.event(frame,{...identity,generation:0});
  f.event(frame,{...identity,subscriptionId:id});
  f.event(frame,{...identity,sidecarInstanceId:"old-owner"});
  assert.equal(f.output.length,3);
  f.emit("ended"); f.subscription.dispose();
  late({protocolVersion:1,subscription:identity,frame});
  assert.deepEqual(f.output,["opening","ready","answer","ended"]);
  assert.equal(f.order.filter(item=>item.startsWith("close:")).length,1);
}
{
  const f = fixture(); await tick(); f.subscription.dispose(); f.resolve(identity);
  assert.equal(await f.subscription.receipt,null);
  assert.deepEqual(f.output,[]);
  assert.equal(f.callbacks.size,0);
  assert.equal(f.order[f.order.length - 1],`close:${identity.subscriptionId}`,"late receipt closes exact owned native subscription");
}
for (const bad of [{...frame,sessionPath:"/foreign"},{...frame,controller:{...controller,workspace:"/foreign"}}]) {
  const f = fixture(); await tick(); f.resolve(identity); await f.subscription.receipt;
  f.emit("ready"); f.event(bad);
  assert.deepEqual(f.output,["ready","failed"]);
  assert.equal(f.callbacks.size,0);
}
{
  const f = fixture(); await tick(); f.resolve(identity); await f.subscription.receipt;
  f.event(); assert.deepEqual(f.output,["failed"],"event before ready is rejected");
}
{
  const f = fixture(); await tick();
  for(let i=0;i<257;i++) f.emit("opening");
  assert.deepEqual(f.output,["failed"]);
  f.resolve(identity); await f.subscription.receipt;
  assert.equal(f.order[f.order.length - 1],`close:${identity.subscriptionId}`);
}
{
  let subscribed = false; let released = false;
  const subscription = openRemoteSessionSubscription({
    listen:async name=> { if(name.endsWith("event")) throw new Error("PRIVATE"); return ()=>{released=true;}; },
    subscribe:async()=>{subscribed=true;return identity;},unsubscribe:async()=>{},
  },controller,request,{state:value=>assert.equal(value,"failed"),event:()=>assert.fail("unexpected frame")});
  assert.equal(await subscription.receipt,null); assert.equal(released,true); assert.equal(subscribed,false);
}
{
  const unhandled: unknown[] = [];
  const onUnhandled = (error: unknown) => { unhandled.push(error); };
  process.on("unhandledRejection", onUnhandled);
  try {
    let released = 0;
    const subscription = openRemoteSessionSubscription({
      listen:async () => async () => { released++; throw new Error("closed WebView"); },
      subscribe:async () => identity,
      unsubscribe:async () => {},
    },controller,request,{state:()=>{},event:()=>{}});
    await subscription.receipt;
    subscription.dispose(); subscription.dispose();
    await new Promise<void>(resolve => setImmediate(resolve));
    assert.equal(released,2,"both native listeners are released exactly once");
    assert.deepEqual(unhandled,[],"async unlisten failures during teardown are contained");

    let resolveListener!: (off: () => Promise<void>) => void;
    let subscribed = false;
    const pending = openRemoteSessionSubscription({
      listen:async () => new Promise<() => Promise<void>>(resolve => { resolveListener = resolve; }),
      subscribe:async () => { subscribed = true; return identity; },
      unsubscribe:async () => {},
    },controller,request,{state:()=>assert.fail("disposed listener cannot publish"),event:()=>assert.fail("disposed listener cannot publish")});
    pending.dispose();
    resolveListener(async () => { released++; throw new Error("closed WebView"); });
    assert.equal(await pending.receipt,null);
    await new Promise<void>(resolve => setImmediate(resolve));
    assert.equal(released,3,"late listener registration is also released");
    assert.equal(subscribed,false,"dispose during listener admission prevents native subscribe");
    assert.deepEqual(unhandled,[]);
  } finally {
    process.off("unhandledRejection", onUnhandled);
  }
}
{
  const emitted: string[] = [];
  let calls = 0;
  const transport:RemoteSubscriptionTransport = {
    listen:async () => { calls++; return ()=>{}; },
    subscribe:async () => { calls++; return identity; },
    unsubscribe:async () => { calls++; },
  };
  for (const generation of [0,-1,1.5,Number.MAX_SAFE_INTEGER+1]) {
    const subscription = openRemoteSessionSubscription(transport,controller,{...request,generation},{state:state=>emitted.push(state),event:()=>assert.fail("invalid admission")});
    assert.equal(await subscription.receipt,null);
  }
  assert.equal(calls,0,"invalid generation cannot install listeners or dispatch native commands");
  assert.deepEqual(emitted,["failed","failed","failed","failed"]);
}
{
  let closes = 0;
  const input = {...request,token:"PRIVATE"};
  const subscription = openRemoteSessionSubscription({
    listen:async () => ()=>{},
    subscribe:async actual => { assert.deepEqual(actual,request); return identity; },
    unsubscribe:() => { closes++; throw new Error("closed native owner"); },
  },controller,input,{state:()=>{},event:()=>{}});
  input.generation = 99; // Capture scope before asynchronous listener admission.
  await subscription.receipt;
  assert.doesNotThrow(()=>subscription.dispose());
  assert.equal(closes,1);
}
console.log("Remote subscription ownership: listener admission, narrow captured scope, receipt queue, generation/owner fences, async teardown, cancellation, bounds and fixed failures passed");
