import { useEffect, useRef, useState, type RefObject } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { isTauri } from "@tauri-apps/api/core";
import { Mic, Pause, Square } from "lucide-react";
import { ContinuousVoice, continuousAPI, type ContinuousState } from "./continuousVoice";
import type { VoiceEventSink } from "./TauriVoiceConversation";

const labels:Record<ContinuousState["phase"],string>={off:"连续聆听已暂停",connecting:"正在连接麦克风…",listening:"正在聆听，直接说话即可",recording:"正在听你说话，停顿后自动发送…",recognizing:"正在识别语音…",waiting:"正在等待模型回答…",synthesizing:"正在准备下一句声音…",speaking:"正在分句播报…",blocked:"等待当前对话空闲后继续聆听…"};
export function TauriContinuousVoice({owner,sequence,canSend,blocked,paused,phaseLabel,recognizer,speaker,voice,eventSink,onSend,onRunning,settingsExpanded=true,api=continuousAPI}:{
 settingsExpanded?:boolean;owner:string;sequence:number;canSend:boolean;blocked:boolean;paused:boolean;phaseLabel:string;recognizer:string;speaker:string;voice:string;
 eventSink:RefObject<VoiceEventSink|null>;onSend:(text:string)=>Promise<boolean>;onRunning:(running:boolean)=>void;api?:typeof continuousAPI;
}){
 const [state,setState]=useState<ContinuousState>({phase:"off",text:"",notice:"",error:""});
 const [bargeIn,setBargeIn]=useState(true);
 const [sensitivity,setSensitivity]=useState(1);
 const engine=useRef<ContinuousVoice|null>(null);
 const alive=useRef(true);
 const current=useRef({owner,sequence,canSend,blocked,onSend,onRunning});current.current={owner,sequence,canSend,blocked,onSend,onRunning};
 useEffect(()=>{
  alive.current=true;let active=true;
  const handler:VoiceEventSink=event=>engine.current?.accept(event);eventSink.current=handler;
  const hidden=()=>{if(document.hidden)engine.current?.pause("页面已隐藏，连续聆听已暂停，麦克风已关闭。");};
  const blur=()=>engine.current?.pause("窗口已失去焦点，连续聆听已暂停。");
  let unlisten:(()=>void)|undefined;
  if(isTauri())void getCurrentWindow().onFocusChanged(event=>{if(!event.payload&&active)blur();}).then(off=>{if(active)unlisten=off;else off();}).catch(()=>{});
  document.addEventListener("visibilitychange",hidden);window.addEventListener("blur",blur);
  return ()=>{active=false;alive.current=false;unlisten?.();if(eventSink.current===handler)eventSink.current=null;engine.current?.pause();engine.current=null;current.current.onRunning(false);document.removeEventListener("visibilitychange",hidden);window.removeEventListener("blur",blur);};
 },[owner,eventSink]);
 useEffect(()=>{engine.current?.update();},[canSend,blocked,owner]);
 const running=state.phase!=="off";
 const start=()=>{
  if(blocked||!canSend||!recognizer||!speaker)return;
  engine.current?.pause();
  const next=new ContinuousVoice({recognizer,speaker,voice,bargeIn,sensitivity},()=>({owner:current.current.owner,sequence:current.current.sequence,canSend:current.current.canSend,blocked:current.current.blocked,send:text=>current.current.onSend(text)}),value=>{if(alive.current){setState(value);current.current.onRunning(value.phase!=="off");}},api);
  engine.current=next;void next.start();
 };
 return <div className="tauri-continuous-voice" aria-label="连续语音对话">
  <div hidden={!settingsExpanded}><p>开始后会持续使用麦克风，检测到说话并停顿约 0.9 秒后发送；回答按句播放，并预加载下一句。暂停、结束或离开窗口会关闭麦克风。</p>
  <label className="tauri-continuous-option"><input type="checkbox" checked={bargeIn} disabled={running} onChange={event=>setBargeIn(event.target.checked)}/>说话时打断播报</label>
  <label>拾音灵敏度<select aria-label="连续对话拾音灵敏度" value={sensitivity} disabled={running} onChange={event=>setSensitivity(Number(event.target.value))}><option value={1.3}>低（环境较吵）</option><option value={1}>标准</option><option value={0.75}>高（轻声说话）</option></select></label>
  </div><p role="status" className="tauri-continuous-status"><span className={running?"is-live":""} aria-hidden="true"/>{state.phase==="waiting"?(paused?"等待你的确认，请在聊天区处理…":phaseLabel):labels[state.phase]}</p>
  {settingsExpanded&&state.text&&<p className="tauri-voice-call-recognized">你说：{state.text}</p>}
  {state.notice&&<p role="status">{state.notice}</p>}{state.error&&<p role="alert" className="tauri-voice-error">{state.error}</p>}
  {running?<button type="button" onClick={()=>engine.current?.pause()}><Pause size={14}/>暂停聆听</button>:<button type="button" disabled={blocked||!canSend||!recognizer||!speaker} onClick={start}><Mic size={14}/>开始连续对话</button>}
  {(state.phase==="speaking"||state.phase==="synthesizing"||state.phase==="waiting")&&<button type="button" onClick={()=>engine.current?.stopPlayback()}><Square size={14}/>停止播报</button>}
  <p hidden={!settingsExpanded}>外放时的打断效果取决于设备回声消除；使用耳机更稳定。也可以关闭“说话时打断播报”，等播放结束再说。</p>
 </div>;
}
