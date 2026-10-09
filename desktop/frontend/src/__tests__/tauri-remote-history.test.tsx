import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerSessionView } from "../lib/bridgeProtocol.generated";
import {TranscriptTestClock} from "./transcript-test-clock";

const dom=new JSDOM("<!doctype html><body><div id='root'></div></body>",{url:"http://localhost/",pretendToBeVisual:true});
const clock=new TranscriptTestClock();
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,Event:dom.window.Event,localStorage:dom.window.localStorage,requestAnimationFrame:clock.requestAnimationFrame,cancelAnimationFrame:clock.cancelAnimationFrame,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const React=await import("react");const {act}=React;const {createRoot}=await import("react-dom/client");
const {LocaleProvider}=await import("../lib/i18n");const {TauriRemoteHistory}=await import("../tauri/TauriRemoteHistory");
const root=createRoot(document.getElementById("root")!);
const tick=async()=>{for(let i=0;i<12;i++)await Promise.resolve();};
const controller={id:"owned-controller",name:"owned",workspace:"/resolved",readOnly:true};
const gates:{path:string;resolve:(view:BridgeRemoteControllerSessionView)=>void;reject:(error:unknown)=>void}[]=[];
const lease:RemoteControllerLease={sessionImage:async()=>({url:"",errorCode:"not-found"}),ready:Promise.resolve(controller),sessions:async()=>[],release:()=>{},sessionView:path=>new Promise((resolve,reject)=>gates.push({path,resolve,reject}))};
const snapshot=(path:string):BridgeRemoteControllerSessionView=>({protocolVersion:1,sessionPath:path,readOnly:true,ownership:"saved",current:false,modelRef:"",label:"",history:[]});
const close=()=>{};
const render=async(path:string)=>act(async()=>{root.render(<LocaleProvider><TauriRemoteHistory lease={lease} controller={controller} sessionPath={path} title={path} onClose={close}/></LocaleProvider>);await tick();});
await render("/old.jsonl");assert.equal(gates.length,1);
await render("/new.jsonl");assert.equal(gates.length,2);
await act(async()=>{gates[1].resolve(snapshot("/new.jsonl"));await tick();});
assert.ok(document.body.textContent?.includes("no displayable messages"));
await act(async()=>{gates[0].reject(new Error("PRIVATE OLD FAILURE"));await tick();});
assert.equal(document.querySelector('[role="alert"]'),null,"old failure/finally cannot override new result");
assert.ok(!document.querySelector('[aria-busy="true"]'));
const refresh=()=>[...document.querySelectorAll<HTMLButtonElement>("button")].find(button=>button.textContent==="Refresh history")!;
await act(async()=>{refresh().click();await tick();});assert.equal(gates.length,3);assert.ok(refresh().disabled);
await act(async()=>{gates[2].reject(new Error("PRIVATE CURRENT FAILURE"));await tick();});
assert.match(document.querySelector('[role="alert"]')?.textContent ?? "",/Upgrade remote Serve/);
assert.ok(!document.body.textContent?.includes("PRIVATE"));
await act(async()=>{refresh().click();await tick();});assert.equal(gates.length,4);
await act(async()=>root.unmount());
await act(async()=>{gates[3].resolve(snapshot("/new.jsonl"));await tick();});
assert.equal(document.getElementById("root")?.children.length,0);

// Reconcile releases the UI lock even when native cancellation is still
// settling. Equal controller props do not retire the current subscription.
const liveRoot=createRoot(document.getElementById("root")!);
const ownedController={...controller,id:"AAAAAAAAAAAAAAAAAAAAAQ"};
const listeners=new Map<string,(payload:unknown)=>void>();
const subscriptions:Record<string,unknown>[]=[];const closed:string[]=[];
const reads:{resolve:(value:unknown)=>void}[]=[];
const transport={
  listen:async(name:string,callback:(payload:unknown)=>void)=>{listeners.set(name,callback);return()=>{if(listeners.get(name)===callback)listeners.delete(name);};},
  subscribe:async(input:Record<string,unknown>)=>{
    const identity={...input,protocolVersion:1,subscriptionId:subscriptions.length?"BBBBBBBBBBBBBBBBBBBBBQ":"CCCCCCCCCCCCCCCCCCCCCA",sidecarInstanceId:"fixture"};
    subscriptions.push(identity);
    listeners.get("bridge:remote-session-state")?.({protocolVersion:1,subscription:identity,state:"ready"});return identity;
  },
  unsubscribe:async(id:string)=>{closed.push(id);},
  snapshot:async()=>new Promise<unknown>(resolve=>reads.push({resolve})),
};
const ownedLease:RemoteControllerLease={...lease,sessionView:async path=>({...snapshot(path),ownership:"serve"})};
const renderOwned=async()=>act(async()=>{liveRoot.render(<LocaleProvider><TauriRemoteHistory lease={ownedLease} controller={{...ownedController}} transport={transport} sessionPath="/owned.jsonl" title="Owned" onClose={close}/></LocaleProvider>);await tick();});
await renderOwned();assert.equal(reads.length,1);
await renderOwned();assert.equal(subscriptions.length,1,"equal controller clone must not replace subscription");
await act(async()=>{listeners.get("bridge:remote-session-state")?.({protocolVersion:1,subscription:subscriptions[0],state:"ended"});await tick();});
assert.ok(document.querySelector('[role="alert"]'));assert.equal(refresh().disabled,false);
await act(async()=>{refresh().click();await tick();});
assert.equal(reads.length,2,"retry starts while cancelled old read is unresolved");
assert.ok(refresh().disabled);
await act(async()=>{reads[0].resolve({private:"late invalid old cut"});await tick();});
assert.ok(refresh().disabled,"old finally cannot unlock newer pending read");
assert.equal(document.querySelector('[role="alert"]'),null);
await act(async()=>liveRoot.unmount());
assert.equal(listeners.size,0);assert.equal(closed.length,2);
await act(async()=>{reads[1].resolve({});await tick();});
assert.equal(document.getElementById("root")?.children.length,0);

