// Same-origin static module: no blob scripts, network or model dependencies.
class ReasonixVoicePCM extends AudioWorkletProcessor {
 constructor(){super();this.frame=new Float32Array(Math.round(sampleRate/50));this.offset=0;}
 process(inputs){
  const channels=inputs[0];if(!channels?.length)return true;
  for(let i=0;i<channels[0].length;i++){
   let sample=0;for(const channel of channels)sample+=channel[i]||0;
   this.frame[this.offset++]=sample/channels.length;
   if(this.offset===this.frame.length){this.port.postMessage({samples:this.frame,rate:sampleRate},[this.frame.buffer]);this.frame=new Float32Array(Math.round(sampleRate/50));this.offset=0;}
  }
  return true;
 }
}
registerProcessor("reasonix-voice-pcm",ReasonixVoicePCM);
