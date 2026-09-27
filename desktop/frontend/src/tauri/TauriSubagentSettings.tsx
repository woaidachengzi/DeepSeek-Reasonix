import { useEffect, useRef, useState } from "react";
import { changeTauriSubagentSettings, tauriMessageFrom, tauriSubagentSettings, type TauriSubagentChange, type TauriSubagentSettings } from "../lib/tauriBridge";

const effortLabel = (value: string) => value ? value : "自动 / 继承";

export function TauriSubagentSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriSubagentSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [query, setQuery] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    void tauriSubagentSettings(workspaceRoot).then(next => { if (active) setView(next); })
      .catch(err => { if (active) setError(tauriMessageFrom(err)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [workspaceRoot]);

  const change = async (action: TauriSubagentChange["action"], value = "", number = 0, name = "") => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriSubagentSettings({ workspaceRoot, action, value, number, name });
      setView(next);
      setPendingApply(true);
      setNotice("已保存到 Preview 配置，新会话将使用更新后的子智能体设置。");
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
        setNotice("当前会话已重新载入子智能体设置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  if (loading) return <div className="tauri-settings-loading">加载中…</div>;
  if (!view) return <div className="tauri-settings-section"><p className="tauri-diagnostic-error" role="alert">{error || "无法读取子智能体设置"}</p><button type="button" className="tauri-settings-button" onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>重试读取</button></div>;

  const models = [...new Set([view.subagentModel, ...view.profiles.map(item => item.configuredModel), ...view.modelRefs].filter(Boolean))].sort();
  const effortOptions = (model: string, current: string) => [...new Set(["", ...(view.modelEfforts?.[model] || []).filter(level => level !== "auto"), current].filter((level, index) => index === 0 || Boolean(level)))];
  const defaultEffortOptions = effortOptions(view.subagentModel || view.defaultModel, view.subagentEffort);
  const filteredProfiles = view.profiles.filter(profile => `${profile.name} ${profile.description} ${profile.scope}`.toLowerCase().includes(query.trim().toLowerCase()));
  return <div className="tauri-settings-section tauri-subagent-settings">
    <h3>运行默认值</h3><p>设置新会话启动的子智能体。项目配置可能覆盖这里的全局设置。</p>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">默认模型<small>留空时跟随主会话模型（当前为 {view.defaultModel || "未设置"}）。</small></span><select className="tauri-settings-input" aria-label="子智能体默认模型" value={view.subagentModel} disabled={busy} onChange={event => void change("model", event.target.value)}><option value="">跟随主会话</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">默认推理等级<small>只列出当前模型支持的等级。</small></span><select className="tauri-settings-input" aria-label="子智能体默认推理等级" value={view.subagentEffort || ""} disabled={busy} onChange={event => void change("effort", event.target.value)}>{defaultEffortOptions.map(value => <option key={value} value={value}>{effortLabel(value)}</option>)}</select></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">最大委派深度<small>允许子智能体再委派的层级。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="最大委派深度">{[1, 2].map(depth => <button key={depth} type="button" role="radio" aria-checked={view.maxDepth === depth} className={`tauri-settings-radio${view.maxDepth === depth ? " is-active" : ""}`} disabled={busy} onClick={() => void change("depth", "", depth)}>{depth} 层</button>)}</div></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">最大并行子智能体<small>同时运行的子智能体上限。</small></span><select className="tauri-settings-input" aria-label="最大并行子智能体" value={view.maxConcurrency} disabled={busy} onChange={event => void change("concurrency", "", Number(event.target.value))}>{Array.from({ length: 32 }, (_, index) => index + 1).map(value => <option key={value} value={value}>{value}</option>)}</select></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">并行写入上限<small>同一时间可以修改文件的子智能体数量。</small></span><select className="tauri-settings-input" aria-label="并行写入上限" value={view.maxParallelWriters} disabled={busy} onChange={event => void change("writers", "", Number(event.target.value))}>{Array.from({ length: view.maxConcurrency }, (_, index) => index + 1).map(value => <option key={value} value={value}>{value}</option>)}</select></div>
    <h3>可发现的子智能体</h3><p>共 {view.profiles.length} 个档案。可按名称覆盖模型和推理等级。配置档案文件的创建、编辑和删除仍需在文件中完成。</p>
    <div className="tauri-subagent-toolbar"><input className="tauri-settings-input" type="search" aria-label="搜索子智能体" placeholder="搜索子智能体" value={query} onChange={event => setQuery(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>刷新</button></div>
    <div className="tauri-subagent-profiles">{filteredProfiles.length ? filteredProfiles.map(profile => <div className="tauri-subagent-profile" key={profile.name}><div className="tauri-subagent-profile-heading"><strong>{profile.name}</strong><small>{profile.scope} · {profile.invocation || `/${profile.name}`}</small></div>{profile.description && <p>{profile.description}</p>}<div className="tauri-subagent-profile-controls"><label>模型<select className="tauri-settings-input" aria-label={`${profile.name} 模型`} value={profile.configuredModel || ""} disabled={busy} onChange={event => void change("profile_model", event.target.value, 0, profile.name)}><option value="">继承默认值</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select></label><label>推理等级<select className="tauri-settings-input" aria-label={`${profile.name} 推理等级`} value={profile.configuredEffort || ""} disabled={busy} onChange={event => void change("profile_effort", event.target.value, 0, profile.name)}>{effortOptions(profile.configuredModel || view.subagentModel || view.defaultModel, profile.configuredEffort).map(value => <option key={value} value={value}>{value ? value : "继承默认值"}</option>)}</select></label></div></div>) : <p>{query ? "没有匹配的子智能体。" : "当前工作区没有可发现的子智能体。"}</p>}</div>
    {pendingApply && currentSessionState && onApplyToCurrentSession && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button></div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
