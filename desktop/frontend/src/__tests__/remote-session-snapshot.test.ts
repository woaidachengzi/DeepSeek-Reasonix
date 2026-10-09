import assert from "node:assert/strict";
import { openRemoteSessionSnapshot, type RemoteSnapshotTransport, type RemoteDisplayCut } from "../lib/remoteSessionSnapshot";
const id="AAAAAAAAAAAAAAAAAAAAAA", next="AQEBAQEBAQEBAQEBAQEBAQ";
const controller={id,name:"owned",workspace:"/resolved",readOnly:true};
const input={controllerId:id,sessionPath:"/owned.jsonl",surfaceId:"surface",generation:1};
const identity={...input,protocolVersion:1,subscriptionId:next,sidecarInstanceId:"owner"};
const event=(seq:number)=>({kind:"text",seq,turnId:"turn",sessionPath:input.sessionPath,text:`text-${seq}`});
function response(initial=true) {
  return {protocolVersion:1,subscription:identity,snapshot:{protocolVersion:1,controller,projection:{protocolVersion:1,sessionPath:input.sessionPath,readOnly:true,initial,history:initial?[{id:"old",role:"user",content:"old"}]:[],userSuffix:initial?[{id:"new",role:"user",content:"new"}]:[],activeTurnId:"turn",turnStatus:"in_progress",replayAfterSeq:0,replay:{events:[event(initial?1:2)],floorSeq:1,latestSeq:2,nextAfterSeq:initial?1:2,hasMore:initial,runtimeEpoch:"epoch"}},...(initial?{nextPage:id}:{})}};
}
const tick=async()=>{for(let i=0;i<20;i++)await Promise.resolve();};
function fixture() {
  const callbacks=new Map<string,(value:unknown)=>void>();
  const calls:{id:string;next?:string}[]=[], states:string[]=[], live:number[]=[], cuts:RemoteDisplayCut[]=[];
  const reads:{resolve:(value:unknown)=>void;reject:(value:unknown)=>void}[]=[];
  let receipt!:(value:unknown)=>void;
  const transport:RemoteSnapshotTransport={
    listen:async(name,callback)=>{callbacks.set(name,callback);return()=>{callbacks.delete(name);};},
    subscribe:async()=>new Promise(resolve=>{receipt=resolve;}),
    unsubscribe:async()=>{states.push("closed");},
    snapshot:async(id,next)=>{calls.push({id,next});return new Promise((resolve,reject)=>{reads.push({resolve,reject});});},
  };
  const stream=openRemoteSessionSnapshot(transport,controller,input,{state:value=>states.push(value),snapshot:cut=>cuts.push(cut),event:frame=>live.push(frame.seq as number)});
  const state=(state:string)=>callbacks.get("bridge:remote-session-state")?.({protocolVersion:1,subscription:identity,state});
  const emit=(seq:number)=>callbacks.get("bridge:remote-session-event")?.({protocolVersion:1,subscription:identity,frame:{protocolVersion:1,controller,sessionPath:input.sessionPath,event:event(seq)}});
  return {receipt:(value:unknown)=>receipt(value),stream,calls,states,cuts,live,reads,state,emit,callbacks};
}
{
  const f=fixture();await tick();f.state("ready");f.emit(1);await tick();assert.equal(f.calls.length,0,"ready cannot beat exact receipt");
  f.receipt(identity);await tick();assert.deepEqual(f.calls,[{id:next,next:undefined}]);
  f.reads[0].resolve(response());await tick();assert.deepEqual(f.calls[1],{id:next,next:id});assert.equal(f.cuts.length,0,"no partial prefix is published");
  f.emit(3);f.reads[1].resolve(response(false));await f.stream.settled;
  assert.deepEqual(f.cuts[0].events.map(frame=>frame.seq),[1,2]);assert.deepEqual(f.live,[3],"buffered snapshot overlap is not reapplied");
  f.emit(4);assert.deepEqual(f.live,[3,4]);assert.equal(f.states[f.states.length-1],"live");
  f.emit(6);assert.equal(f.states[f.states.length-1],"reconcile");assert.equal(f.callbacks.size,0);assert.equal(f.states.filter(state=>state==="closed").length,1);
}
{
  const f=fixture();await tick();f.receipt(identity);await tick();assert.equal(f.calls.length,0,"receipt is not readiness");
  f.state("ready");await tick();f.stream.dispose();f.reads[0].resolve(response());await f.stream.settled;
  assert.equal(f.cuts.length,0);assert.equal(f.calls.length,1);assert.equal(f.callbacks.size,0);
}
for(const change of ["owner","epoch","cut","gap","duplicate-id","secret-token","page-stall","false-completed"]) {
  const f=fixture();await tick();f.receipt(identity);f.state("ready");await tick();
  const bad=response() as unknown as Record<string,any>;
  if(change==="owner")bad.subscription={...identity,sidecarInstanceId:"other"};
  else if(change==="duplicate-id")bad.snapshot.projection.userSuffix[0].id="old";
  else if(change==="secret-token")bad.snapshot.projection.pageToken="private";
  else if(change==="gap")bad.snapshot.projection.replay.events[0].seq=2;
  else if(change==="page-stall")bad.snapshot.projection.replay.events=[];
  else if(change==="false-completed")bad.snapshot.projection.turnStatus="completed";
  if(change==="epoch"||change==="cut"){
    f.reads[0].resolve(response());await tick();const page=response(false);
    if(change==="epoch")page.snapshot.projection.replay.runtimeEpoch="other";
    else page.snapshot.projection.replay.latestSeq=3;
    f.reads[1].resolve(page);
  }else f.reads[0].resolve(bad);
  await f.stream.settled;assert.equal(f.cuts.length,0,change);assert.equal(f.states[f.states.length-1],"reconcile",change);
}
{
  const f=fixture();await tick();f.receipt(identity);f.state("ready");await tick();
  for(let i=1;i<=257;i++)f.emit(i);
  assert.equal(f.states[f.states.length-1],"reconcile");f.reads[0].resolve(response());await f.stream.settled;assert.equal(f.cuts.length,0);
}
console.log("Remote snapshot synchronization: ready/receipt, fixed pages, overlap, disposal, scope and queue budgets passed");
{
  const f=fixture();await tick();f.receipt(identity);f.state("ready");await tick();
  f.reads[0].resolve(response());await tick();f.reads[1].resolve(response(false));await f.stream.settled;
  f.callbacks.get("bridge:remote-session-event")?.({protocolVersion:1,subscription:identity,frame:{protocolVersion:1,controller,sessionPath:input.sessionPath,event:{...event(1),turnId:"replacement-turn"}}});
  assert.equal(f.states[f.states.length-1],"reconcile","a changed turn cannot be discarded as an old sequence");
}
{
  const f=fixture();await tick();f.receipt(identity);f.state("ready");await tick();
  f.callbacks.get("bridge:remote-session-event")?.({protocolVersion:1,subscription:identity,frame:{protocolVersion:1,controller,sessionPath:input.sessionPath,event:{...event(1),text:"x".repeat(8*1024*1024)}}});
  assert.equal(f.states[f.states.length-1],"reconcile","per-frame budget applies before and after snapshot initialization");
  f.reads[0].resolve(response());await f.stream.settled;assert.equal(f.cuts.length,0);
}
