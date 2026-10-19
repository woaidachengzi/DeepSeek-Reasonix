import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import { VoiceReplyTracker, spokenReply } from "../tauri/voiceConversation";
import type { TauriBridgeEvent } from "../lib/tauriBridge";
let sequence=0;
const event=(kind:string,payload:Record<string,unknown>={},sessionId="session"):TauriBridgeEvent=>({protocolVersion:1,sessionId,sequence:++sequence,eventKind:kind,payload:{kind,...payload}});
const tracker=new VoiceReplyTracker("session",0);
assert.equal(tracker.accept(event("text",{text:"OLD"})),null);
tracker.accept(event("turn_started",{turnId:"own"}));
tracker.accept(event("reasoning",{text:"PRIVATE"}));
tracker.accept(event("text",{text:"TOOL PREFACE",turnId:"own"}));tracker.accept(event("tool_dispatch"));
tracker.accept(event("text",{text:"partial"}));tracker.accept(event("stream_attempt",{streamAttempt:{id:"retry",action:"discard"}}));
tracker.accept(event("text",{text:"FOREIGN",turnId:"other"}));tracker.accept(event("text",{text:"最终回答",turnId:"own"}));
const done=event("turn_done",{status:"completed",turnId:"own"});assert.deepEqual(tracker.accept(done),{text:"最终回答"});assert.equal(tracker.accept(done),null);
// The real engine commits each successful sampling attempt after its text.
const committed=new VoiceReplyTracker("session",0);
committed.accept(event("turn_started"));
committed.accept(event("stream_attempt",{streamAttempt:{id:"first",action:"begin"}}));
committed.accept(event("text",{text:"丢弃的旧片段"}));
committed.accept(event("stream_attempt",{streamAttempt:{id:"first",action:"discard"}}));
committed.accept(event("stream_attempt",{streamAttempt:{id:"second",action:"begin"}}));
committed.accept(event("text",{text:"应当朗读的回答。"}));
committed.accept(event("stream_attempt",{streamAttempt:{id:"second",action:"commit"}}));
assert.deepEqual(committed.accept(event("turn_done",{status:"completed"})),{text:"应当朗读的回答。"},"successful sampling commit must retain the spoken reply");
for(const payload of [{status:"failed"},{status:"interrupted"},{outcome:"cancelled"}]){const cancelled=new VoiceReplyTracker("session",0);cancelled.accept(event("turn_started"));cancelled.accept(event("text",{text:"partial"}));assert(cancelled.accept(event("turn_done",payload))?.error);}
assert.deepEqual(spokenReply("**你好** [链接](https://example.com)\n```sh\nprivate command\n```"),{chunks:["你好 链接"],truncated:false});assert.equal(spokenReply("字".repeat(5000)).chunks.join("").length,4000);assert(spokenReply("字".repeat(5000)).truncated);
const dom=new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>",{url:"http://localhost/",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,localStorage:dom.window.localStorage,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const React=await import("react");const {act}=React;const {createRoot}=await import("react-dom/client");const {TauriVoiceConversation}=await import("../tauri/TauriVoiceConversation");const emptyVoiceLibrary={list:async()=>[],import:async()=>[],remove:async()=>[]};
const root=createRoot(document.getElementById("root")!);
const sink:{current:((e:TauriBridgeEvent)=>void)|null}={current:null};
let sendGate:Promise<boolean>|undefined,captureGate:Promise<{stop:()=>Promise<string>;cancel:()=>void}>|undefined,synthGate:Promise<string>|undefined,recognitionGate:Promise<string>|undefined;
let playbackGate:Promise<void>|undefined;
const calls:string[]=[];const sent:string[]=[];const spoken:string[]=[];let signal:AbortSignal|undefined;let failTTS=false;let failAtSpeechCount:number|undefined;
const capture={stop:async()=>"WAV",cancel:()=>calls.push("capture-cancel")};
const api={providers:async()=>[{name:"mimo",label:"套餐",preferred:true}],transcribe:async()=>recognitionGate??"语音问题",synthesize:async(_id:string,_provider:string,text:string)=>{spoken.push(text);if(failTTS||spoken.length===failAtSpeechCount)throw new Error("语音播报服务暂时不可用；请稍后重试。");return synthGate??"AUDIO";},cancel:async()=>{calls.push("native-cancel");},capture:async(next:AbortSignal)=>{signal=next;return captureGate??capture;},playback:()=>({play:async()=>{calls.push("play");await playbackGate;},stop:()=>{calls.push("stop");},close:()=>{calls.push("close");}})};
const send=async(text:string)=>{sent.push(text);return sendGate??true;};const active=()=>{};const tick=async()=>{await new Promise(resolve=>setTimeout(resolve,0));};
const render=async(owner="session",open=true)=>{await act(async()=>{root.render(<TauriVoiceConversation owner={owner} sequence={sequence} open={open} canSend hardBlocked={false} paused={false} phaseLabel="正在等待模型回答…" eventSink={sink} onOpen={()=>{}} onEnd={()=>calls.push("end")} onActiveChange={active} onSend={send} onOpenSettings={()=>{}} api={api} voiceLibraryAPI={emptyVoiceLibrary}/>);await tick();});};
const key=async(type:"keydown"|"keyup")=>{await act(async()=>{document.querySelector('[aria-label="按住说话，松开发送"]')!.dispatchEvent(new dom.window.KeyboardEvent(type,{key:" ",bubbles:true}));await tick();});};
const click=async(text:string)=>{await act(async()=>{const button=[...document.querySelectorAll<HTMLButtonElement>("button")].find(b=>b.textContent===text);assert(button,text);button.click();await tick();});};
const emit=async(kind:string,payload:Record<string,unknown>={})=>{await act(async()=>{sink.current?.(event(kind,payload));await tick();});};
const visibility=async(hidden:boolean)=>{await act(async()=>{Object.defineProperty(document,"hidden",{configurable:true,value:hidden});document.dispatchEvent(new dom.window.Event("visibilitychange"));await tick();});};
try{
 await render();let accept!:(value:boolean)=>void;sendGate=new Promise(resolve=>{accept=resolve;});
 await key("keydown");assert(document.body.textContent?.includes("正在录音"));await key("keyup");assert.equal(sent.length,1);
 await emit("turn_started");await emit("reasoning",{text:"PRIVATE"});await emit("text",{text:"工具之前"});await emit("tool_dispatch");await emit("text",{text:"这是最终回答。"});await emit("stream_attempt",{streamAttempt:{id:"answer",action:"commit"}});await emit("turn_done",{status:"completed"});
 assert.equal(spoken.length,0,"reply cannot be spoken before submission is confirmed");
 await act(async()=>{accept(true);await tick();});assert.deepEqual(spoken,["这是最终回答。"]);assert.equal(calls.filter(c=>c==="play").length,1);sendGate=undefined;
 await emit("stream_attempt",{streamAttempt:{id:"answer",action:"commit"}});await emit("turn_done",{status:"completed"});assert.equal(spoken.length,1,"no duplicate playback");
 let finishAudio!:(text:string)=>void;synthGate=new Promise(resolve=>{finishAudio=resolve;});await key("keydown");await key("keyup");await emit("turn_started");await emit("text",{text:"迟到声音"});await emit("stream_attempt",{streamAttempt:{id:"answer",action:"commit"}});await emit("turn_done",{status:"completed"});await click("停止播报");
 await act(async()=>{finishAudio("LATE");await tick();});assert.equal(calls.filter(c=>c==="play").length,1,"stopped synthesis cannot play late");synthGate=undefined;
 let grant!:(value:typeof capture)=>void;captureGate=new Promise(resolve=>{grant=resolve;});const sends=sent.length;await key("keydown");await key("keyup");assert(signal?.aborted);await act(async()=>{grant(capture);await tick();});assert.equal(sent.length,sends,"releasing before permission never auto sends a late capture");captureGate=undefined;
 let recognize!:(value:string)=>void;recognitionGate=new Promise(resolve=>{recognize=resolve;});await key("keydown");await key("keyup");await render("other");await act(async()=>{recognize("FOREIGN RESULT");await tick();});assert.equal(sent.length,sends);recognitionGate=undefined;
 await render();failTTS=true;await key("keydown");await key("keyup");await emit("turn_started");await emit("text",{text:"TTS失败的回答"});await emit("stream_attempt",{streamAttempt:{id:"answer",action:"commit"}});await emit("turn_done",{status:"completed"});assert(document.body.textContent?.includes("语音播报服务暂时不可用"));assert(!document.body.textContent?.includes("重新发送识别文字"),"TTS failure must not offer a duplicate model submission");failTTS=false;
 const sendsBeforeRetry=sent.length,speechesBeforeRetry=spoken.length;
 await click("重试播报");assert.equal(sent.length,sendsBeforeRetry,"retrying speech never submits the prompt again");assert.equal(spoken.length,speechesBeforeRetry+1);assert.equal(spoken.at(-1),"TTS失败的回答");
 const splitReply="字".repeat(300),splitChunks=spokenReply(splitReply).chunks;
 const splitStart=spoken.length;failAtSpeechCount=splitStart+2;
 await key("keydown");await key("keyup");await emit("turn_started");await emit("text",{text:splitReply});await emit("turn_done",{status:"completed"});
 assert.deepEqual(spoken.slice(splitStart),splitChunks,"the second chunk fails after the first has played");
 const sendsBeforeResume=sent.length;failAtSpeechCount=undefined;
 await click("重试播报");assert.equal(sent.length,sendsBeforeResume);assert.deepEqual(spoken.slice(splitStart),[...splitChunks,splitChunks[1]],"retry resumes only the failed chunk without replaying completed speech");
 // A temporary hidden window must not discard an already-submitted answer.
 await render("session",false);await render();
 const beforeHidden=spoken.length;
 await key("keydown");await key("keyup");await emit("turn_started");
 await visibility(true);
 await emit("text",{text:"返回窗口后应当朗读的回答。"});await emit("stream_attempt",{streamAttempt:{id:"visible-answer",action:"commit"}});await emit("turn_done",{status:"completed"});
 assert.equal(spoken.length,beforeHidden,"a hidden window does not start TTS");
 await visibility(false);
 assert.equal(spoken.length,beforeHidden+1,"returning to the window must speak the submitted answer exactly once");

 // Idle visibility changes must not falsely report a cancelled conversation.
 await visibility(true);assert(!document.body.textContent?.includes("录音与播报已停止"));await visibility(false);
 // A result already being synthesized is retained, but no hidden audio starts.
 let completeTTS!:(value:string)=>void;synthGate=new Promise(resolve=>{completeTTS=resolve;});
 const beforeSynthesis=spoken.length,playsBeforeSynthesis=calls.filter(c=>c==="play").length;
 await key("keydown");await key("keyup");await emit("turn_started");await emit("text",{text:"合成过程中隐藏的回答。"});await emit("turn_done",{status:"completed"});
 assert.equal(spoken.length,beforeSynthesis+1);await visibility(true);
 await act(async()=>{completeTTS("DEFERRED AUDIO");await tick();});assert.equal(calls.filter(c=>c==="play").length,playsBeforeSynthesis);
 await visibility(false);assert.equal(calls.filter(c=>c==="play").length,playsBeforeSynthesis+1);assert.equal(spoken.length,beforeSynthesis+1,"return does not request or charge TTS twice");synthGate=undefined;
 // Returning cannot bypass submission confirmation.
 let confirmHidden!:(value:boolean)=>void;sendGate=new Promise(resolve=>{confirmHidden=resolve;});
 const beforeAdmission=spoken.length;
 await key("keydown");await key("keyup");await emit("turn_started");await visibility(true);await emit("text",{text:"确认发送后才能播报。"});await emit("turn_done",{status:"completed"});await visibility(false);
 assert.equal(spoken.length,beforeAdmission);await act(async()=>{confirmHidden(true);await tick();});assert.equal(spoken.length,beforeAdmission+1);sendGate=undefined;
 // Explicit stop and session replacement still revoke the deferred reply.
 for(const action of ["stop","switch"]){
  const beforeRevocation=spoken.length;
  await key("keydown");await key("keyup");await emit("turn_started");await visibility(true);await emit("text",{text:"已经取消的待播报回答。"});await emit("turn_done",{status:"completed"});
  if(action==="stop")await click("停止播报");else await render("other");
  await visibility(false);assert.equal(spoken.length,beforeRevocation,"deferred answer cannot survive "+action);await render();
 }
 // Recording and active audio still stop on hide; neither restarts on return.
 const beforeRecording=sent.length;await key("keydown");await visibility(true);assert(signal?.aborted);await visibility(false);await key("keyup");assert.equal(sent.length,beforeRecording);
 let finishPlayback!:()=>void;playbackGate=new Promise(resolve=>{finishPlayback=resolve;});
 await key("keydown");await key("keyup");await emit("turn_started");await emit("text",{text:"播放开始后隐藏窗口。"});await emit("turn_done",{status:"completed"});
 const beforePlayingHide=calls.filter(c=>c==="play").length;await visibility(true);assert(signal?.aborted);await visibility(false);await act(async()=>{finishPlayback();await tick();});assert.equal(calls.filter(c=>c==="play").length,beforePlayingHide);playbackGate=undefined;
 await key("keydown");await click("切换到文字输入");assert(signal?.aborted);assert(calls.includes("close"));
 console.log("PASS voice conversation: final answer only, admission fence, exactly-once playback, stop/late audio, late permission, session fence, TTS failure without resubmit, hidden waiting/synthesis/admission, stop/session revocation, recording/playback privacy, end cleanup");
}finally{await act(async()=>root.unmount());dom.window.close();}
