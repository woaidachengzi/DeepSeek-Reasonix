import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriPluginSettings, chooseTauriPluginDirectory, installTauriPlugin, planTauriPluginInstall, removeTauriPlugin, tauriMessageFrom, tauriPluginSettings, type TauriPluginInstallPlan, type TauriPluginItem, type TauriPluginSettings as PluginView } from "../lib/tauriBridge";

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
  const [source, setSource] = useState("");
  const [plan, setPlan] = useState<{ source: string; detail: TauriPluginInstallPlan } | null>(null);
  const [acceptRisk, setAcceptRisk] = useState(false);
  const [removeConfirmation, setRemoveConfirmation] = useState("");
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

  const chooseSource = async () => {
    try {
      const path = await chooseTauriPluginDirectory();
      if (path) { setSource(path); setPlan(null); setAcceptRisk(false); }
    } catch (err) { setError(tauriMessageFrom(err)); }
  };

  const reviewSource = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const candidate = source.trim();
    if (!candidate || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    setPlan(null);
    setAcceptRisk(false);
    try {
      const detail = await planTauriPluginInstall(candidate);
      setPlan({ source: candidate, detail });
    } catch (err) { setError(`无法生成插件安装预览：${tauriMessageFrom(err)}`); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const installReviewed = async () => {
    if (!plan || busyRef.current) return;
    if (plan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await installTauriPlugin({ source: plan.source, planId: plan.detail.planId, acceptRisk });
      setView(result.settings);
      setPlan(null);
      setSource("");
      setPendingApply(true);
      setNotice(result.status === "done" ? "插件已安装；新会话会读取更新后的插件。" : `部分插件已安装；失败：${result.failedNames.join("、") || "未知插件"}。请检查清单后重新预览。`);
    } catch (err) { setError(`安装失败或来源已变化，请重新预览：${tauriMessageFrom(err)}`); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const removeInstalled = async (item: TauriPluginItem) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await removeTauriPlugin({ name: item.name, revision: item.revision });
      setView(result.settings);
      setRemoveConfirmation("");
      setPendingApply(true);
      setNotice(`插件 ${item.name} 已移除；新会话会读取更新后的插件清单。`);
    } catch (err) { setError(`移除失败，请刷新清单后重试：${tauriMessageFrom(err)}`); }
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
      <form className="tauri-plugin-install-form" onSubmit={event => void reviewSource(event)}>
        <label htmlFor="tauri-plugin-source">添加插件<small>选择本地插件目录，或填写公开 GitHub 仓库地址。先预览安装内容，再决定是否安装。</small></label>
        <div><input id="tauri-plugin-source" className="tauri-settings-input" value={source} maxLength={4096} disabled={busy} placeholder="绝对目录路径或 https://github.com/…" onChange={event => { setSource(event.target.value); setPlan(null); setAcceptRisk(false); }} /><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => void chooseSource()}>选择目录</button><button className="tauri-settings-button" type="submit" disabled={busy || !source.trim()}>预览安装</button></div>
      </form>
      {plan && <section className="tauri-plugin-plan" aria-label="插件安装预览">
        <h4>安装预览</h4>
        <p>来源：{plan.source}。以下插件将复制到 Preview 资料；确认前不会写入安装目录。</p>
        {plan.detail.actions.map(action => <div className="tauri-plugin-plan__action" key={action.name}>
          <strong>{action.name}</strong><small>{action.version || "未标注版本"} · {action.manifestKind} · {action.riskLevel === "high" ? "高风险" : action.riskLevel === "medium" ? "中风险" : "低风险"}</small>
          <span>{action.skills} 技能 · {action.agents} 子智能体 · {action.commands} 命令 · {action.hooks} Hooks · {action.mcpServers} MCP · {action.prompts} 提示模板 · {action.themes} 主题</span>
          {action.runtime && <p className="tauri-plugin-plan__risk">完整信任运行时：{action.runtimeCommand || "未标注命令"}。它可读取会话和环境，并在本机执行操作。{action.intercepts.length > 0 && ` 拦截：${action.intercepts.join("、")}。`}{action.replaces.length > 0 && ` 替换：${action.replaces.join("、")}。`}</p>}
          {!action.runtime && action.riskLevel === "high" && <p className="tauri-plugin-plan__risk">包含会话 Hooks 或 MCP 服务，启用后可在会话中运行命令或提供工具。</p>}
        </div>)}
        {plan.detail.warningCount > 0 && <small>{plan.detail.warningCount} 项兼容提示；安装前请检查插件来源。</small>}
        {plan.detail.actions.some(action => action.riskLevel === "high") && <label className="tauri-plugin-plan__ack"><input type="checkbox" checked={acceptRisk} disabled={busy} onChange={event => setAcceptRisk(event.target.checked)} />我已了解高风险插件可在本机执行命令</label>}
        <div className="tauri-plugin-plan__actions"><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setPlan(null)}>取消</button><button className="tauri-settings-button" type="button" disabled={busy || plan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk} onClick={() => void installReviewed()}>安装已预览插件</button></div>
      </section>}
      <div className="tauri-plugin-toolbar"><span>共 {view.plugins.length} 个插件</span><input className="tauri-settings-input" type="search" aria-label="搜索插件" placeholder="搜索插件" value={query} onChange={event => setQuery(event.target.value)} /><button className="tauri-settings-button" type="button" disabled={busy} onClick={reload}>刷新</button></div>
      <div className="tauri-plugin-list">{filtered.length ? filtered.map(item => <article className="tauri-plugin-card" key={item.name}>
        <div className="tauri-plugin-card__heading"><div><strong>{item.name}</strong><small>{item.version || "未标注版本"} · {sourceLabels[item.source] ?? item.source} · {item.manifestKind || "未知格式"}</small></div><label><input type="checkbox" role="switch" aria-label={`启用插件 ${item.name}`} checked={item.enabled} disabled={busy || item.status !== "ready" && !item.enabled} onChange={event => void setEnabled(item, event.target.checked)} />{item.enabled ? "已启用" : "已停用"}</label></div>
        {item.description && <p>{item.description}</p>}
        <small className="tauri-plugin-card__root">{item.root}</small>
        <div className="tauri-plugin-card__counts">{item.status === "ready" ? <><span>{item.skills} 技能</span><span>{item.agents} 子智能体</span><span>{item.commands} 命令</span><span>{item.hooks} Hooks</span><span>{item.mcpServers} MCP</span>{item.runtime && <span>运行时</span>}</> : <span className="tauri-plugin-card__error">{item.issue || "插件不可用"}</span>}</div>
        {item.warningCount > 0 && <small>{item.warningCount} 项兼容提示</small>}
        <div className="tauri-plugin-card__actions">{removeConfirmation === item.name ? <><small>将移除登记及 Preview 管理的复制目录；外部来源文件保留。</small><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setRemoveConfirmation("")}>取消</button><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => void removeInstalled(item)}>确认移除</button></> : <button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setRemoveConfirmation(item.name)}>移除</button>}</div>
      </article>) : <p>{query ? "没有匹配的插件。" : "Preview 资料中尚未安装插件。"}</p>}</div>
      {pendingApply && currentSessionState && onApplyToCurrentSession && <button className="tauri-settings-button" type="button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
