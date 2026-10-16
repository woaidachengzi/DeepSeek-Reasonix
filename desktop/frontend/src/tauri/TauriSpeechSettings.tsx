import { useEffect, useMemo, useRef, useState } from "react";
import { RefreshCw, Save } from "lucide-react";
import { tauriSpeechSettings, saveTauriSpeechSettings, tauriSpeechProviders, tauriSpeechTranscribe, tauriSpeechCancel, tauriMessageFrom, type TauriSpeechSettings as SpeechSettings, type TauriSpeechSettingsChange, type TauriSpeechProvider } from "../lib/tauriBridge";
import { SettingsSection } from "../components/SettingsForm";
import { TauriVoiceInput } from "./TauriVoiceInput";
import { startVoiceCapture } from "./voiceCapture";

const settingsAPI = { load: tauriSpeechSettings, save: saveTauriSpeechSettings, providers: tauriSpeechProviders };
const draftFromView = (view: SpeechSettings): TauriSpeechSettingsChange => ({ mode: view.mode, provider: view.provider, protocol: view.protocol, endpoint: view.endpoint, model: view.model, revision: view.revision, keyAction: "keep", apiKey: "" });

export function TauriSpeechSettings({ onOpenProviders, api = settingsAPI }: { onOpenProviders: () => void; api?: typeof settingsAPI }) {
 const [view, setView] = useState<SpeechSettings | null>(null);
 const [draft, setDraft] = useState<TauriSpeechSettingsChange | null>(null);
 const [providers, setProviders] = useState<TauriSpeechProvider[]>([]);
 const [loading, setLoading] = useState(true);
 const [busy, setBusy] = useState(false);
 const [testing, setTesting] = useState(false);
 const [error, setError] = useState("");
 const [notice, setNotice] = useState("");
 const [result, setResult] = useState("");
 const testRef = useRef<HTMLTextAreaElement>(null);
 const epoch = useRef(0);
 const saving = useRef(false);
 const load = async () => {
  const request = ++epoch.current; setLoading(true); setError(""); setNotice(""); setResult("");
  try {
   const [next, entries] = await Promise.all([api.load(), api.providers()]);
   if (request !== epoch.current) return;
   setView(next); setDraft(draftFromView(next)); setProviders(entries.filter(entry => !entry.name.startsWith("custom:")));
  } catch (cause) { if (request === epoch.current) setError(tauriMessageFrom(cause)); }
  finally { if (request === epoch.current) setLoading(false); }
 };
 useEffect(() => { void load(); return () => { epoch.current++; }; }, [api]);
 const dirty = Boolean(view && draft && JSON.stringify(draft) !== JSON.stringify(draftFromView(view)));
 const change = (patch: Partial<TauriSpeechSettingsChange>) => { setDraft(previous => previous ? { ...previous, ...patch } : previous); setNotice(""); setResult(""); };
 const save = async () => {
  if (!draft || saving.current || testing) return;
  const request = epoch.current; saving.current = true; setBusy(true); setError(""); setNotice(""); setResult("");
  try {
   const next = await api.save(draft);
   if (request !== epoch.current) return;
   setView(next); setDraft(draftFromView(next)); setNotice("语音识别设置已保存，下次录音立即生效。");
  } catch (cause) { if (request === epoch.current) setError(tauriMessageFrom(cause)); }
  finally { saving.current = false; if (request === epoch.current) setBusy(false); }
 };
 const testAPI = useMemo(() => ({ providers: async () => {
  const entries = await api.providers();
  return entries.filter(entry => view?.mode === "custom" ? entry.name === `custom:${view.revision}` : !entry.name.startsWith("custom:") && (!view?.provider || entry.name === view.provider));
 }, transcribe: tauriSpeechTranscribe, cancel: tauriSpeechCancel, capture: startVoiceCapture }), [api, view]);
 const disabled = busy || testing;
 return <div className="tauri-settings-section tauri-speech-settings">
  {loading ? <p role="status">正在读取语音识别设置…</p> : view && draft ? <>
   <SettingsSection title="识别服务" description="语音识别与聊天模型独立。保留现有 MiMo 渠道，也可连接自己部署的兼容服务。">
    <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-mode">服务方式</label><select id="speech-mode" className="tauri-settings-select" disabled={disabled} value={draft.mode} onChange={event => change({ mode: event.target.value as SpeechSettings["mode"] })}><option value="mimo">MiMo 云端渠道</option><option value="custom">自定义识别服务</option></select></div>
    {draft.mode === "mimo" ? <>
     <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-provider">默认 MiMo 渠道</label><select id="speech-provider" className="tauri-settings-select" disabled={disabled} value={draft.provider} onChange={event => change({ provider: event.target.value })}><option value="">自动选择可用候选渠道</option>{draft.provider && !providers.some(entry => entry.name === draft.provider) && <option value={draft.provider}>{draft.provider}（当前不可用）</option>}{providers.map(entry => <option key={entry.name} value={entry.name}>{entry.label}</option>)}</select></div>
     <p>使用 MiMo ASR，需配置有效的 API Key、语音识别权限及可用额度。候选渠道未经测试，不能保证语音可用。</p>
     <button type="button" className="tauri-settings-button" disabled={disabled} onClick={onOpenProviders}>管理 MiMo 渠道</button>
    </> : <>
     <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-protocol">接口格式</label><select id="speech-protocol" className="tauri-settings-select" disabled={disabled} value={draft.protocol} onChange={event => change({ protocol: event.target.value as SpeechSettings["protocol"] })}><option value="transcriptions">音频转写接口（multipart）</option><option value="mimo">MiMo 兼容接口（JSON）</option></select></div>
     <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-endpoint">完整接口地址</label><input id="speech-endpoint" className="tauri-settings-input" type="url" disabled={disabled} maxLength={2048} value={draft.endpoint} placeholder={draft.protocol === "mimo" ? "http://127.0.0.1:8000/v1/chat/completions" : "http://127.0.0.1:8000/v1/audio/transcriptions"} onChange={event => change({ endpoint: event.target.value })} /></div>
     <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-model">模型名称</label><input id="speech-model" className="tauri-settings-input" disabled={disabled} maxLength={256} value={draft.model} placeholder={draft.protocol === "mimo" ? "mimo-v2.5-asr" : "填写服务实际部署的模型名称"} onChange={event => change({ model: event.target.value })} /></div>
     <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="speech-key">API Key（可选）</label><div className="tauri-speech-secret"><input id="speech-key" className="tauri-settings-input" type="password" autoComplete="off" disabled={disabled} maxLength={4096} value={draft.apiKey} placeholder={view.keySet && draft.keyAction === "keep" ? "已保存，留空保留原密钥" : "服务不需要认证时可留空"} onChange={event => change({ apiKey: event.target.value, keyAction: event.target.value ? "replace" : "keep" })} /><button type="button" className="tauri-settings-button" disabled={disabled || (!view.keySet && draft.keyAction !== "replace")} onClick={() => change({ apiKey: "", keyAction: "clear" })}>清除密钥</button></div></div>
     <p>仅支持以上两种接口格式。填写完整接口地址，不会自动补路径；本机和局域网服务可使用 HTTP。开源模型需自行部署并提供兼容接口，Gradio 页面地址不能直接使用。</p>
     {draft.keyAction === "clear" && <p role="status">保存后将清除原密钥。</p>}
     {view.keySet && draft.endpoint !== view.endpoint && draft.keyAction === "keep" && <p role="alert">服务地址已改变；请重新输入 API Key，或清除原密钥后保存。</p>}
    </>}
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={disabled || !dirty} onClick={() => void save()}><Save size={14} />{busy ? "正在保存…" : "保存语音设置"}</button><button type="button" className="tauri-settings-button" disabled={disabled || !dirty} onClick={() => { setDraft(draftFromView(view)); setResult(""); setError(""); setNotice(""); }}>撤销修改</button></div>
   </SettingsSection>
   <SettingsSection title="测试录音与识别" description="测试会将录音发送到所选服务，云端服务可能产生费用。识别结果只显示在这里，不会发送聊天消息。">
    <p role="status">{dirty ? "请先保存设置，再测试识别。" : result ? "本次测试通过；后续可用性仍取决于服务、权限和额度。" : "尚未测试；保存配置不代表服务已验证可用。"}</p>
    <TauriVoiceInput key={view.revision} owner={`speech-test:${view.revision}`} value="" onChange={setResult} inputRef={testRef} disabled={busy || dirty} onActiveChange={setTesting} onOpenSettings={onOpenProviders} api={testAPI} buttonLabel="测试录音与识别" inline />
    {result && <textarea ref={testRef} className="tauri-settings-input" aria-label="语音测试结果" readOnly rows={3} value={result} />}
   </SettingsSection>
   {notice && <p role="status">{notice}</p>}
  </> : null}
  {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  {!loading && <button type="button" className="tauri-settings-button" disabled={disabled} onClick={() => void load()}><RefreshCw size={14} />重新加载设置</button>}
 </div>;
}
