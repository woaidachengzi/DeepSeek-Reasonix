/** Transient capture only: no files, localStorage or session events. */
export interface VoiceCapture { stop(): Promise<string>; cancel(): void }
export const VOICE_MAX_SECONDS = 90;

export function encodeSpeechWav(channels: readonly Float32Array[], sampleRate: number): string {
  if (!channels.length || !Number.isFinite(sampleRate) || sampleRate < 16000 || sampleRate > 192000 || channels.some(c => c.length !== channels[0].length)) throw new Error("录音格式不受支持；请重新录音。");
  const length = Math.floor(channels[0].length * 16000 / sampleRate);
  if (!length || length > 16000 * VOICE_MAX_SECONDS) throw new Error("录音为空或超过 90 秒；请缩短后重试。");
  const bytes = new Uint8Array(44 + length * 2), view = new DataView(bytes.buffer);
  const ascii = (offset: number, value: string) => [...value].forEach((char, i) => bytes[offset + i] = char.charCodeAt(0));
  ascii(0, "RIFF"); view.setUint32(4, bytes.length - 8, true); ascii(8, "WAVE"); ascii(12, "fmt ");
  view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true);
  view.setUint32(24, 16000, true); view.setUint32(28, 32000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true);
  ascii(36, "data"); view.setUint32(40, length * 2, true);
  // Average each source interval before downsampling to reduce aliasing.
  for (let i = 0; i < length; i++) {
    const start = Math.floor(i * sampleRate / 16000), end = Math.max(start + 1, Math.floor((i + 1) * sampleRate / 16000));
    let value = 0;
    for (const channel of channels) for (let j = start; j < end; j++) value += Number.isFinite(channel[j]) ? channel[j] : 0;
    value = Math.max(-1, Math.min(1, value / (channels.length * (end - start))));
    view.setInt16(44 + i * 2, Math.round(value * (value < 0 ? 32768 : 32767)), true);
  }
  let binary = "";
  for (let i = 0; i < bytes.length; i += 8192) binary += String.fromCharCode(...bytes.subarray(i, i + 8192));
  return btoa(binary);
}

/** Optional meter owns no output connection and retains no recorded samples. */
function createVoiceMeter(onLevel: (level: number) => void) {
  // Unlock before getUserMedia's first await, while the click owns activation.
  const context = new AudioContext();
  void context.resume().catch(() => {});
  let source: MediaStreamAudioSourceNode | undefined;
  let analyser: AnalyserNode | undefined;
  let frame: number | undefined;
  let closed = false, previous = 0, lastSample = -Infinity;
  const close = () => {
    if (closed) return;
    closed = true;
    if (frame !== undefined) cancelAnimationFrame(frame);
    source?.disconnect(); analyser?.disconnect();
    void context.close().catch(() => {});
    onLevel(0);
  };
  return {
    close,
    connect(stream: MediaStream) {
      if (closed) return;
      try {
        source = context.createMediaStreamSource(stream);
        analyser = context.createAnalyser(); analyser.fftSize = 256;
        source.connect(analyser);
        const samples = new Float32Array(analyser.fftSize);
        const sample = (now: number) => {
          if (closed || !analyser) return;
          if (now - lastSample >= 50) {
            lastSample = now; analyser.getFloatTimeDomainData(samples);
            const rms = Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / samples.length);
            const next = Math.min(1, Math.max(0, (rms - 0.008) * 9));
            previous = Math.max(next, previous * 0.7);
            onLevel(previous);
          }
          frame = requestAnimationFrame(sample);
        };
        frame = requestAnimationFrame(sample);
      } catch { close(); } // A meter failure must not discard a valid recording.
    },
  };
}

export async function startVoiceCapture(signal: AbortSignal, onLimit: () => void, onLevel?: (level: number) => void): Promise<VoiceCapture> {
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined" || typeof AudioContext === "undefined") throw new Error("当前环境不支持录音；请在新版桌面 App 中使用语音输入。");
  if (signal.aborted) throw new Error("录音已取消");
  const meter = onLevel ? createVoiceMeter(onLevel) : null;
  const abandonMeter = () => meter?.close();
  signal.addEventListener("abort", abandonMeter, { once: true });
  let stream: MediaStream;
  try { stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true }, video: false }); }
  catch (cause) { signal.removeEventListener("abort", abandonMeter); meter?.close(); throw cause; }
  let released = false;
  const release = () => { if (released) return; released = true; signal.removeEventListener("abort", abandonMeter); stream.getTracks().forEach(track => track.stop()); meter?.close(); };
  if (signal.aborted) { release(); throw new Error("录音已取消"); }
  let recorder: MediaRecorder;
  try { recorder = new MediaRecorder(stream); } catch { release(); throw new Error("无法启动录音；请检查麦克风是否被占用。"); }
  let chunks: Blob[] = [], size = 0, cancelled = false, stopping = false;
  let captureError: Error | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let resolveBlob!: (blob: Blob) => void, rejectBlob!: (error: Error) => void;
  const stopped = new Promise<Blob>((resolve, reject) => { resolveBlob = resolve; rejectBlob = reject; });
  // Cancellation may happen without a caller awaiting stop().
  void stopped.catch(() => {});
  const cleanup = () => { release(); clearTimeout(timer); signal.removeEventListener("abort", cancel); };
  const cancel = () => {
    cancelled = true; chunks = []; cleanup();
    if (recorder.state !== "inactive") recorder.stop();
    rejectBlob(captureError ?? new Error("录音已取消"));
  };
  recorder.ondataavailable = event => {
    if (cancelled) return;
    size += event.data.size;
    if (size > 8 * 1024 * 1024) { captureError = new Error("录音过大；请缩短后重新录制。"); cancel(); onLimit(); return; }
    chunks.push(event.data);
  };
  recorder.onerror = () => { captureError = new Error("录音中断；请重新录音。"); cancel(); onLimit(); };
  recorder.onstop = () => {
    cleanup();
    if (cancelled) return;
    resolveBlob(new Blob(chunks, { type: recorder.mimeType })); chunks = [];
  };
  signal.addEventListener("abort", cancel, { once: true });
  try { recorder.start(1000); } catch { cancel(); throw new Error("无法启动录音；请检查麦克风权限。"); }
  meter?.connect(stream);
  timer = setTimeout(onLimit, (VOICE_MAX_SECONDS - 1) * 1000);
  return {
    cancel,
    async stop() {
      if (captureError) throw captureError;
      if (stopping || cancelled || signal.aborted) throw new Error("录音已结束或取消；请重新录音。");
      stopping = true; clearTimeout(timer); meter?.close();
      if (recorder.state !== "inactive") recorder.stop();
      const blob = await stopped;
      const context = new AudioContext();
      try {
        const audio = await context.decodeAudioData(await blob.arrayBuffer());
        if (signal.aborted) throw new Error("录音已取消");
        return encodeSpeechWav(Array.from({ length: audio.numberOfChannels }, (_, i) => audio.getChannelData(i)), audio.sampleRate);
      } finally { await context.close(); }
    },
  };
}
