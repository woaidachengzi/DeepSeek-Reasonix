import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriSandboxSettings, tauriMessageFrom, tauriSandboxSettings, type TauriSandboxChange, type TauriSandboxSettings } from "../lib/tauriBridge";

export function TauriSandboxSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriSandboxSettings | null>(null);
  const [draft, setDraft] = useState<TauriSandboxChange | null>(null);
  const [path, setPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  const reload = () => {
    setLoading(true);
    setError("");
    void tauriSandboxSettings().then(next => {
      setView(next);
      setDraft({ bash: next.bash, network: next.network, workspaceRoot: next.workspaceRoot, allowWrite: next.allowWrite });
    }).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const addPath = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = path.trim();
    if (!value || !draft || draft.allowWrite.includes(value)) return;
    setDraft({ ...draft, allowWrite: [...draft.allowWrite, value] });
    setPath("");
  };

  const save = async () => {
    if (!draft || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriSandboxSettings(draft);
      setView(next);
      setDraft({ bash: next.bash, network: next.network, workspaceRoot: next.workspaceRoot, allowWrite: next.allowWrite });
      setPendingApply(true);
      setNotice("已保存到 Preview 配置；新会话会使用更新后的沙盒设置。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice("当前会话已重新载入配置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const changed = Boolean(view && draft && (view.bash !== draft.bash || view.network !== draft.network || view.workspaceRoot !== draft.workspaceRoot || JSON.stringify(view.allowWrite) !== JSON.stringify(draft.allowWrite)));
  return <div className="tauri-settings-section tauri-sandbox-settings">
    <h3>Bash 沙盒</h3>
    <p>控制命令的系统隔离边界。项目自身的配置可能覆盖这里的全局设置。</p>
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view && draft ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">隔离模式<small>{view.platform === "windows" ? "Windows 暂无 Bash 系统沙盒。" : "启用后，命令在系统沙盒内运行；关闭将取消这层隔离。"}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="Bash 隔离模式">{(["enforce", "off"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={draft.bash === mode} className={`tauri-settings-radio${draft.bash === mode ? " is-active" : ""}`} disabled={busy || view.platform === "windows"} onClick={() => setDraft({ ...draft, bash: mode })}>{mode === "enforce" ? "强制隔离" : "关闭"}</button>)}</div></div>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">允许沙盒内联网<small>允许隔离的 Bash 命令访问网络。</small></span><label><input type="checkbox" checked={draft.network} disabled={busy} onChange={event => setDraft({ ...draft, network: event.target.checked })} /> 允许</label></div>
      <h3>文件写入范围</h3>
      <p>默认写入范围是当前项目。下面可指定全局工作区根目录和额外可写目录；留空则使用当前项目。</p>
      <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-sandbox-root">工作区根目录</label><input id="tauri-sandbox-root" className="tauri-settings-input" aria-label="工作区根目录" value={draft.workspaceRoot} maxLength={4096} disabled={busy} placeholder="留空使用当前项目" onChange={event => setDraft({ ...draft, workspaceRoot: event.target.value })} /></div>
      <div className="tauri-permissions-rule-card"><h4>额外可写目录</h4><p>添加后，该目录下的文件写入工具也可修改文件。</p><div className="tauri-permissions-rule-list">{draft.allowWrite.length ? draft.allowWrite.map(item => <div className="tauri-permissions-rule" key={item}><code>{item}</code><button type="button" aria-label={`移除可写目录 ${item}`} disabled={busy} onClick={() => setDraft({ ...draft, allowWrite: draft.allowWrite.filter(value => value !== item) })}>移除</button></div>) : <span>暂无额外目录</span>}</div><form onSubmit={addPath}><input aria-label="新增可写目录" value={path} maxLength={4096} disabled={busy} placeholder="目录路径" onChange={event => setPath(event.target.value)} /><button type="submit" className="tauri-settings-button" disabled={busy || !path.trim() || draft.allowWrite.length >= 64}>添加</button></form></div>
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || !changed} onClick={() => void save()}>保存沙盒设置</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || changed || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
