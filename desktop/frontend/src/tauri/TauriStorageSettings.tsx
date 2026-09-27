import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy, RefreshCw } from "lucide-react";
import { tauriMessageFrom, tauriStorageSettings, tauriWorkspaceRootsAvailability, type TauriStorageSettings as StorageView } from "../lib/tauriBridge";

export function TauriStorageSettings({ workspaceRoot, defaultWorkspace = "", onChooseDefaultWorkspace, onClearDefaultWorkspace }: {
  workspaceRoot?: string;
  defaultWorkspace?: string;
  onChooseDefaultWorkspace?: () => Promise<string | null>;
  onClearDefaultWorkspace?: () => void;
}) {
  const [view, setView] = useState<StorageView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const [defaultWorkspaceAvailable, setDefaultWorkspaceAvailable] = useState<boolean | null>(null);
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
  useEffect(() => {
    let current = true;
    setDefaultWorkspaceAvailable(null);
    if (defaultWorkspace) void tauriWorkspaceRootsAvailability([defaultWorkspace]).then(
      result => { if (current) setDefaultWorkspaceAvailable(result[0] ?? null); },
      () => { if (current) setDefaultWorkspaceAvailable(null); },
    );
    return () => { current = false; };
  }, [defaultWorkspace]);

  const copy = async (label: string, path: string) => {
    setCopied("");
    setError("");
    try { await navigator.clipboard.writeText(path); setCopied(label); }
    catch { setError(`无法复制“${label}”，请手动选择路径。`); }
  };
  const chooseDefault = async () => {
    if (!onChooseDefaultWorkspace || actionBusy) return;
    setActionBusy(true); setError("");
    try { await onChooseDefaultWorkspace(); }
    catch (cause) { setError(tauriMessageFrom(cause)); }
    finally { setActionBusy(false); }
  };
  const clearDefault = () => {
    if (!onClearDefaultWorkspace || actionBusy) return;
    setError("");
    try { onClearDefaultWorkspace(); }
    catch (cause) { setError(tauriMessageFrom(cause)); }
  };
  const paths = view ? [
    { label: "当前工作区", value: workspaceRoot ?? "" },
    { label: "Preview 配置目录", value: view.profilePath },
    { label: "状态目录", value: view.statePath },
    { label: "缓存目录", value: view.cachePath },
    { label: "扩展目录", value: view.extensionsPath },
  ] : [];

  return <section className="tauri-settings-section tauri-storage-settings" aria-label="存储位置">
    <div className="tauri-settings-data__heading"><div><h3>存储位置</h3><p>默认工作区保存在 Preview；其余路径由正在运行的核心报告。</p></div><button type="button" className="tauri-settings-button" disabled={loading} onClick={() => void load()}><RefreshCw size={13} />刷新</button></div>
    {loading && <div className="tauri-settings-loading">加载中…</div>}
    <div className="tauri-storage-paths">
      <div className="tauri-storage-path"><span>新对话默认工作区</span><div><input aria-label="新对话默认工作区" value={defaultWorkspace} readOnly placeholder="未设置" /><button type="button" aria-label="复制新对话默认工作区" title={copied === "新对话默认工作区" ? "已复制" : "复制路径"} disabled={!defaultWorkspace} onClick={() => void copy("新对话默认工作区", defaultWorkspace)}>{copied === "新对话默认工作区" ? <Check size={15} /> : <Copy size={15} />}</button>{onChooseDefaultWorkspace && <button type="button" className="tauri-storage-path__select" disabled={actionBusy} onClick={() => void chooseDefault()}>选择</button>}{onClearDefaultWorkspace && <button type="button" className="tauri-storage-path__select" disabled={actionBusy || !defaultWorkspace} onClick={clearDefault}>清除</button>}</div></div>
      {defaultWorkspace && defaultWorkspaceAvailable === false && <p className="tauri-storage-path__warning" role="status">默认工作区文件夹当前不可用；新对话发送前请重新选择或清除。</p>}
      {paths.map(({ label, value }) => <div className="tauri-storage-path" key={label}><span>{label}</span><div><input aria-label={label} value={value} readOnly placeholder="未设置" /><button type="button" aria-label={`复制${label}`} title={copied === label ? "已复制" : "复制路径"} disabled={!value} onClick={() => void copy(label, value)}>{copied === label ? <Check size={15} /> : <Copy size={15} />}</button></div></div>)}
    </div>
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </section>;
}
