import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy, RefreshCw } from "lucide-react";
import { tauriMessageFrom, tauriStorageSettings, type TauriStorageSettings as StorageView } from "../lib/tauriBridge";

export function TauriStorageSettings({ workspaceRoot }: { workspaceRoot?: string }) {
  const [view, setView] = useState<StorageView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const requestRef = useRef(0);
  const load = useCallback(async () => {
    const request = ++requestRef.current;
    setLoading(true);
    setError("");
    try {
      const next = await tauriStorageSettings();
      if (request === requestRef.current) setView(next);
    } catch (cause) {
      if (request === requestRef.current) { setView(null); setError(tauriMessageFrom(cause)); }
    } finally {
      if (request === requestRef.current) setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); return () => { requestRef.current += 1; }; }, [load]);

  const copy = async (label: string, path: string) => {
    setCopied("");
    setError("");
    try { await navigator.clipboard.writeText(path); setCopied(label); }
    catch { setError(`无法复制“${label}”，请手动选择路径。`); }
  };
  const paths = view ? [
    { label: "当前工作区", value: workspaceRoot ?? "" },
    { label: "Preview 配置目录", value: view.profilePath },
    { label: "状态目录", value: view.statePath },
    { label: "缓存目录", value: view.cachePath },
    { label: "扩展目录", value: view.extensionsPath },
  ] : [];

  return <section className="tauri-settings-section tauri-storage-settings" aria-label="存储位置">
    <div className="tauri-settings-data__heading"><div><h3>存储位置</h3><p>路径由正在运行的 Preview 核心报告，便于查看和复制。</p></div><button type="button" className="tauri-settings-button" disabled={loading} onClick={() => void load()}><RefreshCw size={13} />刷新</button></div>
    {loading && <div className="tauri-settings-loading">加载中…</div>}
    {view && <div className="tauri-storage-paths">{paths.map(({ label, value }) => <div className="tauri-storage-path" key={label}><span>{label}</span><div><input aria-label={label} value={value} readOnly placeholder="未设置" /><button type="button" aria-label={`复制${label}`} title={copied === label ? "已复制" : "复制路径"} disabled={!value} onClick={() => void copy(label, value)}>{copied === label ? <Check size={15} /> : <Copy size={15} />}</button></div></div>)}</div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </section>;
}
