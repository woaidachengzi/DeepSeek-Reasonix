import { tauriSpeechTranscribe, tauriSpeechSynthesize, tauriSpeechCancel, type TauriBridgeEvent } from "../lib/tauriBridge";
import { startContinuousCapture, type ContinuousCapture } from "./continuousCapture";
import { VoiceActivity, likelyPlaybackEcho } from "./voiceActivity";
import { VoiceReplyTracker, spokenSentences, createVoicePlayback, type VoiceReply, type VoicePlayback } from "./voiceConversation";

export type ContinuousPhase="off"|"connecting"|"listening"|"recording"|"recognizing"|"waiting"|"synthesizing"|"speaking"|"blocked";
export interface ContinuousState { phase:ContinuousPhase; text:string; error:string; notice:string }
interface Host {owner:string;sequence:number;canSend:boolean;blocked:boolean;send(text:string):Promise<boolean>}
export interface ContinuousOptions {recognizer:string;speaker:string;voice:string;bargeIn:boolean;sensitivity:number}
export const continuousAPI={capture:startContinuousCapture,transcribe:tauriSpeechTranscribe,synthesize:tauriSpeechSynthesize,cancel:tauriSpeechCancel,playback:createVoicePlayback};
type Turn={controller:AbortController;ids:Set<string>;tracker?:VoiceReplyTracker;admitted:boolean;reply?:VoiceReply;done:boolean;suppressed:boolean;startedDuringPlayback:boolean;referenceText:string};
const initialState:ContinuousState={phase:"off",text:"",error:"",notice:""};
/** One continuous session owns a mic, one model turn, and a bounded speech queue. */
export class ContinuousVoice {
 private active=false;
 private controller=new AbortController();
 private capture:ContinuousCapture|null=null;
 private playback:VoicePlayback|null=null;
 private turn:Turn|null=null;
 private detector:VoiceActivity;
 private state={...initialState};
 private owner:string;
 private lastSpoken="";
 private lastOutput=false;
 private generation=0;
 constructor(private options:ContinuousOptions,private host:()=>Host,private changed:(state:ContinuousState)=>void,private api=continuousAPI){
  this.owner=host().owner;
  this.detector=new VoiceActivity({start:()=>this.speechStarted(),end:(audio,limited)=>void this.recognize(audio,limited),discard:()=>{this.cancelTurn();this.patch({notice:"声音片段过短或为持续噪音，未发送，可以继续说话。"});this.ready();}},options.sensitivity);
 }
 private valid(turn?:Turn){const host=this.host();return this.active&&!this.controller.signal.aborted&&host.owner===this.owner&&!host.blocked&&(!turn||this.turn===turn&&!turn.controller.signal.aborted);}
 private patch(patch:Partial<ContinuousState>){this.state={...this.state,...patch};this.changed({...this.state});}
 private cancelTurn(){const turn=this.turn;this.turn=null;if(turn){turn.controller.abort();for(const id of turn.ids)void this.api.cancel(id).catch(()=>{});}this.playback?.stop();}
 private request(turn:Turn){const id=crypto.randomUUID();turn.ids.add(id);return id;}
 private newTurn(duringPlayback:boolean):Turn{return {controller:new AbortController(),ids:new Set(),admitted:false,done:false,suppressed:false,startedDuringPlayback:duringPlayback,referenceText:this.lastSpoken};}
 private ready(){if(this.valid())this.patch({phase:this.host().canSend?"listening":"blocked"});}
 update(){
  if(!this.active)return;
  if(this.host().blocked||this.host().owner!==this.owner){this.pause("对话连接或会话已变化，连续聆听已暂停。");return;}
  if(this.state.phase==="blocked"&&this.host().canSend)this.ready();
  if((this.state.phase==="listening"||this.state.phase==="recording")&&!this.host().canSend){this.cancelTurn();this.detector.reset();this.patch({phase:"blocked"});}
 }
 async start(){
  if(this.active||this.host().blocked||!this.host().canSend)return;
  this.active=true;this.controller=new AbortController();const generation=++this.generation;
  this.patch({phase:"connecting",text:"",notice:"",error:""});
  try{
   this.playback=this.api.playback();
   const capture=await this.api.capture(this.controller.signal,frame=>this.frame(frame),error=>{if(this.valid())this.fail(error);});
   if(!this.valid()||this.generation!==generation){capture.close();return;}
   this.capture=capture;
   if(!capture.echoCancellation&&this.options.bargeIn){this.options={...this.options,bargeIn:false};this.patch({notice:"当前设备未确认开启系统回声消除，已关闭说话打断；仍可手动停止播报。"});}
   this.ready();
  }catch(cause){if(this.valid()&&this.generation===generation)this.fail(cause);}
 }
 pause(notice="连续聆听已暂停，麦克风已关闭。"){
  this.active=false;this.generation++;this.controller.abort();this.cancelTurn();this.capture?.close();this.capture=null;this.playback?.close();this.playback=null;this.detector.reset();this.patch({phase:"off",notice});
 }
 private fail(cause:unknown){
  const denied=cause instanceof Error&&(cause.name==="NotAllowedError"||cause.name==="PermissionDeniedError");
  const error=denied?"麦克风权限未开启；请在系统设置中允许 Reasonix 使用麦克风后重试。":cause instanceof Error?cause.message:"连续语音失败；请检查语音服务后重新开始。";
  this.pause("连续对话已暂停，文字记录仍可查看。");this.patch({error});
 }
 private frame(frame:Float32Array){
  if(!this.valid())return;
  const output=this.state.phase==="speaking"||this.state.phase==="synthesizing";
  const allow=(this.state.phase==="listening"||this.state.phase==="recording"||output&&this.options.bargeIn)&&this.host().canSend;
  const reference=this.playback?.reference?.()??null;
  this.lastOutput=output||!!reference;
  this.detector.push(frame,allow,reference);
 }
 private speechStarted(){
  if(!this.valid())return;
  const interrupt=this.state.phase==="speaking"||this.state.phase==="synthesizing";
  this.cancelTurn();this.turn=this.newTurn(this.lastOutput);
  this.patch({phase:"recording",error:"",notice:interrupt?"已停止播报，正在听你说话…":"检测到说话，停顿后自动发送。"});
 }
 private async recognize(audio:string,limited:boolean){
  const turn=this.turn;if(!turn||!this.valid(turn))return;
  this.patch({phase:"recognizing",notice:limited?"本次已达到录音时长上限，正在识别。":""});
  try{
   const text=(await this.api.transcribe(this.request(turn),this.options.recognizer,audio)).trim();if(!this.valid(turn))return;
   if(!text){this.cancelTurn();this.ready();return;}
   if(turn.startedDuringPlayback&&likelyPlaybackEcho(text,turn.referenceText)){
    this.cancelTurn();this.patch({notice:"检测到与播报重复的内容，未发送；请重新说话。"});this.ready();return;
   }
   this.patch({text});
   if(!this.host().canSend)throw new Error("当前对话暂不能发送，识别文字已保留；请处理草稿或等待当前回合结束后重新开始。");
   turn.tracker=new VoiceReplyTracker(this.owner,this.host().sequence);this.patch({phase:"waiting"});
   const admitted=await this.host().send(text);if(!this.valid(turn))return;
   if(!admitted)throw new Error("语音消息未确认发送成功；请查看聊天记录后再继续，避免重复发送。");
   turn.admitted=true;
   if(turn.reply)void this.speak(turn,turn.reply);
  }catch(cause){if(this.valid(turn))this.fail(cause);}
 }
 accept(event:TauriBridgeEvent){
  const turn=this.turn;if(!turn?.tracker||!this.valid(turn))return;
  const reply=turn.tracker.accept(event);if(!reply)return;
  turn.done=true;
  if(turn.admitted)void this.speak(turn,reply);else turn.reply=reply;
 }
 private async speak(turn:Turn,reply:VoiceReply){
  if(!this.valid(turn))return;
  if(reply.error){this.fail(new Error(reply.error));return;}
  if(turn.suppressed){this.cancelTurn();this.ready();return;}
  const {chunks,truncated}=spokenSentences(reply.text);
  if(!chunks.length){this.cancelTurn();this.patch({notice:"本轮没有可播报的文字，请查看聊天记录。"});this.ready();return;}
  if(truncated)this.patch({notice:"回答较长，语音播放前 4000 字；完整内容保留在聊天记录。"});
  this.lastSpoken="";
  const fetch=(index:number)=>{
   if(!this.valid(turn)||index>=chunks.length)return null;
   const pending=this.api.synthesize(this.request(turn),this.options.speaker,chunks[index],this.options.voice);
   void pending.catch(()=>{});return pending;
  };
  try{
   this.patch({phase:"synthesizing"});let pending=fetch(0);
   for(let index=0;index<chunks.length;index++){
    if(!pending||!this.valid(turn))return;
    const audio=await pending;if(!this.valid(turn))return;
    pending=fetch(index+1);
    this.lastSpoken=chunks[index];
    this.patch({phase:"speaking"});
    await this.playback!.play(audio,turn.controller.signal);if(!this.valid(turn))return;
    if(pending)this.patch({phase:"synthesizing"});
   }
   if(this.valid(turn)){this.turn=null;this.detector.reset();this.ready();}
  }catch(cause){if(this.valid(turn))this.fail(cause);}
 }
 stopPlayback(){
  const turn=this.turn;if(!this.valid()||!turn)return;
  if(!turn.done){turn.suppressed=true;this.patch({notice:"本轮将不再播报，等待模型完成后继续聆听。"});return;}
  this.cancelTurn();this.detector.reset();this.patch({notice:"播报已停止，可以继续说话。"});this.ready();
 }
}
