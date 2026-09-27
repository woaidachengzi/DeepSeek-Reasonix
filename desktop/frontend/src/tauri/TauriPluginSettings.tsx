import { useEffect, useRef, useState } from "react";
import { changeTauriPluginSettings, tauriMessageFrom, tauriPluginSettings, type TauriPluginItem, type TauriPluginSettings as PluginView } from "../lib/tauriBridge";

const sourceLabels: Record<TauriPluginItem["source"], string> = {
  local: "本地来源", remote: "远程来源", package: "软件包来源", unknown: "未知来源",
};

export function TauriPluginSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<PluginView | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [query, setQuery] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const request = useRef(0);
  const busyRef = useRef(false);

  const reload = () => {
    const current = ++request.current;
    setLoading(true);
    setView(null);
    setError("");
    void tauriPluginSettings().then(next => { if (request.current === current) setView(next); })
      .catch(err => { if (request.current === current) setError(tauriMessageFrom(err)); })
      .finally(() => { if (request.current === current) setLoading(false); });
  };

  useEffect(() => {
    reload();
    return () => { request.current += 1; };
  }, []);

  const setEnabled = async (item: TauriPluginItem, enabled: boolean) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriPluginSettings({ name: item.name, revision: item.revision, enabled });
      setView(next);
      setPendingApply(true);
      setNotice(`插件 ${item.name} 已${enabled ? "启用" : "停用"}；新会话会读取更新后的状态。`);
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice("当前会话已重新载入插件状态。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const filtered = view?.plugins.filter(item => `${item.name} ${item.description} ${item.source}`.toLowerCase().includes(query.trim().toLowerCase())) ?? [];
  return <div className="tauri-settings-section tauri-plugin-settings">
    <h3>已安装插件</h3>
    <p>查看 Preview 资料中的插件包及其贡献。启用或停用会保存到插件清单，新会话据此加载技能、命令、Hooks 和 MCP 服务。</p>
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view ? <>
      <div className="tauri-plugin-toolbar"><span>共 {view.plugins.length} 个插件</span><input className="tauri-settings-input" type="search" aria-label="搜索插件" placeholder="搜索插件" value={query} onChange={event => setQuery(event.target.value)} /><button className="tauri-settings-button" type="button" disabled={busy} onClick={reload}>刷新</button></div>
      <div className="tauri-plugin-list">{filtered.length ? filtered.map(item => <article className="tauri-plugin-card" key={item.name}>
        <div className="tauri-plugin-card__heading"><div><strong>{item.name}</strong><small>{item.version || "未标注版本"} · {sourceLabels[item.source] ?? item.source} · {item.manifestKind || "未知格式"}</small></div><label><input type="checkbox" role="switch" aria-label={`启用插件 ${item.name}`} checked={item.enabled} disabled={busy || item.status !== "ready" && !item.enabled} onChange={event => void setEnabled(item, event.target.checked)} />{item.enabled ? "已启用" : "已停用"}</label></div>
        {item.description && <p>{item.description}</p>}
        <small className="tauri-plugin-card__root">{item.root}</small>
        <div className="tauri-plugin-card__counts">{item.status === "ready" ? <><span>{item.skills} 技能</span><span>{item.agents} 子智能体</span><span>{item.commands} 命令</span><span>{item.hooks} Hooks</span><span>{item.mcpServers} MCP</span>{item.runtime && <span>运行时</span>}</> : <span className="tauri-plugin-card__error">{item.issue || "插件不可用"}</span>}</div>
        {item.warningCount > 0 && <small>{item.warningCount} 项兼容提示</small>}
      </article>) : <p>{query ? "没有匹配的插件。" : "Preview 资料中尚未安装插件。"}</p>}</div>
      <p className="tauri-plugin-settings__hint">安装与卸载入口仍在接入；此处只管理已登记插件的启用状态。</p>
      {pendingApply && currentSessionState && onApplyToCurrentSession && <button className="tauri-settings-button" type="button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
