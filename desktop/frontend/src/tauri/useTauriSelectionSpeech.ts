import { useCallback, useEffect, useRef, useState } from "react";
import { tauriMessageFrom, tauriSpeechProviders, tauriSpeechSynthesize, tauriSpeechCancel } from "../lib/tauriBridge";
import { createVoicePlayback, spokenReply, type VoicePlayback } from "./voiceConversation";
import { readVoicePreferences } from "./voicePreferences";

const speechAPI={providers:tauriSpeechProviders,synthesize:tauriSpeechSynthesize,cancel:tauriSpeechCancel,playback:createVoicePlayback};
type Reading={owner:string;controller:AbortController;requests:Set<string>;playback:VoicePlayback};
type RetainedSpeech={owner:string;chunks:string[];truncated:boolean;provider:string;voice:string};
export function useTauriSelectionSpeech({owner,blocked,voiceCallActive,onStart,api=speechAPI}:{owner:string;blocked:boolean;voiceCallActive:boolean;onStart:()=>void;api?:typeof speechAPI}) {
 const [status,setStatus]=useState("");
 const [error,setError]=useState("");
 const [retrySpeech,setRetrySpeech]=useState<RetainedSpeech|null>(null);
 const reading=useRef<Reading|null>(null);
 const current=useRef({owner,blocked,onStart});current.current={owner,blocked,onStart};
 const stop=useCallback(()=>{
  const pending=reading.current;reading.current=null;
  if(pending){pending.controller.abort();pending.playback.close();for(const id of pending.requests)void api.cancel(id).catch(()=>{});}
  setStatus("");
  setRetrySpeech(null);
 },[api]);
 useEffect(()=>{setError("");return stop;},[owner,stop]);
 useEffect(()=>{if(blocked||voiceCallActive)stop();},[blocked,voiceCallActive,stop]);
 useEffect(()=>{
  const hidden=()=>{if(document.hidden)stop();};
  const escape=(event:KeyboardEvent)=>{if(event.key==="Escape"&&reading.current)stop();};
  document.addEventListener("visibilitychange",hidden);document.addEventListener("keydown",escape);
  return()=>{document.removeEventListener("visibilitychange",hidden);document.removeEventListener("keydown",escape);};
 },[stop]);
 const speak=useCallback((resolveText:()=>Promise<string|null>,retained?:RetainedSpeech)=>{
  if(current.current.blocked||!current.current.owner)return;
  stop();setError("");
  let pending:Reading;
  try {
   // Unlock while the selection action still owns the user gesture. Logical
   // selections may need asynchronous materialization before yielding text.
   pending={owner:current.current.owner,controller:new AbortController(),requests:new Set(),playback:api.playback()};
  }catch(cause){setError(tauriMessageFrom(cause));if(retained)setRetrySpeech(retained);return;}
  reading.current=pending;current.current.onStart();setStatus("正在准备选文播报…");
  const valid=()=>reading.current===pending&&!pending.controller.signal.aborted&&current.current.owner===pending.owner&&!current.current.blocked&&!document.hidden;
  void (async()=>{
   let prepared:RetainedSpeech|undefined;
   let chunkIndex=0;
   try {
    if(retained)prepared=retained;
    else{
     const text=await resolveText();if(!valid())return;
     if(!text)return;
     const {chunks,truncated}=spokenReply(text);
     if(!chunks.length)throw new Error("选中的内容没有可播报的文字；请重新选择正文。");
     const providers=await api.providers();if(!valid())return;
     const preferences=readVoicePreferences(),speakers=providers.filter(entry=>!entry.name.startsWith("custom:"));
     const speaker=speakers.find(entry=>entry.name===preferences.speaker)??speakers.find(entry=>entry.preferred)??speakers[0];
     if(!speaker)throw new Error("没有可用的播报渠道；请在语音识别设置中配置 MiMo 服务。");
     prepared={owner:pending.owner,chunks,truncated,provider:speaker.name,voice:preferences.voice};
    }
    for(;chunkIndex<prepared.chunks.length;chunkIndex++){
     if(!valid())return;
     setStatus(prepared.truncated?"正在播报选中文字（前 4000 字）…":"正在播报选中文字…");
     const id=crypto.randomUUID();pending.requests.add(id);
     const audio=await api.synthesize(id,prepared.provider,prepared.chunks[chunkIndex],prepared.voice);pending.requests.delete(id);
     if(!valid())return;
     await pending.playback.play(audio,pending.controller.signal);
    }
   }catch(cause){if(valid()){
    setError(tauriMessageFrom(cause));
    if(prepared)setRetrySpeech({...prepared,chunks:prepared.chunks.slice(chunkIndex)});
   }}
   finally{if(reading.current===pending){reading.current=null;pending.playback.close();setStatus("");}}
  })();
 },[api,stop]);
 const canRetry=Boolean(retrySpeech&&retrySpeech.owner===owner&&!blocked&&!voiceCallActive);
 const retry=()=>{if(canRetry&&retrySpeech)speak(async()=>null,retrySpeech);};
 return {speak,stop,status,error,canRetry,retry,dismissError:()=>{setError("");setRetrySpeech(null);}};
}
