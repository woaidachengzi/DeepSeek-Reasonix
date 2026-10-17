import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom=new JSDOM("<html><body><div id='root'></div><textarea></textarea><input><div contenteditable='true'>编辑内容</div><button id='other'>其他</button></body></html>",{url:"http://localhost",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,localStorage:dom.window.localStorage,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const React=await import("react"),{act}=React,{createRoot}=await import("react-dom/client");
const {TauriVoiceConversation}=await import("../tauri/TauriVoiceConversation");
const {readVoicePreferences}=await import("../tauri/voicePreferences");
let captures=0,sends=0,cancels=0,ends=0;const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
const api={providers:async()=>[{name:"mimo",label:"MiMo",preferred:true}],transcribe:async()=>"语音问题",synthesize:async()=>"WAV",cancel:async()=>{},capture:async()=>{captures++;return{stop:async()=>"WAV",cancel:()=>{cancels++;}};},playback:()=>({play:async()=>{},stop:()=>{},close:()=>{}})};
const root=createRoot(document.getElementById("root")!);const sink={current:null};
const render=async(open=true,hardBlocked=false)=>{await act(async()=>{root.render(<TauriVoiceConversation owner="session" sequence={0} open={open} hardBlocked={hardBlocked} canSend paused={false} phaseLabel="等待回答" eventSink={sink} onOpen={()=>{}} onEnd={()=>{ends++;}} onActiveChange={()=>{}} onSend={async()=>{sends++;return true;}} onOpenSettings={()=>{}} api={api}/>);await tick();});};
const key=async(type:string,target:Element|Document=document,options:KeyboardEventInit={})=>{const event=new dom.window.KeyboardEvent(type,{key:" ",bubbles:true,cancelable:true,...options});await act(async()=>{target.dispatchEvent(event);await tick();});return event;};
const click=async(text:string)=>{await act(async()=>{[...document.querySelectorAll<HTMLButtonElement>("button")].find(b=>b.textContent===text)!.click();await tick();});};
try{
 await render();assert(document.querySelector<HTMLDivElement>('.tauri-voice-call-settings')?.hidden,"settings are collapsed by default");
 assert((await key("keydown")).defaultPrevented);assert.equal(captures,1);await key("keydown",document,{repeat:true});assert.equal(captures,1);
 await key("keyup",document.querySelector("textarea")!);assert.equal(sends,1,"release still submits after focus moves");await key("keyup");assert.equal(sends,1);
 await click("停止播报");
 for(const selector of ['textarea','input','[contenteditable]','#other']){const before=captures;assert(!(await key("keydown",document.querySelector(selector)!)).defaultPrevented);await key("keyup");assert.equal(captures,before,selector);}
 for(const options of [{ctrlKey:true},{metaKey:true},{altKey:true},{shiftKey:true},{isComposing:true},{repeat:true}]){await key("keydown",document,options);assert.equal(captures,1);}
 await key("keydown",document.querySelector('[aria-label="开始语音对话"]')!);assert.equal(captures,2,"space works immediately after the headphone control was clicked");
 await act(async()=>{window.dispatchEvent(new dom.window.Event("blur"));await tick();});await key("keyup");assert.equal(sends,1);assert(cancels>0,"window blur cancels without sending");
 await click("设置");assert(!document.querySelector<HTMLDivElement>('.tauri-voice-call-settings')?.hidden);
 const option=[...document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find(i=>i.parentElement?.textContent?.includes("空格键"))!;
 await act(async()=>{option.click();await tick();});await key("keydown");assert.equal(captures,2,"space shortcut can be disabled");assert.equal(readVoicePreferences().spaceEnabled,false,"shortcut preference survives a new conversation");await key("keyup");
 await click("收起设置");await render(false);await key("keydown");assert.equal(captures,2,"closed voice mode never captures space");
 await render(true,true);await key("keydown");assert.equal(captures,2,"blocked voice mode cannot capture");assert.equal(ends,0);
 console.log("PASS voice keyboard: compact defaults, global space/release, no repeats or duplicate sends, headphone focus, editable/control/modifier/IME exclusions, blur cancellation, configurable shortcut, closed/blocked guards");
}finally{await act(async()=>root.unmount());dom.window.close();}
