import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { createPortal } from "react-dom";
import { LoaderCircle, Mic, Square, X } from "lucide-react";
import { tauriSpeechCancel, tauriSpeechProviders, tauriSpeechTranscribe } from "../lib/tauriBridge";
import { startVoiceCapture, VOICE_MAX_SECONDS, type VoiceCapture } from "./voiceCapture";

// Defer the recording surface CSS alongside the already-loaded voice control.
if (import.meta.env?.MODE) void import("./TauriVoiceInput.css");

type Operation = { id: string; controller: AbortController; capture?: VoiceCapture; provider: Promise<string>; text: string; revision: number; finishing?: boolean; start: number; end: number };
type VoiceAPI = { providers: typeof tauriSpeechProviders; transcribe: typeof tauriSpeechTranscribe; cancel: typeof tauriSpeechCancel; capture: typeof startVoiceCapture };
const defaultAPI: VoiceAPI = { providers: tauriSpeechProviders, transcribe: tauriSpeechTranscribe, cancel: tauriSpeechCancel, capture: startVoiceCapture };
const BARS = Array.from({ length: 23 }, (_, index) => 0.28 + 0.72 * Math.sin((index + 1) / 24 * Math.PI) ** 2);

export function TauriVoiceInput({ owner, value, onChange, inputRef, disabled, onActiveChange, onOpenSettings, buttonLabel = "语音输入", inline = false, api = defaultAPI }: {
  owner: string; value: string; onChange: (value: string) => void;
  inputRef: RefObject<HTMLTextAreaElement | null>; disabled: boolean;
  onActiveChange: (active: boolean) => void; onOpenSettings: () => void; buttonLabel?: string; inline?: boolean; api?: VoiceAPI;
}) {
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<"idle" | "preparing" | "recording" | "transcribing">("idle");
  const [providerLabel, setProviderLabel] = useState("");
  const [error, setError] = useState("");
  const [transcript, setTranscript] = useState("");
  const [level, setLevel] = useState(0);
  const [elapsed, setElapsed] = useState(0);
  const startedAt = useRef(0);
  const dialogRef = useRef<HTMLElement>(null);
  const operation = useRef<Operation | null>(null);
  const current = useRef({ owner, value, disabled, onChange, onActiveChange, revision: 0 });
  const revision = current.current.revision + (current.current.owner !== owner || current.current.value !== value ? 1 : 0);
  current.current = { owner, value, disabled, onChange, onActiveChange, revision };
  const isCurrent = (op: Operation) => operation.current === op && !op.controller.signal.aborted && current.current.owner === owner && !current.current.disabled;
  const cancel = useCallback(() => {
    const op = operation.current;
    operation.current = null;
    if (op) { op.controller.abort(); op.capture?.cancel(); void api.cancel(op.id).catch(() => {}); }
    current.current.onActiveChange(false);
    setLevel(0);
  }, [api]);
  const close = useCallback(() => { cancel(); setPhase("idle"); setOpen(false); setError(""); setTranscript(""); }, [cancel]);
  useEffect(() => () => { close(); }, [owner, close]);
  useEffect(() => { if (disabled) close(); }, [disabled, close]);
  useEffect(() => {
    if (phase !== "recording") return;
    const update = () => setElapsed(Math.min(VOICE_MAX_SECONDS, Math.floor((performance.now() - startedAt.current) / 1000)));
    const timer = setInterval(update, 250); update();
    return () => clearInterval(timer);
  }, [phase]);
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement;
    const dialog = dialogRef.current;
    dialog?.focus({ preventScroll: true });
    const hidden = () => { if (document.hidden) close(); };
    const blur = () => {
      if (!operation.current?.capture || operation.current.finishing) return;
      cancel(); setPhase("idle"); setError("窗口已失去焦点，录音已取消；请重新录音。");
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); return; }
      if (event.key !== "Tab") return;
      const controls = Array.from(dialog?.querySelectorAll<HTMLElement>('button:not(:disabled),textarea:not(:disabled)') ?? []);
      const first = controls[0], last = controls[controls.length - 1], focused = document.activeElement;
      if (first && last && (focused === dialog || !dialog?.contains(focused) || (event.shiftKey ? focused === first : focused === last))) {
        event.preventDefault(); (event.shiftKey ? last : first).focus({ preventScroll: true });
      }
    };
    document.addEventListener("visibilitychange", hidden);
    document.addEventListener("keydown", key, true);
    window.addEventListener("blur", blur);
    return () => {
      document.removeEventListener("visibilitychange", hidden); document.removeEventListener("keydown", key, true); window.removeEventListener("blur", blur);
      if (previous instanceof HTMLElement && previous.isConnected && (document.activeElement === document.body || dialog?.contains(document.activeElement))) previous.focus({ preventScroll: true });
    };
  }, [open, cancel, close]);
  async function finish(op: Operation) {
    if (!isCurrent(op) || !op.capture || op.finishing) return;
    op.finishing = true;
    setPhase("transcribing"); setLevel(0);
    try {
      const audio = await op.capture.stop();
      if (!isCurrent(op)) return;
      const provider = await op.provider;
      if (!isCurrent(op)) return;
      const text = await api.transcribe(op.id, provider, audio);
      if (!isCurrent(op)) return;
      // Never overwrite typing, external inserts, or a newer conversation draft.
      if (current.current.value === op.text && current.current.revision === op.revision) {
        current.current.onChange(op.text.slice(0, op.start) + text + op.text.slice(op.end));
        setOpen(false); setTranscript(""); inputRef.current?.focus({ preventScroll: true });
      } else {
        setTranscript(text); setError("输入内容已变化，识别结果已保留；请确认后追加到输入框。");
      }
    } catch (cause) {
      if (isCurrent(op)) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (isCurrent(op)) { operation.current = null; setPhase("idle"); current.current.onActiveChange(false); }
    }
  }
  async function start() {
    if (current.current.disabled || operation.current) return;
    setOpen(true); setError(""); setTranscript(""); setProviderLabel(""); setElapsed(0); setLevel(0);
    const op: Operation = { id: crypto.randomUUID(), controller: new AbortController(), provider: Promise.resolve(""), text: value, revision: current.current.revision, start: inputRef.current?.selectionStart ?? value.length, end: inputRef.current?.selectionEnd ?? value.length };
    operation.current = op; setPhase("preparing"); current.current.onActiveChange(true);
    try {
      // Start capture in the click gesture; catalogue lookup runs independently.
      const captured = api.capture(op.controller.signal, () => { void finish(op); }, next => {
        if (isCurrent(op) && !op.finishing) setLevel(Number.isFinite(next) ? Math.max(0, Math.min(1, next)) : 0);
      }).then(capture => {
        if (!isCurrent(op)) { capture.cancel(); return; }
        op.capture = capture; startedAt.current = performance.now(); setPhase("recording");
      });
      op.provider = api.providers().then(entries => {
        if (!isCurrent(op)) throw new Error("录音已取消");
        const chosen = entries.find(entry => entry.preferred) ?? entries[0];
        if (!chosen) throw new Error("尚无可用语音服务；请先在语音识别设置中配置 MiMo 渠道或自定义服务。");
        setProviderLabel(chosen.label); return chosen.name;
      });
      await Promise.all([captured, op.provider]);
    } catch (cause) {
      if (!isCurrent(op)) return;
      cancel(); setPhase("idle");
      const denied = cause instanceof Error && (cause.name === "NotAllowedError" || cause.name === "PermissionDeniedError");
      setError(denied ? "麦克风权限未开启；请在系统隐私设置中允许 Reasonix 使用麦克风后重试。" : cause instanceof Error ? cause.message : "无法启动录音；请检查麦克风后重试。");
    }
  }
  const active = phase !== "idle";
  const label = phase === "preparing" ? "正在请求麦克风…" : phase === "recording" ? "正在录音，请说话" : phase === "transcribing" ? "正在识别…" : transcript ? "识别结果已保留" : "录音未完成";
  const timer = `${String(Math.floor(elapsed / 60)).padStart(2,"0")}:${String(elapsed % 60).padStart(2,"0")}`;
  return <div className={`tauri-voice-input${inline ? " tauri-voice-input--inline" : ""}`}>
    <button type="button" className="tauri-voice-button" disabled={disabled || open} aria-label={buttonLabel} title="语音输入：点击录音，完成后填入输入框" aria-haspopup="dialog" aria-expanded={open} onClick={() => void start()}><Mic size={17} aria-hidden="true" />{inline && <span>{buttonLabel}</span>}</button>
    {open && createPortal(<div className="tauri-dictation-overlay" onPointerDown={event => { if (event.target === event.currentTarget) close(); }}>
      <section ref={dialogRef} className="tauri-dictation-dialog" role="dialog" aria-modal="true" aria-label="语音输入" tabIndex={-1}>
        <header><strong><Mic size={17} aria-hidden="true"/>语音输入</strong><button type="button" className="tauri-dictation-close" onClick={close} aria-label="关闭语音输入"><X size={16}/></button></header>
        <div className={`tauri-dictation-wave${phase === "recording" ? " is-recording" : ""}`} aria-hidden="true" data-level={level.toFixed(3)}>
          {BARS.map((weight,index) => <span key={index} style={{ height: `${6 + (phase === "recording" ? level * weight * 62 : 0)}px` }}/>) }
        </div>
        <p className="tauri-dictation-status" role="status">{phase === "transcribing" && <LoaderCircle size={16} className="tauri-dictation-spinner" aria-hidden="true"/>}{label}</p>
        <div className="tauri-dictation-timer"><time aria-label="录音时长">{timer}</time><span>最长 90 秒</span></div>
        <p className="tauri-dictation-hint">{phase === "transcribing" ? "识别完成后填入输入框，确认文字后再发送。" : "点击完成后转成文字，填入输入框。"}</p>
        {providerLabel && <p className="tauri-dictation-provider">录音将发送至 {providerLabel} 识别</p>}
        {error && <p role="alert" className="tauri-dictation-error">{error}</p>}
        {transcript && <textarea aria-label="保留的语音识别结果" readOnly value={transcript} rows={3}/>}
        <div className="tauri-dictation-actions">
          <button type="button" onClick={close}>取消</button>
          {phase === "recording" ? <button type="button" className="is-primary" onClick={() => { const op = operation.current; if (op) void finish(op); }}><Square size={13}/>完成录音</button>
            : !active && transcript ? <button type="button" className="is-primary" disabled={disabled} onClick={() => { onChange([value, transcript].filter(Boolean).join("\n")); close(); inputRef.current?.focus({ preventScroll: true }); }}>追加到输入框</button>
            : !active && <button type="button" className="is-primary" disabled={disabled} onClick={() => void start()}><Mic size={14}/>重新录音</button>}
        </div>
        {!active && <button type="button" className="tauri-dictation-settings" onClick={() => { close(); onOpenSettings(); }}>语音识别设置</button>}
        <small className="tauri-dictation-escape">Esc 取消</small>
      </section>
    </div>, document.body)}
  </div>;
}
