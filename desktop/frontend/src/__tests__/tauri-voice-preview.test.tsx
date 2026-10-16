import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom=new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>",{url:"http://localhost/",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,localStorage:dom.window.localStorage,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const React=await import("react");const {act}=React;const {createRoot}=await import("react-dom/client");
const {TauriVoiceConversation}=await import("../tauri/TauriVoiceConversation");
const root=createRoot(document.getElementById("root")!);
const tick=async()=>{await new Promise(resolve=>setTimeout(resolve,0));};
const requests:{id:string;provider:string;text:string;voice:string;resolve:(audio:string)=>void;reject:(error:Error)=>void}[]=[];
const cancelled:string[]=[],played:string[]=[];let closed=0,captures=0,sends=0;
const api={providers:async()=>[{name:"mimo",label:"MiMo",preferred:true},{name:"second",label:"Second MiMo"}],
 synthesize:(id:string,provider:string,text:string,voice:string)=>new Promise<string>((resolve,reject)=>requests.push({id,provider,text,voice,resolve,reject})),
 transcribe:async()=>"识别文字",cancel:async(id:string)=>{cancelled.push(id);},
 capture:async()=>{captures++;return {stop:async()=>"WAV",cancel:()=>{}};},
 playback:()=>({play:(audio:string,signal:AbortSignal)=>{played.push(audio);return new Promise<void>((_resolve,reject)=>signal.addEventListener("abort",()=>reject(new Error("cancelled")),{once:true}));},stop:()=>{},close:()=>{closed++;}}),
};
const sink={current:null};const noop=()=>{};
const render=async(owner="session",open=true,hardBlocked=false)=>{await act(async()=>{root.render(<TauriVoiceConversation owner={owner} sequence={0} open={open} canSend hardBlocked={hardBlocked} paused={false} phaseLabel="等待" eventSink={sink} onOpen={noop} onEnd={noop} onActiveChange={noop} onSend={async()=>{sends++;return true;}} onOpenSettings={noop} api={api}/>);await tick();});};
const choose=async(label:string,value:string)=>{await act(async()=>{const select=document.querySelector<HTMLSelectElement>(`[aria-label="${label}"]`)!;select.value=value;select.dispatchEvent(new dom.window.Event("change",{bubbles:true}));await tick();});};
const click=async(text:string)=>{await act(async()=>{const button=[...document.querySelectorAll<HTMLButtonElement>("button")].find(b=>b.textContent===text);assert(button,text);button.click();await tick();});};
const resolve=async(index:number,audio:string)=>{await act(async()=>{requests[index].resolve(audio);await tick();});};
try{
 await render();assert.equal(requests.length,0,"opening the panel never auto-spends speech quota");
 await choose("语音对话音色","苏打");assert.equal(requests.length,1);assert.equal(requests[0].voice,"苏打");assert.equal(requests[0].provider,"mimo");assert(requests[0].text.includes("这是我的声音"));
 assert(document.body.textContent?.includes("正在准备音色试听"));
 await choose("语音对话音色","Mia");assert(cancelled.includes(requests[0].id));
 await resolve(0,"OLD");assert.deepEqual(played,[],"late synthesis from previous voice cannot play");
 await resolve(1,"MIA");assert.deepEqual(played,["MIA"]);assert(document.body.textContent?.includes("正在试听：Mia"));
 await click("停止试听");assert(cancelled.includes(requests[1].id));assert(!document.body.textContent?.includes("正在试听：Mia"));
 await click("试听");assert.equal(requests[2].voice,"Mia");
 await choose("对话语音播报渠道","second");assert(cancelled.includes(requests[2].id));await resolve(2,"WRONG CHANNEL");assert.deepEqual(played,["MIA"]);
 await click("试听");assert.equal(requests[3].provider,"second");await render("another");await resolve(3,"WRONG SESSION");assert.deepEqual(played,["MIA"]);
 await choose("语音对话交流方式","continuous");await click("试听");await resolve(4,"CONTINUOUS PREVIEW");assert(played.includes("CONTINUOUS PREVIEW"));await click("停止试听");
 await click("试听");await act(async()=>{requests[5].reject(new Error("播报渠道未授权；请检查 API Key 及 MiMo TTS 权限。"));await tick();});assert(document.querySelector('[role="alert"]')?.textContent?.includes("播报渠道未授权"));
 await click("试听");await render("another",true,true);await resolve(6,"BLOCKED");assert(!played.includes("BLOCKED"));await render("another");
 await choose("语音对话交流方式","hold");await click("试听");await resolve(7,"BEFORE MIC");
 await act(async()=>{document.querySelector('[aria-label="按住说话，松开发送"]')!.dispatchEvent(new dom.window.KeyboardEvent("keydown",{key:" ",bubbles:true}));await tick();});assert(cancelled.includes(requests[7].id));assert.equal(captures,1);assert(document.querySelector<HTMLSelectElement>('[aria-label="语音对话音色"]')?.disabled);
 await click("结束对话");await render("another",false);assert.equal(sends,0,"preview never submits a model message");assert(closed>=8);
 await render("last");await click("试听");await render("last",false);await resolve(8,"CLOSED");assert(!played.includes("CLOSED"));
 console.log("PASS voice preview: explicit selection/replay, latest voice only, stop playback, provider/session/close/blocked fences, both modes, errors, microphone ownership and zero model sends");
}finally{await act(async()=>root.unmount());dom.window.close();}
