import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { Mic, Square, X } from "lucide-react";
import { tauriSpeechCancel, tauriSpeechProviders, tauriSpeechTranscribe, type TauriSpeechProvider } from "../lib/tauriBridge";
import { startVoiceCapture, type VoiceCapture } from "./voiceCapture";

type Operation = { id: string; controller: AbortController; capture?: VoiceCapture; text: string; revision: number; finishing?: boolean; start: number; end: number };
type VoiceAPI = { providers: typeof tauriSpeechProviders; transcribe: typeof tauriSpeechTranscribe; cancel: typeof tauriSpeechCancel; capture: typeof startVoiceCapture };
const defaultAPI: VoiceAPI = { providers: tauriSpeechProviders, transcribe: tauriSpeechTranscribe, cancel: tauriSpeechCancel, capture: startVoiceCapture };

export function TauriVoiceInput({ owner, value, onChange, inputRef, disabled, onActiveChange, onOpenSettings, buttonLabel = "语音输入", inline = false, api = defaultAPI }: {
  owner: string; value: string; onChange: (value: string) => void;
  inputRef: RefObject<HTMLTextAreaElement | null>; disabled: boolean;
  onActiveChange: (active: boolean) => void; onOpenSettings: () => void; buttonLabel?: string; inline?: boolean; api?: VoiceAPI;
}) {
  const [open, setOpen] = useState(false);
  const [providers, setProviders] = useState<TauriSpeechProvider[]>([]);
  const [provider, setProvider] = useState("");
  const [loading, setLoading] = useState(false);
  const [phase, setPhase] = useState<"idle" | "preparing" | "recording" | "transcribing">("idle");
  const [error, setError] = useState("");
  const [transcript, setTranscript] = useState("");
  const operation = useRef<Operation | null>(null);
  const catalogEpoch = useRef(0);
  const current = useRef({ owner, value, disabled, onChange, revision: 0 });
  const revision = current.current.revision + (current.current.owner !== owner || current.current.value !== value ? 1 : 0);
  current.current = { owner, value, disabled, onChange, revision };
  const isCurrent = (op: Operation) => operation.current === op && !op.controller.signal.aborted;
  const cancel = useCallback(() => {
    const op = operation.current;
    operation.current = null;
    if (op) { op.controller.abort(); op.capture?.cancel(); void api.cancel(op.id).catch(() => {}); }
    onActiveChange(false);
  }, [api, onActiveChange]);
  useEffect(() => {
    return () => { catalogEpoch.current++; cancel(); };
  }, [owner, cancel]);
  useEffect(() => {
    if (disabled) { cancel(); setPhase("idle"); }
  }, [disabled, cancel]);
  useEffect(() => {
    if (!open) return;
    const abortCapture = () => { if (document.hidden) { cancel(); setPhase("idle"); } };
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") { event.stopPropagation(); cancel(); setPhase("idle"); setOpen(false); } };
    document.addEventListener("visibilitychange", abortCapture);
    document.addEventListener("keydown", key, true);
    return () => { document.removeEventListener("visibilitychange", abortCapture); document.removeEventListener("keydown", key, true); };
  }, [open, cancel]);
  async function loadProviders() {
    const epoch = ++catalogEpoch.current;
    setLoading(true); setError("");
    try {
      const entries = await api.providers();
      if (catalogEpoch.current !== epoch) return;
      setProviders(entries); setProvider(previous => entries.find(entry => entry.preferred)?.name ?? (entries.some(entry => entry.name === previous) ? previous : entries[0]?.name ?? ""));
    } catch { if (catalogEpoch.current === epoch) setError("无法读取语音渠道；请重试或检查语音识别设置。"); }
    finally { if (catalogEpoch.current === epoch) setLoading(false); }
  }
  function close() { catalogEpoch.current++; cancel(); setPhase("idle"); setOpen(false); setError(""); setTranscript(""); }
  async function finish(op: Operation) {
    if (!isCurrent(op) || !op.capture || op.finishing) return;
    op.finishing = true;
    setPhase("transcribing");
    try {
      const audio = await op.capture.stop();
      if (!isCurrent(op)) return;
      const text = await api.transcribe(op.id, provider, audio);
      if (!isCurrent(op) || current.current.owner !== owner || current.current.disabled) return;
      // A transcription never overwrites typing, external inserts, or a newer
      // conversation draft. The user can explicitly append a retained result.
      if (current.current.value === op.text && current.current.revision === op.revision) {
        current.current.onChange(op.text.slice(0, op.start) + text + op.text.slice(op.end));
        setOpen(false); setTranscript("");
        inputRef.current?.focus();
      } else {
        setTranscript(text); setError("输入内容已变化，识别结果已保留；请确认后追加到输入框。");
      }
    } catch (cause) {
      if (isCurrent(op)) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (isCurrent(op)) { operation.current = null; setPhase("idle"); onActiveChange(false); }
    }
  }
  async function start() {
    if (disabled || !provider || loading || operation.current) return;
    setError(""); setTranscript("");
    const op: Operation = { id: crypto.randomUUID(), controller: new AbortController(), text: value, revision: current.current.revision, start: inputRef.current?.selectionStart ?? value.length, end: inputRef.current?.selectionEnd ?? value.length };
    operation.current = op; setPhase("preparing"); onActiveChange(true);
    try {
      const capture = await api.capture(op.controller.signal, () => { void finish(op); });
      if (!isCurrent(op)) { capture.cancel(); return; }
      op.capture = capture; setPhase("recording");
    } catch (cause) {
      if (!isCurrent(op)) return;
      cancel(); setPhase("idle");
      const denied = cause instanceof Error && (cause.name === "NotAllowedError" || cause.name === "PermissionDeniedError");
      setError(denied ? "麦克风权限未开启；请在系统隐私设置中允许 Reasonix 使用麦克风后重试。" : cause instanceof Error ? cause.message : "无法启动录音；请检查麦克风后重试。");
    }
  }
  const active = phase !== "idle";
  return <div className={`tauri-voice-input${inline ? " tauri-voice-input--inline" : ""}`}>
    <button type="button" className="tauri-voice-button" disabled={disabled} aria-label={buttonLabel} title="语音输入：录音转成文字，确认后发送" aria-expanded={open} onClick={() => { if (open) close(); else { setOpen(true); void loadProviders(); } }}><Mic size={17} aria-hidden="true" />{inline && <span>{buttonLabel}</span>}</button>
    {open && <section className="tauri-voice-panel" aria-label="语音输入设置">
      <div className="tauri-voice-heading"><strong>语音输入</strong><button type="button" onClick={close} aria-label="关闭语音输入"><X size={14} /></button></div>
      <label>识别渠道<select aria-label="语音识别渠道" value={provider} disabled={active || loading} onChange={event => setProvider(event.target.value)}>{providers.map(entry => <option key={entry.name} value={entry.name}>{entry.label}</option>)}</select></label>
      <p>{providers.find(entry => entry.name === provider)?.description}</p>
      <p>录音结束后发送到所选渠道，转成文字。最长 90 秒，确认文字后再发送消息。</p>
      {error && <p role="alert" className="tauri-voice-error">{error}</p>}
      {transcript && <><textarea aria-label="保留的语音识别结果" readOnly value={transcript} rows={3} /><button type="button" disabled={disabled} onClick={() => { onChange([value, transcript].filter(Boolean).join("\n")); setTranscript(""); setError(""); setOpen(false); inputRef.current?.focus(); }}>追加到输入框</button></>}
      {loading ? <span role="status">正在读取语音渠道…</span> : active ? <div className="tauri-voice-actions"><span role="status">{phase === "preparing" ? "正在请求麦克风…" : phase === "recording" ? "正在录音，点击完成开始识别" : "正在识别…"}</span>{phase === "recording" && <button type="button" onClick={() => { const op = operation.current; if (op) void finish(op); }}><Square size={12} />完成录音</button>}<button type="button" onClick={() => { cancel(); setPhase("idle"); }}>取消</button></div> : providers.length ? <button type="button" disabled={disabled || !provider} onClick={() => void start()}><Mic size={14} />开始录音</button> : <><p>尚无可用语音服务，请先在语音识别设置中配置 MiMo 渠道或自定义服务。</p><button type="button" onClick={() => { close(); onOpenSettings(); }}>打开语音识别设置</button></>}
      {!inline && providers.length > 0 && !active && <button type="button" onClick={() => { close(); onOpenSettings(); }}>语音识别设置</button>}
      {!active && <button type="button" disabled={loading} onClick={() => void loadProviders()}>刷新渠道</button>}
    </section>}
  </div>;
}
