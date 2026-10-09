import assert from "node:assert/strict";
import { createRemotePromptDecision, type RemotePromptScope } from "../lib/remotePromptDecision";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerSessionView,BridgeRemoteControllerSessionPromptRequest,BridgeRemoteControllerSessionPromptReceipt } from "../lib/bridgeProtocol.generated";

const deferred=<T>()=>{let resolve!:(value:T)=>void,reject!:(error:unknown)=>void;const promise=new Promise<T>((ok,no)=>{resolve=ok;reject=no;});return {promise,resolve,reject};};
const tick=async()=>{for(let i=0;i<8;i++)await Promise.resolve();};
const scope:RemotePromptScope={sessionPath:"/owned.jsonl",runtimeEpoch:"controller-instance",turnId:"owned-turn",promptId:"owned-prompt",promptRuntimeEpoch:"prompt-routing",kind:"ask"};
const view=():BridgeRemoteControllerSessionView=>({protocolVersion:1,sessionPath:scope.sessionPath,readOnly:true,ownership:"serve",current:false,modelRef:"",label:"",history:[],runtimeState:{schemaVersion:1,runtimeEpoch:scope.runtimeEpoch,revision:8,phase:"executing",running:true,cancellable:true,cancelRequested:false,pendingPrompt:true,backgroundJobs:0,activity:"waiting",turnId:scope.turnId,turnStatus:"waiting_user",turnEventSeq:12}});
function fixture(selected:RemotePromptScope=scope){
  const read=deferred<BridgeRemoteControllerSessionView>(),receipt=deferred<BridgeRemoteControllerSessionPromptReceipt>();
  const calls:BridgeRemoteControllerSessionPromptRequest[]=[],reads:string[]=[],outcomes:string[]=[];
  let current=true;
  const lease:RemoteControllerLease={ready:Promise.resolve({id:"owned",name:"owned",workspace:"/owned",readOnly:true}),sessions:async()=>[],sessionImage:async()=>({url:"",errorCode:"not-found"}),release:()=>{},
    sessionView:path=>{reads.push(path);return read.promise;},sessionPrompt:input=>{calls.push(input);return receipt.promise;}};
  const command=createRemotePromptDecision(lease,selected,()=>current,outcome=>outcomes.push(outcome));
  return {command,lease,read,receipt,calls,reads,outcomes,setCurrent:(value:boolean)=>{current=value;}};
}
for(const [kind,answer] of [
  ["ask",{questions:[{questionId:"q",selected:["One"]}]}],
  ["approval",{allow:false,session:false,persist:false}],
  ["plan",{action:"revise_plan",feedback:"adjust"}],
  ["recovery",{action:"continue_task"}],
  ["mcp",{action:"accept",content:{name:"value"}}],
] as const){
  const selected={...scope,kind},f=fixture(selected),pending=f.command.resolve(answer);
  assert.equal(await f.command.resolve(answer),"ignored","duplicate clicks never join or dispatch another decision");
  assert.deepEqual(f.reads,[scope.sessionPath]);assert.deepEqual(f.outcomes,["pending"]);
  f.read.resolve(view());await tick();assert.deepEqual(f.calls,[{...selected,answer}]);
  f.receipt.resolve({protocolVersion:1,...selected,resolved:true});assert.equal(await pending,"sent");
  assert.deepEqual(f.outcomes,["pending","sent"]);assert.equal(await f.command.resolve(answer),"ignored");
  assert.equal(f.calls.length,1,"confirmed receipt cannot automatically repeat the decision");
}
{
  const mutableScope={...scope},f=fixture(mutableScope),answer={questions:[{questionId:"q",selected:["original"]}]};
  const pending=f.command.resolve(answer);mutableScope.promptId="mutated";answer.questions[0].selected[0]="mutated";
  f.read.resolve(view());await tick();assert.equal(f.calls[0].promptId,scope.promptId);
  assert.deepEqual(f.calls[0].answer,{questions:[{questionId:"q",selected:["original"]}]});
  f.receipt.reject(new Error("PRIVATE key/endpoint"));assert.equal(await pending,"unknown");
  assert.deepEqual(f.outcomes,["pending","unknown"]);assert.equal(await f.command.resolve(answer),"ignored");assert.equal(f.calls.length,1);
}
for(const change of [
  (v:BridgeRemoteControllerSessionView)=>({...v,sessionPath:"/other.jsonl"}),
  (v:BridgeRemoteControllerSessionView)=>({...v,ownership:"saved" as const}),
  (v:BridgeRemoteControllerSessionView)=>({...v,runtimeState:{...v.runtimeState!,runtimeEpoch:"replacement"}}),
  (v:BridgeRemoteControllerSessionView)=>({...v,runtimeState:{...v.runtimeState!,turnId:"new-turn"}}),
  (v:BridgeRemoteControllerSessionView)=>({...v,runtimeState:{...v.runtimeState!,pendingPrompt:false}}),
  (v:BridgeRemoteControllerSessionView)=>({...v,runtimeState:{...v.runtimeState!,phase:"finishing" as const}}),
  (v:BridgeRemoteControllerSessionView)=>({...v,runtimeState:{...v.runtimeState!,cancelRequested:true}}),
]){
  const f=fixture(),pending=f.command.resolve({questions:[]});f.read.resolve(change(view()));
  assert.equal(await pending,"changed");assert.equal(f.calls.length,0);assert.deepEqual(f.outcomes,["pending","changed"]);
}
for(const dispose of [false,true]){
  const f=fixture(),pending=f.command.resolve({questions:[]});
  if(dispose)f.command.dispose();else f.setCurrent(false);
  f.read.resolve(view());assert.equal(await pending,"discarded");assert.equal(f.calls.length,0);assert.deepEqual(f.outcomes,["pending"]);
}
for(const fail of [false,true]){
  const f=fixture(),pending=f.command.resolve({questions:[]});f.read.resolve(view());await tick();assert.equal(f.calls.length,1);
  f.setCurrent(false);
  if(fail)f.receipt.reject(new Error("PRIVATE old failure"));else f.receipt.resolve({protocolVersion:1,...scope,resolved:true});
  assert.equal(await pending,"discarded");assert.deepEqual(f.outcomes,["pending"],"old settlement cannot alter the replacement card");
}
{
  const f=fixture(),pending=f.command.resolve({questions:[]});f.read.resolve(view());await tick();
  f.receipt.resolve({protocolVersion:1,...scope,promptRuntimeEpoch:"controller-instance",resolved:true});
  assert.equal(await pending,"unknown","instance and prompt routing epochs cannot alias");
  assert.equal(f.calls.length,1);assert.equal(await f.command.resolve({questions:[]}),"ignored");
}
{
  const f=fixture();delete f.lease.sessionPrompt;
  assert.equal(await f.command.resolve({questions:[]}),"changed");assert.equal(f.calls.length,0);assert.equal(f.reads.length,0);
}
{
  const f=fixture(),pending=f.command.resolve({questions:[]});f.read.reject(new Error("PRIVATE read failure"));
  assert.equal(await pending,"changed");assert.equal(f.calls.length,0);assert.deepEqual(f.outcomes,["pending","changed"]);
}
{
  const f=fixture(),pending=f.command.resolve({questions:[]});f.read.resolve(view());await tick();
  f.command.dispose();f.receipt.resolve({protocolVersion:1,...scope,resolved:true});
  assert.equal(await pending,"discarded");assert.deepEqual(f.outcomes,["pending"]);
  assert.equal(await f.command.resolve({questions:[]}),"discarded");assert.equal(f.calls.length,1);
}
{
  const f=fixture();assert.equal(await f.command.resolve({unsupported:()=>{}}),"changed");
  assert.equal(f.reads.length,0);assert.equal(f.calls.length,0,"uncloneable drafts never dispatch");
}
console.log("Remote prompt decision: five answers, captured immutable scope, fresh liveness, duplicate/unknown no retry and replaced owner zero publication passed");
