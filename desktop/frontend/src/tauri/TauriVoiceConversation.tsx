import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { Headphones, Mic, PhoneOff, Square, Volume2 } from "lucide-react";
import { tauriSpeechProviders, tauriSpeechTranscribe, tauriSpeechSynthesize, tauriSpeechCancel, tauriMessageFrom, type TauriSpeechProvider, type TauriBridgeEvent } from "../lib/tauriBridge";
import { TauriContinuousVoice } from "./TauriContinuousVoice";
import { startVoiceCapture, type VoiceCapture } from "./voiceCapture";
import { VoiceReplyTracker, spokenReply, createVoicePlayback, type VoicePlayback, type VoiceReply } from "./voiceConversation";

const callAPI = { providers: tauriSpeechProviders, transcribe: tauriSpeechTranscribe, synthesize: tauriSpeechSynthesize, cancel: tauriSpeechCancel, capture: startVoiceCapture, playback: createVoicePlayback };
type Phase = "idle" | "preparing" | "recording" | "recognizing" | "waiting" | "synthesizing" | "speaking";
type CallOperation = { controller: AbortController; requests: Set<string>; capture?: VoiceCapture; released?: boolean; finishing?: boolean; tracker?: VoiceReplyTracker; admitted?: boolean; reply?: VoiceReply; ttsProvider: string; voice: string };
export type VoiceEventSink = (event: TauriBridgeEvent) => void;

