import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom=new JSDOM('<html><body><div id="root"></div></body></html>',{url:'http://localhost',pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,localStorage:dom.window.localStorage,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const React=await import("react"),{act}=React,{createRoot}=await import("react-dom/client");
const {useTauriSelectionSpeech}=await import("../tauri/useTauriSelectionSpeech");const {saveVoicePreferences}=await import("../tauri/voicePreferences");
const calls:string[]=[],requests:{id:string;provider:string;text:string;voice:string;resolve:(audio:string)=>void}[]=[];const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
let noProviders=false;const api={providers:async()=>noProviders?[]:[{name:"first",label:"First",preferred:true},{name:"saved",label:"Saved"}],synthesize:(id:string,provider:string,text:string,voice:string)=>new Promise<string>(resolve=>requests.push({id,provider,text,voice,resolve})),cancel:async(id:string)=>{calls.push("cancel:"+id);},playback:()=>{calls.push("unlock");return{play:async(audio:string)=>{calls.push(audio);},stop:()=>{},close:()=>{calls.push("close");}};}};
let control!:ReturnType<typeof useTauriSelectionSpeech>;
function Harness({owner="a",blocked=false,voiceCallActive=false}:{owner?:string;blocked?:boolean;voiceCallActive?:boolean}){control=useTauriSelectionSpeech({owner,blocked,voiceCallActive,onStart:()=>{calls.push("pause-conversation");},api});return <div>{control.status}{control.error}</div>;}
const root=createRoot(document.getElementById("root")!);const render=async(owner="a",blocked=false,voiceCallActive=false)=>{await act(async()=>{root.render(<Harness owner={owner} blocked={blocked} voiceCallActive={voiceCallActive}/>);await tick();});};
const start=async(text="选中的回答。")=>{await act(async()=>{control.speak(async()=>text);await tick();});};
const resolve=async(index:number,audio:string)=>{await act(async()=>{requests[index].resolve(audio);await tick();});};
try{
 saveVoicePreferences({speaker:"saved",voice:"苏打"});await render();let textReady!:(value:string|null)=>void;
 await act(async()=>{control.speak(()=>{assert.equal(calls.at(-2),"unlock","audio unlocked before resolving logical text");return new Promise(resolve=>{textReady=resolve;});});await tick();});assert.equal(requests.length,0);
 await act(async()=>{textReady("**选中文字**，而非整轮回答。");await tick();});assert.equal(requests[0].provider,"saved");assert.equal(requests[0].voice,"苏打");assert.equal(requests[0].text,"选中文字，而非整轮回答。");await resolve(0,"PLAY");assert(calls.includes("PLAY"));
 await start();await act(async()=>{control.stop();await tick();});assert(calls.includes("cancel:"+requests[1].id));await resolve(1,"STOPPED");assert(!calls.includes("STOPPED"));
 await start();await render("b");await resolve(2,"OLD SESSION");assert(!calls.includes("OLD SESSION"));
 await start();await act(async()=>{Object.defineProperty(document,"hidden",{configurable:true,value:true});document.dispatchEvent(new dom.window.Event("visibilitychange"));await tick();});await resolve(3,"HIDDEN");assert(!calls.includes("HIDDEN"));Object.defineProperty(document,"hidden",{configurable:true,value:false});
 await start();await render("b",false,true);await resolve(4,"MIC OWNS AUDIO");assert(!calls.includes("MIC OWNS AUDIO"));await render("b");
 await start("旧选择");await start("新选择");await resolve(5,"OLD SELECTION");assert(!calls.includes("OLD SELECTION"));await resolve(6,"LATEST");assert(calls.includes("LATEST"));
 await start();await render("b",true);await resolve(7,"BLOCKED");assert(!calls.includes("BLOCKED"));await start();assert.equal(requests.length,8);await render("b");
 noProviders=true;await start();assert(document.body.textContent?.includes("没有可用的播报渠道"));noProviders=false;
 await act(async()=>{control.speak(async()=>null);await tick();});assert.equal(requests.length,8,"stale logical selection cannot spend TTS quota");
 console.log("PASS selection speech: gesture unlock before async selection, saved provider/voice, exact selected text, pause conversation, stop/session/visibility/microphone/blocked fences, latest selection only, unavailable channel and stale-selection feedback");
}finally{await act(async()=>root.unmount());dom.window.close();}