// Stop captures the displayed cut's turn, never a later sampled foreground.
const stopRoot=createRoot(document.getElementById("root")!);
const stops:{scope:{sessionPath:string;runtimeEpoch:string;turnId:string};resolve:(value:{protocolVersion:number;sessionPath:string;runtimeEpoch:string;turnId:string;cancelled:boolean})=>void;reject:(error:unknown)=>void}[]=[];
const stopLease:RemoteControllerLease={...ownedLease,
  sessionView:async path=>({...snapshot(path),ownership:"serve",runtimeState:{schemaVersion:1,runtimeEpoch:"controller-instance",revision:1,phase:"executing",running:true,cancelRequested:false,cancellable:true,pendingPrompt:false,backgroundJobs:0,activity:"streaming",turnId:"owned-turn",turnStatus:"in_progress",turnEventSeq:3}}),
  sessionCancel:scope=>new Promise((resolve,reject)=>stops.push({scope,resolve,reject})),
};
const renderStop=async(path:string)=>act(async()=>{stopRoot.render(<LocaleProvider><TauriRemoteHistory lease={stopLease} controller={ownedController} transport={transport} sessionPath={path} title={path} onClose={close}/></LocaleProvider>);await tick();});
const answerStopCut=async(path:string)=>act(async()=>{
  const identity=subscriptions.at(-1)!;
  reads.at(-1)!.resolve({protocolVersion:1,subscription:identity,snapshot:{protocolVersion:1,controller:ownedController,projection:{protocolVersion:1,sessionPath:path,readOnly:true,initial:true,history:[],userSuffix:[{id:"question",role:"user",content:"Active question"}],activeTurnId:"owned-turn",turnStatus:"in_progress",replayAfterSeq:0,replay:{events:[{kind:"turn_started",seq:1,turnId:"owned-turn",sessionPath:path,status:"in_progress"},{kind:"user_message_admitted",seq:2,turnId:"owned-turn",sessionPath:path,messageId:"question",status:"in_progress"},{kind:"text",seq:3,turnId:"owned-turn",sessionPath:path,text:"Partial answer",status:"in_progress"}],floorSeq:1,latestSeq:3,nextAfterSeq:3,hasMore:false,runtimeEpoch:"routing-epoch"}}}});
  await tick();
});
const stopButton=()=>[...document.querySelectorAll<HTMLButtonElement>("button")].find(button=>button.textContent==="Stop")!;
await renderStop("/stop-old.jsonl");await answerStopCut("/stop-old.jsonl");
assert.ok(!stopButton().hidden&&!stopButton().disabled);
await act(async()=>{stopButton().click();stopButton().click();await tick();});
assert.equal(stops.length,1,"double click dispatches one Stop");assert.ok(stopButton().disabled&&refresh().disabled);
assert.deepEqual(stops[0].scope,{sessionPath:"/stop-old.jsonl",runtimeEpoch:"controller-instance",turnId:"owned-turn"},"Controller epoch is not event routing epoch");
await act(async()=>{stops[0].reject(new Error("PRIVATE endpoint/key"));await tick();});
assert.match(document.querySelector('[role="alert"]')?.textContent??"",/outcome is unknown/);
assert.ok(stopButton().disabled&&!refresh().disabled);assert.ok(!document.body.textContent?.includes("PRIVATE"));
await act(async()=>{refresh().click();await tick();});await answerStopCut("/stop-old.jsonl");
await act(async()=>{stopButton().click();await tick();});assert.equal(stops.length,2);
await renderStop("/stop-new.jsonl");await answerStopCut("/stop-new.jsonl");
await act(async()=>{stops[1].reject(new Error("PRIVATE stale stop"));await tick();});
assert.equal(document.querySelector('[role="alert"]'),null);assert.ok(!stopButton().disabled,"old finally does not disable or unlock new operation");
await act(async()=>{stopButton().click();await tick();});assert.equal(stops.length,3);
await act(async()=>{stops[2].resolve({protocolVersion:1,...stops[2].scope,cancelled:true});await tick();});
assert.ok(document.body.textContent?.includes("Waiting for the remote turn to finish"));assert.ok(stopButton().disabled);
await act(async()=>{listeners.get("bridge:remote-session-state")?.({protocolVersion:1,subscription:subscriptions.at(-1),state:"ended"});await tick();});
assert.ok(stopButton().hidden,"reconcile cannot leave a stale Stop action enabled");
await act(async()=>stopRoot.unmount());
assert.equal(listeners.size,0);dom.window.close();
assert.equal(clock.frames.size,0);
console.log("Remote Stop UI: exact displayed turn/controller epoch, duplicate guard, fixed unknown state, manual refresh, stale failure and waiting for terminal passed");
console.log("Remote history UI ownership: old failure/finally, refresh guard, fixed errors, empty state and unmount passed");
console.log("Remote live UI ownership: equal controller identity, reconcile unlock, retry before old read settlement, stale finally and subscription cleanup passed");
