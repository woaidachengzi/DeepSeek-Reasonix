import { encodeSpeechWav } from "./voiceCapture";

export function mono16k(channels: readonly Float32Array[], rate: number): Float32Array {
 if(!channels.length || !Number.isFinite(rate) || rate<16000 || rate>192000) throw new Error("音频采样格式无效；请重新连接麦克风。");
 const length=Math.floor(channels[0].length*16000/rate), output=new Float32Array(length);
 for(let i=0;i<length;i++) {
  const start=Math.floor(i*rate/16000),end=Math.max(start+1,Math.floor((i+1)*rate/16000));let sum=0;
  for(const channel of channels) for(let j=start;j<end;j++) sum+=Number.isFinite(channel[j])?channel[j]:0;
  output[i]=Math.max(-1,Math.min(1,sum/(channels.length*(end-start))));
 }
 return output;
}
export function rms(samples: Float32Array): number { let sum=0;for(const x of samples)sum+=x*x;return Math.sqrt(sum/Math.max(1,samples.length)); }
/** Remove correlated playback from a 20 ms near-end frame, including echo tails. */
export function removePlaybackEcho(frame: Float32Array, reference: Float32Array | null): Float32Array {
 if(!reference || reference.length<frame.length || rms(frame)<0.0001) return frame;
 let best=0,offset=0;
 let energy=0;for(let i=0;i<frame.length;i+=8)energy+=frame[i]*frame[i];
 const limit=reference.length-frame.length;
 // Search every sample delay. A coarse time grid misses short/high-frequency
 // echoes; sparse frame scoring keeps the complete delay search bounded.
 for(let start=0;start<=limit;start++){
  let dot=0,y2=0;for(let i=0;i<frame.length;i+=8){const y=reference[start+i];dot+=frame[i]*y;y2+=y*y;}
  const score=Math.abs(dot)/Math.sqrt(Math.max(1e-14,energy*y2));if(score>best){best=score;offset=start;}
 }
 if(best<0.60)return frame;
 let dot=0,x2=0,y2=0;
 for(let i=0;i<frame.length;i++){const x=frame[i],y=reference[offset+i];dot+=x*y;x2+=x*x;y2+=y*y;}
 if(Math.abs(dot)/Math.sqrt(Math.max(1e-14,x2*y2))<0.60)return frame;
 const scale=Math.max(-4,Math.min(4,dot/Math.max(1e-12,y2)));
 const residual=new Float32Array(frame.length);
 for(let i=0;i<frame.length;i++)residual[i]=frame[i]-scale*reference[offset+i];
 return residual;
}
export interface VoiceActivityCallbacks {
 start():void;
 discard?():void;
 end(audio: string, limited: boolean):void;
}
/** Sample-clock VAD: bounded pre-roll, stable onset, silence end and hard cap. */
export class VoiceActivity {
 private noise=0.0015;
 private onset=0;
 private speaking=false;
 private silence=0;
 private voiced=0;
 private duration=0;
 private minLevel=Infinity;
 private maxLevel=0;
 private preRoll:Float32Array[]=[];
 private frames:Float32Array[]=[];
 constructor(private callbacks:VoiceActivityCallbacks,private sensitivity=1){}
 reset(){this.onset=0;this.speaking=false;this.silence=0;this.voiced=0;this.duration=0;this.minLevel=Infinity;this.maxLevel=0;this.preRoll=[];this.frames=[];}
 push(input:Float32Array,allow:boolean,reference:Float32Array|null=null){
  if(!allow){this.reset();return;}
  if(!input.length||input.length>1600)return;
  const frame=removePlaybackEcho(input,reference),ms=frame.length/16,level=rms(frame);
  const threshold=Math.max(reference?0.014:0.007,this.noise*(reference?4:3))*this.sensitivity;
  const voiced=level>=threshold;
  if(!this.speaking){
   this.preRoll.push(frame.slice());while(this.preRoll.reduce((sum,f)=>sum+f.length,0)>6400)this.preRoll.shift();
   if(!voiced&&!reference&&level<0.02)this.noise=this.noise*0.98+level*0.02;
   this.onset=voiced?this.onset+ms:0;
   if(this.onset<(reference?280:180))return;
   this.speaking=true;this.voiced=this.onset;this.frames=this.preRoll;this.preRoll=[];this.duration=this.frames.reduce((sum,f)=>sum+f.length/16,0);this.silence=0;this.minLevel=level;this.maxLevel=level;
   this.callbacks.start();return;
  }
  this.frames.push(frame.slice());this.duration+=ms;
  if(voiced){this.voiced+=ms;this.silence=0;this.minLevel=Math.min(this.minLevel,level);this.maxLevel=Math.max(this.maxLevel,level);}else this.silence+=ms;
  const limited=this.duration>=88000;
  if(!limited&&this.silence<900)return;
  const frames=this.frames,steady=this.voiced>=700&&this.maxLevel-this.minLevel<this.maxLevel*0.04,valid=this.voiced>=300&&!steady;
  if(steady)this.noise=Math.max(this.noise,this.minLevel);this.reset();
  if(!valid){this.callbacks.discard?.();return;}
  const samples=new Float32Array(frames.reduce((sum,f)=>sum+f.length,0));let offset=0;
  for(const part of frames){samples.set(part,offset);offset+=part.length;}
  this.callbacks.end(encodeSpeechWav([samples],16000),limited);
 }
}
export function likelyPlaybackEcho(text:string,spoken:string): boolean {
 const clean=(value:string)=>Array.from(value.toLowerCase().replace(/[\p{P}\p{Z}\s]/gu,"")).join("");
 const input=clean(text),output=clean(spoken);
 if(input.length<6||output.length<6)return false;
 if(output.includes(input))return true;
 const grams=(s:string)=>new Set(Array.from({length:Math.max(0,s.length-2)},(_,i)=>s.slice(i,i+3)));
 const a=grams(input),b=grams(output);let common=0;for(const gram of a)if(b.has(gram))common++;
 return a.size>0&&common/a.size>=0.86;
}
