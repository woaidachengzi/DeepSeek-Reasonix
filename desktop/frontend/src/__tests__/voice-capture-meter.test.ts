import assert from "node:assert/strict";
import { startVoiceCapture } from "../tauri/voiceCapture";

let energy=0,closed=0,resumed=0,tracksStopped=0,destinationConnections=0;
let sequence=0;const frames=new Map<number,FrameRequestCallback>();
Object.assign(globalThis,{requestAnimationFrame:(callback:FrameRequestCallback)=>{frames.set(++sequence,callback);return sequence;},cancelAnimationFrame:(id:number)=>{frames.delete(id);}});
const frame=(time:number)=>{for(const [id,callback] of [...frames]){frames.delete(id);callback(time);}};
class Context {
 destination={};
 async resume(){resumed++;}
 async close(){closed++;}
 createMediaStreamSource(){return{connect:(node:unknown)=>{if(node===this.destination)destinationConnections++;},disconnect:()=>{}};}
 createAnalyser(){return{fftSize:256,getFloatTimeDomainData:(samples:Float32Array)=>samples.fill(energy),disconnect:()=>{}};}
 async decodeAudioData(){return{numberOfChannels:1,sampleRate:16000,getChannelData:()=>new Float32Array(1600).fill(0.1)};}
}
let recorderFailure=false;
class Recorder {
 state="inactive";mimeType="audio/webm";
 ondataavailable:((event:{data:Blob})=>void)|null=null;onstop:(()=>void)|null=null;
 constructor(_stream:MediaStream){if(recorderFailure)throw new Error("unavailable");}
 start(){this.state="recording";}
 stop(){this.state="inactive";this.ondataavailable?.({data:new Blob(["fixture"])});this.onstop?.();}
}
const stream=()=>({getTracks:()=>[{stop:()=>{tracksStopped++;}}]} as unknown as MediaStream);
let getMedia:()=>Promise<MediaStream>=async()=>stream();
Object.defineProperty(globalThis,"navigator",{configurable:true,value:{mediaDevices:{getUserMedia:()=>getMedia()}}});
Object.assign(globalThis,{AudioContext:Context,MediaRecorder:Recorder});
const levels:number[]=[];
const controller=new AbortController();const capture=await startVoiceCapture(controller.signal,()=>{},level=>levels.push(level));
assert.equal(resumed,1,"meter unlocks during start");energy=0;frame(0);assert.equal(levels[levels.length - 1],0,"silence stays flat");
energy=0.1;frame(60);assert(levels[levels.length - 1]!>0.5,"actual RMS drives the volume meter");
energy=0;for(let i=2;i<16;i++)frame(i*60);assert(levels[levels.length - 1]!<0.02,"meter falls back to silence");
assert.equal(destinationConnections,0,"capture meter never routes microphone to output");
const encoded=await capture.stop();assert(encoded.length>100);assert.equal(frames.size,0);assert.equal(levels[levels.length - 1],0);assert.equal(closed,2,"meter and decode contexts close after finish");assert.equal(tracksStopped,1);
const cancelled=new AbortController();await startVoiceCapture(cancelled.signal,()=>{},()=>{});cancelled.abort();assert.equal(frames.size,0);assert.equal(closed,3);assert.equal(tracksStopped,2);
let grant!:(stream:MediaStream)=>void;getMedia=()=>new Promise(resolve=>{grant=resolve;});
const late=new AbortController(),waiting=startVoiceCapture(late.signal,()=>{},()=>{});late.abort();assert.equal(closed,4,"cancelling pending permission immediately closes the meter");grant(stream());await assert.rejects(waiting,/取消/);assert.equal(tracksStopped,3);assert.equal(closed,4,"late grant does not re-close or restart the meter");
getMedia=async()=>{throw Object.assign(new Error("denied"),{name:"NotAllowedError"});};await assert.rejects(startVoiceCapture(new AbortController().signal,()=>{},()=>{}),/denied/);assert.equal(closed,5);
getMedia=async()=>stream();recorderFailure=true;await assert.rejects(startVoiceCapture(new AbortController().signal,()=>{},()=>{}),/无法启动录音/);assert.equal(closed,6);assert.equal(tracksStopped,4);
console.log("PASS capture meter: real RMS/silence decay, no speaker connection, finish/cancel cleanup, pending/late permission, denial and recorder failure release all resources");
