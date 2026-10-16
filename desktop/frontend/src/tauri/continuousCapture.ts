import { mono16k } from "./voiceActivity";

export interface ContinuousCapture { close():void; echoCancellation:boolean }
export async function startContinuousCapture(signal:AbortSignal,onFrame:(frame:Float32Array)=>void,onError:(error:Error)=>void):Promise<ContinuousCapture> {
 if(!navigator.mediaDevices?.getUserMedia || typeof AudioWorkletNode==="undefined")throw new Error("当前环境不支持连续录音；请使用按住说话模式。");
 const context=new AudioContext();
 const resume=context.resume();void resume.catch(()=>{});
 let stream:MediaStream|undefined,node:AudioWorkletNode|undefined,source:MediaStreamAudioSourceNode|undefined,gain:GainNode|undefined,closed=false;
 const close=()=>{if(closed)return;closed=true;signal.removeEventListener("abort",close);if(node){node.port.onmessage=null;node.onprocessorerror=null;node.port.close();node.disconnect();}source?.disconnect();gain?.disconnect();stream?.getTracks().forEach(track=>{track.onended=null;track.stop();});void context.close().catch(()=>{});};
 const fail=(message:string)=>{if(!closed){close();onError(new Error(message));}};
 signal.addEventListener("abort",close,{once:true});
 try {
  if(signal.aborted)throw new Error("连续录音已取消");
  stream=await navigator.mediaDevices.getUserMedia({audio:{channelCount:1,echoCancellation:true,noiseSuppression:true,autoGainControl:true},video:false});
  if(closed||signal.aborted){stream.getTracks().forEach(track=>track.stop());throw new Error("连续录音已取消");}
  await resume;
  if(!context.audioWorklet)throw new Error("当前环境不支持连续录音；请使用按住说话模式。");
  await context.audioWorklet.addModule(new URL("voice-capture-worklet.js",document.baseURI).href);
  if(closed||signal.aborted)throw new Error("连续录音已取消");
  source=context.createMediaStreamSource(stream);node=new AudioWorkletNode(context,"reasonix-voice-pcm");gain=context.createGain();gain.gain.value=0;
  source.connect(node);node.connect(gain);gain.connect(context.destination);
  node.port.onmessage=event=>{
   if(closed||signal.aborted)return;
   const {samples,rate}=event.data??{};
   if(!(samples instanceof Float32Array)||samples.length>4000||!Number.isFinite(rate))return;
   try{onFrame(mono16k([samples],rate));}catch{fail("连续录音处理失败；请重新连接麦克风。");}
  };
  node.onprocessorerror=()=>fail("连续录音已中断；请重新开始或切换按住说话。");
  const track=stream.getAudioTracks()[0];if(!track)throw new Error("未找到麦克风；请检查系统输入设备。");
  track.onended=()=>fail("麦克风连接已断开；请重新连接后开始。");
  return {close,echoCancellation:track.getSettings().echoCancellation===true};
 }catch(error){close();throw error;}
}
