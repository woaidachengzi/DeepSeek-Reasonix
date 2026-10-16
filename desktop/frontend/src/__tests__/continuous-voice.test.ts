import assert from "node:assert/strict";
import vm from "node:vm";
import {readFile} from "node:fs/promises";
import {VoiceActivity,removePlaybackEcho,rms,likelyPlaybackEcho} from "../tauri/voiceActivity";
import {spokenSentences} from "../tauri/voiceConversation";
import {ContinuousVoice,type ContinuousState} from "../tauri/continuousVoice";
import type {TauriBridgeEvent} from "../lib/tauriBridge";
const silence=new Float32Array(320);
let frameNumber=0;
const speech=()=>{const frame=new Float32Array(320),gain=frameNumber++%3===0?0.09:0.04;for(let i=0;i<frame.length;i++)frame[i]=Math.sin((frameNumber*320+i)*0.09)*gain;return frame;};
let starts=0,discards=0;const utterances:{audio:string;limited:boolean}[]=[];
const vad=new VoiceActivity({start:()=>starts++,end:(audio,limited)=>utterances.push({audio,limited}),discard:()=>discards++});
for(let i=0;i<6000;i++)vad.push(silence,true);assert.equal(starts,0);assert.equal(utterances.length,0,"silence never uploads");
for(let i=0;i<25;i++)vad.push(speech(),true);for(let i=0;i<46;i++)vad.push(silence,true);
assert.equal(starts,1);assert.equal(utterances.length,1);const wav=Buffer.from(utterances[0].audio,"base64");assert.equal(wav.toString("ascii",0,4),"RIFF");assert.equal(wav.readUInt32LE(24),16000);assert(wav.length>44+32000*0.5&&wav.length<44+32000*2);
for(let i=0;i<10;i++)vad.push(speech(),true);for(let i=0;i<46;i++)vad.push(silence,true);assert.equal(utterances.length,1);assert.equal(discards,1,"a short noise candidate releases the recording state");
vad.reset();const hum=new Float32Array(320);for(let i=0;i<320;i++)hum[i]=Math.sin(i*0.15)*0.04;
for(let i=0;i<50;i++)vad.push(hum,true);for(let i=0;i<46;i++)vad.push(silence,true);assert.equal(utterances.length,1,"steady hum is not sent as speech");
const fresh=new VoiceActivity({start:()=>{},end:(audio,limited)=>{assert(limited);assert(Buffer.from(audio,"base64").length<=44+16000*2*90);}});for(let i=0;i<4401;i++)fresh.push(speech(),true);
let seed=7;const reference=new Float32Array(6400);for(let i=0;i<reference.length;i++){seed=(Math.imul(seed,1664525)+1013904223)|0;reference[i]=(seed/2147483648)*0.06;}
const echo=reference.slice(1243,1563).map(x=>x*0.7);assert(rms(removePlaybackEcho(echo,reference))<0.002,"correlated playback is removed");
const mixed=echo.slice();for(let i=0;i<mixed.length;i++)mixed[i]+=Math.sin(i*0.09)*0.04;assert(rms(removePlaybackEcho(mixed,reference))>0.02,"near-end double talk remains audible to VAD");
const echoVAD=new VoiceActivity({start:()=>{throw new Error("echo triggered speech");},end:()=>{throw new Error("echo uploaded");}});for(let i=0;i<200;i++)echoVAD.push(echo,true,reference);
assert(likelyPlaybackEcho("这是刚刚播放的完整回答","这是刚刚播放的完整回答。"));assert(!likelyPlaybackEcho("请换一个新的解释","这是刚刚播放的完整回答。"));
const sentences=spokenSentences("这是第一句话，说明这次是分句播报并提前准备下一句声音。接下来是第二句话，它也应该作为独立的声音段落播放。");assert.equal(sentences.chunks.length,2);assert(sentences.chunks.every(text=>Array.from(text).length<=180));
// The actual worklet batches by sample time and mixes channels without output.
let Processor: (new()=>{process(inputs:Float32Array[][]):boolean})|undefined;const packets:{samples:Float32Array;rate:number}[]=[];
vm.runInNewContext(await readFile(new URL("../../public/voice-capture-worklet.js",import.meta.url),"utf8"),{sampleRate:48000,Float32Array,AudioWorkletProcessor:class{port={postMessage:(packet:{samples:Float32Array;rate:number})=>packets.push(packet)};},registerProcessor:(_name:string,type:typeof Processor)=>{Processor=type;}});
const processor=new Processor!();for(let i=0;i<8;i++)processor.process([[new Float32Array(128).fill(0.4),new Float32Array(128).fill(0.2)]]);assert.equal(packets.length,1);assert.equal(packets[0].samples.length,960);assert(Math.abs(packets[0].samples[0]-0.3)<0.001);
let sequence=0;const event=(kind:string,payload:Record<string,unknown>={}):TauriBridgeEvent=>({protocolVersion:1,sessionId:"session",sequence:++sequence,eventKind:kind,payload:{kind,...payload}});
const tick=async()=>{for(let i=0;i<4;i++)await new Promise(resolve=>setTimeout(resolve,0));};
let feed:(frame:Float32Array)=>void=()=>{},captureClosed=0,played=0,stopped=0,cancelled=0,synthesized:string[]=[],sent:string[]=[],states:ContinuousState[]=[];
let reply="请解释一下这个功能",sendGate:Promise<boolean>|undefined,captureGate:Promise<{close:()=>void;echoCancellation:boolean}>|undefined,synthGate:Promise<string>|undefined,recognizeGate:Promise<string>|undefined;
let releasePlay:()=>void=()=>{};let holdPlay=true;let echoEnabled=true;
const host={owner:"session",sequence:0,canSend:true,blocked:false,send:async(text:string)=>{sent.push(text);return sendGate??true;}};
const api={capture:async(_signal:AbortSignal,onFrame:(frame:Float32Array)=>void)=>{feed=onFrame;return captureGate??{close:()=>captureClosed++,echoCancellation:echoEnabled};},transcribe:async()=>recognizeGate??reply,synthesize:async(_id:string,_provider:string,text:string)=>{synthesized.push(text);return synthesized.length%2===0&&synthGate?synthGate:"AUDIO";},cancel:async()=>{cancelled++;},playback:()=>({reference:()=>null,play:async(_audio:string,signal:AbortSignal)=>{played++;if(holdPlay)await new Promise<void>((resolve,reject)=>{const abort=()=>{signal.removeEventListener("abort",abort);reject(new Error("cancelled"));};signal.addEventListener("abort",abort,{once:true});releasePlay=()=>{signal.removeEventListener("abort",abort);resolve();};});},stop:()=>{stopped++;},close:()=>{stopped++;}})};
const options={recognizer:"mimo",speaker:"mimo",voice:"mimo_default",bargeIn:true,sensitivity:1};
const talk=async()=>{for(let i=0;i<25;i++)feed(speech());for(let i=0;i<46;i++)feed(silence);await tick();};
const complete=(engine:ContinuousVoice,text:string)=>{engine.accept(event("turn_started"));engine.accept(event("reasoning",{text:"PRIVATE"}));engine.accept(event("text",{text}));engine.accept(event("stream_attempt",{streamAttempt:{id:"answer",action:"commit"}}));engine.accept(event("turn_done",{status:"completed"}));};
const engine=new ContinuousVoice({...options},()=>host,state=>states.push(state),api);await engine.start();for(let i=0;i<100;i++)feed(silence);assert.equal(sent.length,0);
let admit!:(value:boolean)=>void;sendGate=new Promise(resolve=>admit=resolve);await talk();assert.equal(sent.length,1);
complete(engine,"这是第一句话，说明这次是分句播报并提前准备下一句声音。接下来是第二句话，它也应该作为独立的声音段落播放。");await tick();assert.equal(synthesized.length,0,"completion waits for confirmed admission");
let late!:(audio:string)=>void;synthGate=new Promise(resolve=>late=resolve);admit(true);await tick();assert.equal(played,1);assert.equal(synthesized.length,2,"next sentence prefetches during current playback");sendGate=undefined;
reply="请重新给一个更短的回答";await talk();assert.equal(sent.length,2,"speaking interrupts output and creates one new turn");assert(cancelled>=2);assert(stopped>0);
late("LATE PREFETCH");await tick();assert.equal(played,1,"late prefetched audio never plays after barge-in");synthGate=undefined;
engine.pause();assert(captureClosed>0);assert.equal(states[states.length-1]?.phase,"off");const count=sent.length;await talk();assert.equal(sent.length,count,"paused frames do not submit");
// A late permission grant is closed after pause, and stale ASR cannot cross owner.
let grant!:(capture:{close:()=>void;echoCancellation:boolean})=>void;captureGate=new Promise(resolve=>grant=resolve);const lateEngine=new ContinuousVoice({...options},()=>host,state=>states.push(state),api);const connecting=lateEngine.start();lateEngine.pause();grant({close:()=>captureClosed++,echoCancellation:true});await connecting;captureGate=undefined;assert.equal(states[states.length-1]?.phase,"off");
let recognize!:(text:string)=>void;recognizeGate=new Promise(resolve=>recognize=resolve);const foreign=new ContinuousVoice({...options},()=>host,state=>states.push(state),api);await foreign.start();await talk();host.owner="other";foreign.update();recognize("STALE");await tick();assert.equal(sent.length,count);host.owner="session";recognizeGate=undefined;
// No verified device AEC => continuous turns work, automatic barge-in is off.
echoEnabled=false;const protectedEngine=new ContinuousVoice({...options},()=>host,state=>states.push(state),api);await protectedEngine.start();assert(states[states.length-1]?.notice.includes("已关闭"));await talk();complete(protectedEngine,"这是正在播放的一句话，未确认系统回声消除时不会被声音打断。");await tick();const protectedCount=sent.length;await talk();assert.equal(sent.length,protectedCount);protectedEngine.stopPlayback();releasePlay();await tick();assert.equal(states[states.length-1]?.phase,"listening");protectedEngine.pause();echoEnabled=true;
// Reply echo returned by ASR is discarded before any new model admission.
const echoEngine=new ContinuousVoice({...options},()=>host,state=>states.push(state),api);await echoEngine.start();reply="正常的新问题";await talk();const repeated="这是一个足够长的播报内容，用来检查重复回声不会再次发送。";complete(echoEngine,repeated);await tick();const beforeEcho=sent.length;reply=repeated;await talk();assert.equal(sent.length,beforeEcho);assert(states[states.length-1]?.notice.includes("未发送"));echoEngine.pause();holdPlay=false;
console.log("PASS continuous voice: PCM worklet, silence/onset/hangover/cap, echo subtraction/double talk, sentence prefetch, admission fence, barge-in/late audio, pause/late permission, owner fence, AEC fallback and duplicate echo rejection");
// Actual capture startup must close a microphone granted after cancellation.
const originalNavigator=Object.getOwnPropertyDescriptor(globalThis,"navigator");
let lateGrant!:(stream:MediaStream)=>void,trackStops=0,contextCloses=0;
Object.defineProperty(globalThis,"navigator",{configurable:true,value:{mediaDevices:{getUserMedia:()=>new Promise<MediaStream>(resolve=>lateGrant=resolve)}}});
Object.assign(globalThis,{AudioContext:class{resume(){return Promise.resolve();}close(){contextCloses++;return Promise.resolve();}},AudioWorkletNode:class{}});
const {startContinuousCapture}=await import("../tauri/continuousCapture");
const abort=new AbortController();const pendingCapture=startContinuousCapture(abort.signal,()=>{throw new Error("late frame");},()=>{});abort.abort();
lateGrant({getTracks:()=>[{stop:()=>trackStops++}]} as unknown as MediaStream);
await assert.rejects(pendingCapture,/取消/);assert.equal(trackStops,1);assert.equal(contextCloses,1);
if(originalNavigator)Object.defineProperty(globalThis,"navigator",originalNavigator);else Reflect.deleteProperty(globalThis,"navigator");
console.log("PASS actual continuous capture releases a late permission grant");
