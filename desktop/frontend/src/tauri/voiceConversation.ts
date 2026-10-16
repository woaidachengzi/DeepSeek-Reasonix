import { mono16k } from "./voiceActivity";
import { tauriAssistantTextDelta, type TauriBridgeEvent } from "../lib/tauriBridge";

export interface VoiceReply { text: string; error?: string }
/** Only the explicitly submitted turn can become a spoken reply. */
export class VoiceReplyTracker {
 private started = false;
 private text = "";
 private sequence: number;
 private done = false;
 private turnID = "";
 constructor(private sessionID: string, afterSequence: number) { this.sequence = afterSequence; }
 accept(event: TauriBridgeEvent): VoiceReply | null {
  if (this.done || event.sessionId !== this.sessionID || event.sequence <= this.sequence || event.payload.kind !== event.eventKind) return null;
  this.sequence = event.sequence;
  const turn = typeof event.payload.turnId === "string" ? event.payload.turnId : "";
  if (event.eventKind === "turn_started") {
   if (this.started) { this.done = true; return {text:"",error:"当前回合已变化；本次语音播报已取消。"}; }
   this.started = true; this.turnID = turn; return null;
  }
  if (!this.started || (this.turnID && turn && turn !== this.turnID)) return null;
  // A successful attempt commits after its text. Only a new/discarded
  // attempt invalidates the accumulated answer; commit preserves it for TTS.
  const attempt = event.payload.streamAttempt;
  const action = attempt && typeof attempt === "object" && !Array.isArray(attempt)
   ? (attempt as Record<string, unknown>).action : undefined;
  if (event.eventKind === "tool_dispatch" || (event.eventKind === "stream_attempt" && (action === "begin" || action === "discard"))) this.text = "";
  const delta = tauriAssistantTextDelta(event);
  if (delta) this.text = (this.text + delta).slice(0, 64000);
  if (event.eventKind !== "turn_done") return null;
  this.done = true;
  const status = event.payload.status;
  if ((status && status !== "completed" && status !== "ok" && status !== "success") || event.payload.err || event.payload.outcome === "cancelled" || event.payload.outcome === "interrupted") return {text:"",error:"本轮未正常完成；请查看对话中的状态和错误提示。"};
  return {text:this.text};
 }
}

/** Keep code, markup and long answers in the transcript; speak bounded prose. */
export function spokenReply(text: string): { chunks: string[]; truncated: boolean } {
 const prose = text.replace(/```[\s\S]*?(?:```|$)/g, "").replace(/~~~[\s\S]*?(?:~~~|$)/g, "")
  .replace(/!\[[^\]]*\]\([^)]*\)/g, "").replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
  .replace(/<[^>]*>/g, "").replace(/[*_`#>|]/g, "").trim();
 const chars = Array.from(prose), truncated = chars.length > 4000;
 const chunks: string[] = []; const bounded = chars.slice(0,4000);
 while (bounded.length) chunks.push(bounded.splice(0,280).join(""));
 return {chunks,truncated};
}
export interface VoicePlayback { play(audio: string, signal: AbortSignal): Promise<void>; stop(): void; close(): void; reference?(): Float32Array | null }
export function createVoicePlayback(): VoicePlayback {
 const context = new AudioContext();
 // Called synchronously from the user's press gesture to unlock native audio.
 const ready = context.resume(); void ready.catch(() => {});
 let source: AudioBufferSourceNode | null = null;
 let rendered: {samples:Float32Array;start:number;stop:number}|null=null;
 const stop = () => { const current=source; source=null; if(current&&rendered) rendered.stop=context.currentTime; if(current) { try {current.stop();} catch { /* Already stopped. */ } current.disconnect(); } };
 return {
  stop,
  reference() {
   if(!rendered)return null;
   const now=context.currentTime;
   const end=Math.min(rendered.samples.length,Math.floor((Math.min(now,rendered.stop)-rendered.start)*16000));
   const start=Math.max(0,Math.floor((now-rendered.start-0.45)*16000));
   return end>start?rendered.samples.subarray(start,end):null;
  },
  close() { stop(); void context.close().catch(() => {}); },
  async play(encoded, signal) {
   await ready; if(signal.aborted) throw new Error("播报已取消");
   const bytes=Uint8Array.from(atob(encoded),char=>char.charCodeAt(0));
   const buffer=await context.decodeAudioData(bytes.buffer);
   if(signal.aborted) throw new Error("播报已取消");
   await new Promise<void>((resolve,reject)=>{
    const node=context.createBufferSource(); source=node; node.buffer=buffer; node.connect(context.destination);
    rendered={samples:mono16k(Array.from({length:buffer.numberOfChannels},(_,i)=>buffer.getChannelData(i)),buffer.sampleRate),start:context.currentTime,stop:Infinity};
    const finish=(error?: Error)=>{ signal.removeEventListener("abort",abort); node.onended=null; node.disconnect(); if(source===node) {source=null;if(rendered)rendered.stop=context.currentTime;} if(error) reject(error); else resolve(); };
    const abort=()=>{ try {node.stop();} catch { /* Already stopped. */ } finish(new Error("播报已取消")); };
    node.onended=()=>finish(); signal.addEventListener("abort",abort,{once:true});
    try {node.start();} catch {finish(new Error("无法播放声音；请检查系统输出设备。"));}
   });
  },
 };
}

/** Short complete sentences, bounded by the same public-prose and length rules. */
export function spokenSentences(text:string):{chunks:string[];truncated:boolean} {
 const plain=spokenReply(text),chars=Array.from(plain.chunks.join("")),chunks:string[]=[];let sentence="";
 for(let i=0;i<chars.length;i++){
  const char=chars[i];sentence+=char;
  const boundary=/[。！？!?；;\n]/.test(char)||(char==="."&&(!chars[i+1]||/\s/.test(chars[i+1])));
  if(Array.from(sentence).length>=160||(boundary&&Array.from(sentence).length>=24)){if(sentence.trim())chunks.push(sentence.trim());sentence="";}
 }
 if(sentence.trim())chunks.push(sentence.trim());
 return {chunks,truncated:plain.truncated};
}