export function TauriVoiceConversation({ owner, sequence, open, hardBlocked, canSend, paused, phaseLabel, eventSink, onOpen, onEnd, onActiveChange, onSend, onOpenSettings, api = callAPI }: {
 owner: string; sequence: number; open: boolean; hardBlocked: boolean; canSend: boolean; paused: boolean; phaseLabel: string;
 eventSink: RefObject<VoiceEventSink | null>; onOpen: () => void; onEnd: () => void; onActiveChange: (active: boolean) => void;
 onSend: (text: string) => Promise<boolean>; onOpenSettings: () => void; api?: typeof callAPI;
}) {
 const [providers,setProviders] = useState<TauriSpeechProvider[]>([]);
 const [recognizer,setRecognizer] = useState("");
 const [speaker,setSpeaker] = useState("");
 const [voice,setVoice] = useState("mimo_default");
 const [previewPhase,setPreviewPhase] = useState<"idle"|"loading"|"playing">("idle");
 const [previewError,setPreviewError] = useState("");
 const preview = useRef<{controller:AbortController;id:string;playback:VoicePlayback}|null>(null);
 const [phase,setPhase] = useState<Phase>("idle");
 const [mode,setMode]=useState<"hold"|"continuous">("hold");
 const [continuousRunning,setContinuousRunning]=useState(false);
 const [loading,setLoading] = useState(false);
 const [error,setError] = useState("");
 const [recognized,setRecognized] = useState("");
 const [notice,setNotice] = useState("");
 const [retrySend,setRetrySend] = useState(false);
 const operation = useRef<CallOperation | null>(null);
 const playback = useRef<VoicePlayback | null>(null);
 const pointer = useRef<number | null>(null);
 const holding = useRef(false);
 const current = useRef({owner,sequence,open,hardBlocked,canSend,onSend,onEnd,onActiveChange});
 current.current = {owner,sequence,open,hardBlocked,canSend,onSend,onEnd,onActiveChange};
 const valid = (op: CallOperation) => operation.current === op && !op.controller.signal.aborted && current.current.open && current.current.owner === owner && !current.current.hardBlocked;
 const stopPreview = useCallback(() => {
  const pending=preview.current;preview.current=null;
  if(pending){pending.controller.abort();pending.playback.close();void api.cancel(pending.id).catch(()=>{});}
  setPreviewPhase("idle");
 },[api]);
 async function previewVoice(nextVoice:string) {
  stopPreview();setPreviewError("");
  if(!current.current.open||current.current.hardBlocked||loading||!speaker||operation.current||continuousRunning)return;
  let pending:NonNullable<typeof preview.current>|undefined;
  try {
   // Resume audio directly in the selection/click gesture, before requesting TTS.
   pending={controller:new AbortController(),id:crypto.randomUUID(),playback:api.playback()};
   preview.current=pending;setPreviewPhase("loading");
   const audio=await api.synthesize(pending.id,speaker,"你好，很高兴与你交流。这是我的声音，你喜欢吗？",nextVoice);
   if(preview.current!==pending||pending.controller.signal.aborted)return;
   setPreviewPhase("playing");await pending.playback.play(audio,pending.controller.signal);
  }catch(cause){if(!pending||preview.current===pending)setPreviewError(tauriMessageFrom(cause));}
  finally{if(pending&&preview.current===pending){preview.current=null;pending.playback.close();setPreviewPhase("idle");}}
 }
 const cancel = useCallback(() => {
  stopPreview();
  const op=operation.current; operation.current=null; pointer.current=null; holding.current=false;
  if(op) { op.controller.abort(); op.capture?.cancel(); for(const id of op.requests) void api.cancel(id).catch(()=>{}); }
  playback.current?.stop();
 },[api,stopPreview]);
 const end = () => { cancel(); playback.current?.close(); playback.current=null; setPhase("idle"); current.current.onEnd(); };
 const stop = () => { cancel(); setPhase("idle"); setNotice("本次录音或播报已停止，可以继续按住说话。"); };
 const nativeID = (op: CallOperation) => { const id=crypto.randomUUID(); op.requests.add(id); return id; };
 async function speak(op: CallOperation,reply: VoiceReply) {
  if(!valid(op)) return;
  if(reply.error) { setError(reply.error); operation.current=null; setPhase("idle"); return; }
  const {chunks,truncated}=spokenReply(reply.text);
  if(!chunks.length) { setNotice("本轮没有可播报的文字，请查看聊天记录。"); operation.current=null; setPhase("idle"); return; }
  if(truncated) setNotice("回答较长，语音播放前 4000 字；完整回答保留在聊天记录中。");
  try {
   for(const text of chunks) {
    if(!valid(op)) return;
    setPhase("synthesizing");
    const audio=await api.synthesize(nativeID(op),op.ttsProvider,text,op.voice);
    if(!valid(op)) return;
    setPhase("speaking");
    if(!playback.current) throw new Error("播放设备已关闭；请重新按住说话。");
    await playback.current.play(audio,op.controller.signal);
   }
  } catch(cause) { if(valid(op)) setError(tauriMessageFrom(cause)); }
  finally { if(valid(op)) { operation.current=null; setPhase("idle"); } }
 }
 // The parent forwards only events already admitted by its session/stream fence.
 const sink: VoiceEventSink = event => {
  const op=operation.current;
  if(!op?.tracker || !valid(op)) return;
  const reply=op.tracker.accept(event);
  if(!reply) return;
  if(op.admitted) void speak(op,reply); else op.reply=reply;
 };
 const sinkRef=useRef(sink); sinkRef.current=sink;
 useEffect(()=>{
  if(!open || mode!=="hold") return;
  const handler: VoiceEventSink = event=>sinkRef.current(event);
  eventSink.current=handler;
  return ()=>{if(eventSink.current===handler) eventSink.current=null;};
 },[open,owner,eventSink,mode]);
 useEffect(()=>{
  if(!open) return;
  current.current.onActiveChange(true);
  let active=true; setLoading(true); setError(""); setPreviewError(""); setRecognized(""); setNotice(""); setPhase("idle");
  void api.providers().then(entries=>{
   if(!active) return;
   setProviders(entries);
   const preferred=entries.find(entry=>entry.preferred)??entries[0];
   setRecognizer(preferred?.name??"");
   const speakers=entries.filter(entry=>!entry.name.startsWith("custom:"));
   setSpeaker(speakers.some(entry=>entry.name===preferred?.name)?preferred!.name:speakers[0]?.name??"");
  }).catch(cause=>{if(active) setError(tauriMessageFrom(cause));}).finally(()=>{if(active) setLoading(false);});
  return ()=>{active=false; cancel(); playback.current?.close(); playback.current=null; current.current.onActiveChange(false);};
 },[open,owner,api,cancel]);
 useEffect(()=>{
  if(!open) return;
  if(hardBlocked) { cancel(); setPhase("idle"); setError("当前对话连接或操作状态已变化；请恢复对话后重新开始。"); }
 },[open,hardBlocked,cancel]);
 useEffect(()=>{
  if(!open) return;
  const abort=()=>{cancel();setPhase("idle");setNotice("录音与播报已停止，可以重新按住说话。");};
  const hidden=()=>{if(document.hidden) abort();};
  const blur=()=>{if(holding.current) abort();};
  const key=(event:KeyboardEvent)=>{if(event.key==="Escape") {event.stopPropagation();abort();current.current.onEnd();}};
  document.addEventListener("visibilitychange",hidden); document.addEventListener("keydown",key,true); window.addEventListener("blur",blur);
  return ()=>{document.removeEventListener("visibilitychange",hidden);document.removeEventListener("keydown",key,true);window.removeEventListener("blur",blur);};
 },[open,cancel]);
 async function sendRecognized(op: CallOperation,text: string) {
  if(!valid(op)) return;
  if(!current.current.canSend) { setError("当前对话暂不能发送；请处理草稿、附件或等待本轮结束，再重新发送识别文字。"); operation.current=null;setPhase("idle");return; }
  setRetrySend(false); op.tracker=new VoiceReplyTracker(owner,current.current.sequence); setPhase("waiting");
  try {
   const admitted=await current.current.onSend(text);
   if(!valid(op)) return;
   if(!admitted) { setRetrySend(true); throw new Error("语音消息未确认发送成功；请查看对话状态，确认后再重新发送。"); }
   op.admitted=true;
   if(op.reply) void speak(op,op.reply);
  } catch(cause) { if(valid(op)) {setError(tauriMessageFrom(cause));operation.current=null;setPhase("idle");} }
 }
 async function finish(op: CallOperation) {
  if(!valid(op) || op.finishing) return;
  op.released=true;
  if(!op.capture) { cancel(); setPhase("idle"); setNotice("录音尚未就绪，已取消；允许麦克风后请再次按住说话。"); return; }
  op.finishing=true;setPhase("recognizing");
  try {
   const audio=await op.capture.stop(); if(!valid(op)) return;
   const text=await api.transcribe(nativeID(op),recognizer,audio); if(!valid(op)) return;
   setRecognized(text);
   await sendRecognized(op,text);
  } catch(cause) {if(valid(op)) {setError(tauriMessageFrom(cause));operation.current=null;setPhase("idle");}}
 }
 async function begin() {
  if(!open || hardBlocked || !canSend || loading || !recognizer || !speaker || operation.current) return;
  stopPreview();
  setError("");setNotice("");setRecognized("");setRetrySend(false);
  const op: CallOperation={controller:new AbortController(),requests:new Set(),ttsProvider:speaker,voice}; operation.current=op;holding.current=true;setPhase("preparing");
  try {
   playback.current??=api.playback();
   const capture=await api.capture(op.controller.signal,()=>{holding.current=false;setNotice("录音已达到时长上限，正在识别并发送。");void finish(op);});
   if(!valid(op)) {capture.cancel();return;}
   op.capture=capture;
   if(op.released) void finish(op); else setPhase("recording");
  } catch(cause) {if(valid(op)) {cancel();setPhase("idle");setError(tauriMessageFrom(cause));}}
 }
 const release=()=>{pointer.current=null;holding.current=false;const op=operation.current;if(op) void finish(op);};
 const active=phase!=="idle"||continuousRunning;
 const speakers=providers.filter(entry=>!entry.name.startsWith("custom:"));
 const ready=!loading && !active && canSend && !hardBlocked && !!recognizer && !!speaker;
 const status=loading?"正在读取语音渠道…":phase==="preparing"?"正在请求麦克风，松开后发送…":phase==="recording"?"正在录音，松开发送…":phase==="recognizing"?"正在识别语音…":phase==="waiting"?(paused?"等待你的确认，请在聊天区处理…":phaseLabel):phase==="synthesizing"?"正在合成回答声音…":phase==="speaking"?"正在播报回答…":"按住说话，松开后自动发送";
 return <div className="tauri-voice-input tauri-voice-call">
  <button type="button" className="tauri-voice-button" aria-label="开始语音对话" title="语音对话：按住说话，回答自动播放" aria-expanded={open} disabled={hardBlocked && !open} onClick={()=>{if(open) end();else onOpen();}}><Headphones size={17}/></button>
  {open && <section className="tauri-voice-panel tauri-voice-call-panel" aria-label="语音对话">
   <div className="tauri-voice-heading"><strong>语音对话</strong><button type="button" onClick={end}><PhoneOff size={14}/>结束对话</button></div>
   <p>录音和回答文本会发送到所选语音服务，并使用其额度。每次录音最长 90 秒。</p>
   <label className="tauri-voice-field"><span>交流方式</span><select aria-label="语音对话交流方式" value={mode} disabled={active} onChange={event=>{cancel();setMode(event.target.value as "hold"|"continuous");setError("");}}><option value="hold">按住说话</option><option value="continuous">连续对话</option></select></label>
   <label className="tauri-voice-field"><span>识别渠道</span><select aria-label="对话语音识别渠道" value={recognizer} disabled={active||loading} onChange={event=>setRecognizer(event.target.value)}>{providers.map(entry=><option key={entry.name} value={entry.name}>{entry.label}</option>)}</select></label>
   <label className="tauri-voice-field"><span>播报渠道</span><select aria-label="对话语音播报渠道" value={speaker} disabled={active||loading} onChange={event=>{stopPreview();setPreviewError("");setSpeaker(event.target.value);}}>{speakers.map(entry=><option key={entry.name} value={entry.name}>{entry.label}</option>)}</select></label>
   <div className="tauri-voice-preview-row"><label className="tauri-voice-field"><span>音色</span><select aria-label="语音对话音色" value={voice} disabled={active||loading} onChange={event=>{setVoice(event.target.value);void previewVoice(event.target.value);}}>{["mimo_default","冰糖","茉莉","苏打","白桦","Mia","Chloe","Milo","Dean"].map(name=><option key={name} value={name}>{name==="mimo_default"?"MiMo 默认音色":name}</option>)}</select></label>
   {previewPhase==="idle"?<button type="button" disabled={active||loading||hardBlocked||!speaker} onClick={()=>void previewVoice(voice)}><Volume2 size={14}/>试听</button>:<button type="button" onClick={stopPreview}><Square size={14}/>停止试听</button>}</div>
   <p role="status">{previewPhase==="loading"?"正在准备音色试听…":previewPhase==="playing"?`正在试听：${voice==="mimo_default"?"MiMo 默认音色":voice}`:"切换音色会自动试听，使用所选播报渠道的额度。"}</p>
   {previewError&&<p role="alert" className="tauri-voice-error">{previewError}</p>}
   {!loading&&(!providers.length||!speakers.length)&&<p>语音对话需要识别服务及支持 MiMo TTS 的播报渠道；请先配置可用的 MiMo 渠道。</p>}
   {mode==="continuous"&&<TauriContinuousVoice owner={owner} sequence={sequence} canSend={canSend} blocked={hardBlocked||loading} paused={paused} phaseLabel={phaseLabel} recognizer={recognizer} speaker={speaker} voice={voice} eventSink={eventSink} onSend={onSend} onRunning={running=>{if(running)stopPreview();setContinuousRunning(running);}}/> }
   {mode==="hold"&&<>
   {!canSend&&!active&&<p>请先处理输入框草稿与附件，或等待当前回合结束，再按住说话。</p>}
   <p role="status">{status}</p>
   {recognized&&<p className="tauri-voice-call-recognized">你说：{recognized}</p>}
   {notice&&<p role="status">{notice}</p>}{error&&<p className="tauri-voice-error" role="alert">{error}</p>}
   <button type="button" className="tauri-voice-hold" aria-label="按住说话，松开发送" disabled={!ready&&phase!=="preparing"&&phase!=="recording"}
    onPointerDown={event=>{if(event.button!==0||!ready)return;event.preventDefault();event.currentTarget.setPointerCapture(event.pointerId);pointer.current=event.pointerId;void begin();}}
    onPointerUp={event=>{if(pointer.current===event.pointerId)release();}}
    onPointerCancel={()=>{cancel();setPhase("idle");setNotice("录音已取消，没有发送。");}}
    onLostPointerCapture={()=>{if(pointer.current!==null){cancel();setPhase("idle");}}}
    onKeyDown={event=>{if((event.key===" "||event.key==="Enter")&&!event.repeat&&ready){event.preventDefault();void begin();}}}
    onKeyUp={event=>{if(event.key===" "||event.key==="Enter"){event.preventDefault();release();}}}><Mic size={16}/>{phase==="preparing"||phase==="recording"?"松开发送":"按住说话"}</button>
   {active&&<button type="button" onClick={stop}><Square size={14}/>{phase==="speaking"||phase==="synthesizing"||phase==="waiting"?"停止播报":"取消本次录音"}</button>}
   {retrySend&&recognized&&error&&phase==="idle"&&<button type="button" disabled={!ready} onClick={()=>{const op:CallOperation={controller:new AbortController(),requests:new Set(),ttsProvider:speaker,voice};operation.current=op;try{playback.current??=api.playback();void sendRecognized(op,recognized);}catch(cause){operation.current=null;setError(tauriMessageFrom(cause));}}}>重新发送识别文字</button>}
   </>}
   <button type="button" disabled={active} onClick={()=>{end();onOpenSettings();}}>语音识别设置</button>
   <p>停止播报或结束对话会停止录音和声音；已发送的模型任务仍可在聊天区查看、审批或停止。</p>
  </section>}
 </div>;
}
