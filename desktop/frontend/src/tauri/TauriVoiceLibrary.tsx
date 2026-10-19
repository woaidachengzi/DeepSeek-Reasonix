import { useEffect, useRef, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { deleteTauriSpeechVoice, importTauriSpeechVoice, tauriMessageFrom, tauriSpeechVoices, type TauriSpeechVoice } from "../lib/tauriBridge";

export const builtinSpeechVoices = ["mimo_default", "冰糖", "茉莉", "苏打", "白桦", "Mia", "Chloe", "Milo", "Dean"];
export const voiceLibraryAPI = { list: tauriSpeechVoices, import: importTauriSpeechVoice, remove: deleteTauriSpeechVoice };
export type VoiceLibraryAPI = typeof voiceLibraryAPI;

export function useTauriVoiceLibrary(open: boolean, owner: string, api = voiceLibraryAPI) {
  const [voices, setVoices] = useState<TauriSpeechVoice[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const epoch = useRef(0);
  useEffect(() => {
    const revision = ++epoch.current;
    setVoices([]); setError(""); setSaving(false); setLoading(open);
    if (open) void api.list().then(result => {
      if (epoch.current === revision) setVoices(result);
    }).catch(cause => {
      if (epoch.current === revision) setError(tauriMessageFrom(cause));
    }).finally(() => { if (epoch.current === revision) setLoading(false); });
    return () => { epoch.current++; };
  }, [open, owner, api]);

  const mutate = async (operation: () => Promise<TauriSpeechVoice[]>) => {
    const revision = epoch.current;
    setSaving(true); setError("");
    try {
      const result = await operation();
      if (epoch.current !== revision) return null;
      setVoices(result); return result;
    } catch (cause) {
      if (epoch.current === revision) setError(tauriMessageFrom(cause));
      return null;
    } finally { if (epoch.current === revision) setSaving(false); }
  };
  const add = (name: string, file: File) => {
    const importEpoch = epoch.current;
    return mutate(async () => {
    const format = file.name.toLowerCase().endsWith(".mp3") ? "mp3" : file.name.toLowerCase().endsWith(".wav") ? "wav" : null;
    if (!format || file.size > 7_500_000 || !file.size) throw new Error("声音样本无效或过大；请选择不超过 7.5 MB 的 MP3 或 WAV 录音。");
    const audio = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).split(",")[1] ?? "");
      reader.onerror = () => reject(new Error("录音读取失败；请重新选择文件。"));
      reader.readAsDataURL(file);
    });
    // Closing or changing sessions while a file is being read revokes import.
    if (!open || epoch.current !== importEpoch) throw new Error("导入已取消；请重新选择文件。");
    return api.import({ name: name.trim(), format, audio });
    });
  };
  return { voices, loading, saving, error, add, remove: (id: string) => mutate(() => api.remove(id)) };
}

export function TauriVoiceLibrary({ library, selected, disabled, onSelect, onBeforeChange }: {
  library: ReturnType<typeof useTauriVoiceLibrary>; selected: string; disabled: boolean;
  onSelect: (id: string) => void; onBeforeChange: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const selectedVoice = library.voices.find(voice => voice.id === selected);
  const busy = disabled || library.loading || library.saving;
  useEffect(() => { setConfirmDelete(false); }, [selected]);
  return <div className="tauri-voice-library">
    <div className="tauri-voice-library__actions">
      <button type="button" disabled={busy} aria-expanded={expanded} onClick={() => setExpanded(value => !value)}><Plus size={14} />导入克隆音色</button>
      {selectedVoice && !confirmDelete && <button type="button" disabled={busy} onClick={() => setConfirmDelete(true)}><Trash2 size={14} />删除音色</button>}
      {selectedVoice && confirmDelete && <><span>删除“{selectedVoice.name}”？</span><button type="button" disabled={busy} onClick={async () => {
        onBeforeChange();
        if (await library.remove(selectedVoice.id)) { onSelect("mimo_default"); setConfirmDelete(false); }
      }}>确认删除</button><button type="button" disabled={busy} onClick={() => setConfirmDelete(false)}>取消</button></>}
    </div>
    {expanded && <div className="tauri-voice-library__import">
      <label className="tauri-voice-field"><span>音色名称</span><input aria-label="克隆音色名称" maxLength={40} value={name} disabled={busy} placeholder="例如：我的声音" onChange={event => setName(event.target.value)} /></label>
      <label className="tauri-voice-field"><span>声音样本</span><input ref={input} aria-label="克隆声音样本" type="file" accept=".mp3,.wav,audio/mpeg,audio/wav" disabled={busy} onChange={event => setFile(event.target.files?.[0] ?? null)} /></label>
      <button type="button" disabled={busy || !name.trim() || !file} onClick={async () => {
        if (!file) return;
        onBeforeChange();
        const voices = await library.add(name, file);
        const imported = voices?.find(voice => voice.name === name.trim());
        if (imported) { onSelect(imported.id); setExpanded(false); setName(""); setFile(null); if (input.current) input.current.value = ""; }
      }}>{library.saving ? "正在保存…" : "保存音色"}</button>
    </div>}
    <p>{library.loading ? "正在读取本机音色…" : "克隆样本保存在本机；试听及播报时会发送给所选语音渠道。"}</p>
    {library.error && <p role="alert" className="tauri-voice-error">{library.error}</p>}
  </div>;
}
