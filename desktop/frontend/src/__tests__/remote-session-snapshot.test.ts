import assert from "node:assert/strict";
import { openRemoteSessionSnapshot, type RemoteSnapshotTransport, type RemoteDisplayCut } from "../lib/remoteSessionSnapshot";
const id="AAAAAAAAAAAAAAAAAAAAAA", next="AQEBAQEBAQEBAQEBAQEBAQ";
const controller={id,name:"owned",workspace:"/resolved",readOnly:true};
const input={controllerId:id,sessionPath:"/owned.jsonl",surfaceId:"surface",generation:1};
const identity={...input,protocolVersion:1,subscriptionId:next,sidecarInstanceId:"owner"};
const event=(seq:number):Record<string,unknown>=>({kind:"text",seq,turnId:"turn",sessionPath:input.sessionPath,text:`text-${seq}`});
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
  const frame=(value:Record<string,unknown>)=>callbacks.get("bridge:remote-session-event")?.({protocolVersion:1,subscription:identity,frame:{protocolVersion:1,controller,sessionPath:input.sessionPath,event:{...event(value.seq as number),...value}}});
  return {receipt:(value:unknown)=>receipt(value),stream,calls,states,cuts,live,reads,state,emit,frame,callbacks};
}
function admittedResponse(turn="next-turn",messageId="next-question",after=2) {
  const result=response();const p=result.snapshot.projection;
  p.history.push({id:"previous-answer",role:"assistant",content:"previous answer"});
  p.userSuffix=[{id:messageId,role:"user",content:"next question"}];p.activeTurnId=turn;p.replayAfterSeq=after;
  p.replay={...p.replay,events:[
    {...event(after+1),turnId:turn,kind:"turn_started"},
    {...event(after+2),turnId:turn,kind:"user_message_admitted",messageId},
    {...event(after+3),turnId:turn,text:"next answer"},
  ],latestSeq:after+3,nextAfterSeq:after+3,hasMore:false};
  delete (result.snapshot as {nextPage?:string}).nextPage;return result;
}
async function readyFixture(){const f=fixture();await tick();f.receipt(identity);f.state("ready");await tick();return f;}
async function liveFixture(){const f=await readyFixture();f.reads[0].resolve(response());await tick();f.reads[1].resolve(response(false));await f.stream.settled;return f;}
function compactedResponse(){
  const result=response();const p=result.snapshot.projection;
  p.replay={...p.replay,events:[event(1),event(2),{...event(3),kind:"compaction_started"},{...event(4),kind:"compaction_done",compaction:{summary:"verified digest",archive:"private archive"}}],latestSeq:4,nextAfterSeq:4,hasMore:false};
  delete (result.snapshot as {nextPage?:string}).nextPage;return result;
}
function preAppendResponse(){
  const early=admittedResponse();const p=early.snapshot.projection;
  p.userSuffix=[];p.replay={...p.replay,events:[],latestSeq:3,nextAfterSeq:3};p.replayAfterSeq=3;
  return early;
}
{
  const f=await liveFixture();f.frame({kind:"compaction_started",seq:3});f.frame({kind:"compaction_done",seq:4});await tick();
  assert.equal(f.calls.length,3);assert.deepEqual(f.live,[3],"unverified completion is not applied to old view");
  f.emit(5);f.reads[2].resolve(compactedResponse());await tick();
  assert.equal(f.cuts.length,2);assert.deepEqual(f.live,[3,5]);assert.equal(f.states[f.states.length-1],"live");f.stream.dispose();
}
{
  const f=await readyFixture();f.reads[0].resolve(response());await tick();
  f.frame({kind:"compaction_started",seq:3});f.frame({kind:"compaction_done",seq:4});
  f.reads[1].resolve(response(false));await tick();
  assert.equal(f.calls.length,3,"buffered completion also obtains a verified cut");
  f.reads[2].resolve(compactedResponse());await f.stream.settled;assert.equal(f.cuts.length,2);f.stream.dispose();
}
{
  const f=await liveFixture();f.frame({kind:"compaction_started",seq:3});f.frame({kind:"compaction_done",seq:4});await tick();
  f.reads[2].reject(new Error("canonical base rewritten"));await tick();
  assert.equal(f.cuts.length,1);assert.equal(f.states[f.states.length-1],"reconcile");assert.equal(f.callbacks.size,0);
}
console.log("Remote compaction: same-subscription verified recapture, buffered barrier, resumed live and canonical rewrite refusal passed");
{
  const f=await liveFixture();
  f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});await tick();
  assert.equal(f.calls.length,2,"execution admission is not canonical question readiness");
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});await tick();
  assert.equal(f.calls.length,3);assert.deepEqual(f.calls[2],{id:next,next:undefined},"same ready subscription opens a new complete cut");
  f.frame({kind:"text",seq:6,turnId:"next-turn",text:"buffered next answer"});
  f.reads[2].resolve(admittedResponse());await tick();
  assert.equal(f.cuts.length,2);assert.equal(f.cuts[1].projection.activeTurnId,"next-turn");assert.deepEqual(f.live,[6]);
  assert.equal(f.states[f.states.length-1],"live");assert.equal(f.states.includes("closed"),false);
  f.frame({kind:"text",seq:7,turnId:"next-turn"});assert.deepEqual(f.live,[6,7]);f.stream.dispose();
}
{
  const f=await readyFixture();
  f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});await tick();
  assert.equal(f.calls.length,1,"replacement waits for the pending native read slot");
  f.reads[0].reject(new Error("superseded read"));await tick();
  assert.equal(f.calls.length,2,"superseded failure cannot retire the newer admitted turn");
  f.reads[1].resolve(admittedResponse());await f.stream.settled;
  assert.equal(f.cuts.length,1);assert.equal(f.cuts[0].projection.activeTurnId,"next-turn");f.stream.dispose();
}
{
  const f=await liveFixture();f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"foreign-question"});await tick();
  f.reads[2].resolve(admittedResponse());await tick();assert.equal(f.cuts.length,1);assert.equal(f.states[f.states.length-1],"reconcile","wrong canonical question cannot replace display");
}
console.log("Remote cross-turn snapshot: canonical admission gate, same subscription, serialized supersession, old failure fence and question identity passed");
{
  const f=await readyFixture();const early=preAppendResponse();
  // The subscription starts after turn_started. A body captured before the
  // canonical append arrives after its admission frame has already buffered.
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});
  f.frame({kind:"text",seq:6,turnId:"next-turn"});
  f.reads[0].resolve(early);await tick();
  assert.equal(f.cuts.length,0,"empty cut must never replace the conversation");
  assert.equal(f.calls.length,2,"already buffered canonical admission must start a fresh cut without another event");
  f.reads[1].resolve(admittedResponse());await f.stream.settled;
  assert.equal(f.cuts.length,1);assert.deepEqual(f.live,[6]);
  assert.equal(f.states[f.states.length-1],"live");f.stream.dispose();
}
for(const scenario of ["wrong question","dispose","missing admission"]){
  const f=await readyFixture();
  if(scenario==="missing admission")f.frame({kind:"text",seq:4,turnId:"next-turn"});
  else f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:scenario==="wrong question"?"foreign-question":"next-question"});
  f.reads[0].resolve(preAppendResponse());await tick();
  if(scenario==="missing admission")assert.equal(f.calls.length,1,"text cannot manufacture canonical readiness");
  else {
    assert.equal(f.calls.length,2);
    if(scenario==="dispose")f.stream.dispose();
    f.reads[1].resolve(admittedResponse());await f.stream.settled;
  }
  assert.equal(f.cuts.length,0,scenario);assert.equal(f.callbacks.size,0,scenario);
  if(scenario!=="dispose")assert.equal(f.states[f.states.length-1],"reconcile",scenario);
}
console.log("Remote pre-append body: buffered canonical barrier drained, no partial publication, wrong identity/text refusal and disposal passed");
{
  const f=await readyFixture();f.reads[0].resolve(response());await tick();
  assert.equal(f.calls.length,2);
  f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});
  f.reads[1].resolve(response(false));await tick();
  assert.equal(f.cuts.length,0,"old continuation is never published after replacement admission");
  assert.equal(f.calls.length,3);f.stream.dispose();f.reads[2].resolve(admittedResponse());await f.stream.settled;
  assert.equal(f.cuts.length,0,"dispose also fences replacement cut");assert.equal(f.callbacks.size,0);
}
{
  const f=await liveFixture();f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});await tick();
  const terminal=admittedResponse();const p=terminal.snapshot.projection;
  p.history.push(...p.userSuffix);p.userSuffix=[];p.activeTurnId="";p.turnStatus="completed";p.replayAfterSeq=6;
  p.replay={...p.replay,events:[],latestSeq:6,nextAfterSeq:6};
  f.reads[2].resolve(terminal);await tick();assert.equal(f.cuts.length,2);
  f.frame({kind:"turn_done",seq:6,turnId:"next-turn",status:"completed"});
  assert.equal(f.states[f.states.length-1],"live","late overlap of fast-settled turn is not a new foreign turn");f.stream.dispose();
}
{
  const f=await liveFixture();f.frame({kind:"turn_started",seq:1,turnId:"stale-turn"});
  assert.equal(f.states[f.states.length-1],"reconcile","old sequence cannot trigger replacement cut");assert.equal(f.calls.length,2);
}
{
  const f=await liveFixture();f.frame({kind:"turn_started",seq:3,turnId:"next-turn"});
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});await tick();
  const replaced=admittedResponse();replaced.snapshot.projection.replay.runtimeEpoch="replacement-runtime";
  f.reads[2].resolve(replaced);await tick();assert.equal(f.cuts.length,1);assert.equal(f.states[f.states.length-1],"reconcile","runtime replacement is not a same-owner cross-turn refresh");
}
{
  const f=await readyFixture();const early=admittedResponse();
  early.snapshot.projection.userSuffix=[];
  early.snapshot.projection.replay={...early.snapshot.projection.replay,events:[],latestSeq:2,nextAfterSeq:2};
  early.snapshot.projection.replayAfterSeq=2;
  f.reads[0].resolve(early);await f.stream.settled;
  assert.equal(f.cuts.length,0);assert.equal(f.calls.length,1,"empty canonical admission cut is not polled");
  f.frame({kind:"user_message_admitted",seq:4,turnId:"next-turn",messageId:"next-question"});await tick();
  assert.equal(f.calls.length,2);f.reads[1].resolve(admittedResponse());await tick();
  assert.equal(f.cuts.length,1);assert.equal(f.states[f.states.length-1],"live");f.stream.dispose();
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
