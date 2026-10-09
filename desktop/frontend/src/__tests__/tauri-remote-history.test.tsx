import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerSessionView } from "../lib/bridgeProtocol.generated";

const dom=new JSDOM("<body><div id='root'></div></body>",{url:"http://localhost/",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,Event:dom.window.Event,localStorage:dom.window.localStorage,IS_REACT_ACT_ENVIRONMENT:true});
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
assert.equal(document.getElementById("root")?.children.length,0);dom.window.close();
console.log("Remote history UI ownership: old failure/finally, refresh guard, fixed errors, empty state and unmount passed");
console.log("Remote live UI ownership: equal controller identity, reconcile unlock, retry before old read settlement, stale finally and subscription cleanup passed");
