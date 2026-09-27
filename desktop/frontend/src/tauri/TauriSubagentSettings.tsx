import { useEffect, useRef, useState } from "react";
import { changeTauriSubagentSettings, tauriMessageFrom, tauriSubagentSettings, type TauriSubagentChange, type TauriSubagentProfile, type TauriSubagentProfileInput, type TauriSubagentSettings } from "../lib/tauriBridge";
import { PROJECT_COLOR_OPTIONS } from "../lib/projectColors";

const effortLabel = (value: string) => value ? value : "自动 / 继承";
const emptyProfile = (): TauriSubagentProfileInput => ({ name: "", description: "", systemPrompt: "", color: "", model: "", effort: "", allowedTools: [], readOnly: false });

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
  const [editing, setEditing] = useState<{ name: string; scope: "global" | "project"; revision: string } | null>(null);
  const [creating, setCreating] = useState(false);
  const [profileScope, setProfileScope] = useState<"global" | "project">("global");
  const [profileDraft, setProfileDraft] = useState<TauriSubagentProfileInput>(emptyProfile);
  const [toolsDraft, setToolsDraft] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState("");
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

  const beginCreate = () => {
    setEditing(null);
    setCreating(true);
    setProfileScope(workspaceRoot ? "project" : "global");
    setProfileDraft(emptyProfile());
    setToolsDraft("");
    setError("");
  };

  const beginEdit = (profile: TauriSubagentProfile) => {
    if (!profile.editable || !profile.revision) return;
    setCreating(false);
    setEditing({ name: profile.name, scope: profile.scope === "project" ? "project" : "global", revision: profile.revision });
    setProfileScope(profile.scope === "project" ? "project" : "global");
    setProfileDraft({ name: profile.name, description: profile.description, systemPrompt: profile.body || "", color: profile.color || "", model: profile.model || "", effort: profile.effort || "", allowedTools: profile.allowedTools || [], readOnly: Boolean(profile.readOnly) });
    setToolsDraft((profile.allowedTools || []).join(", "));
    setError("");
  };

  const saveProfile = async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const profile = { ...profileDraft, allowedTools: toolsDraft.split(/[,\n]/).map(value => value.trim()).filter(Boolean) };
      const next = await changeTauriSubagentSettings({ workspaceRoot, action: editing ? "update_profile" : "create_profile", name: editing?.name || "", value: "", number: 0, scope: editing?.scope || profileScope, revision: editing?.revision || "", profile });
      setView(next);
      setEditing(null);
      setCreating(false);
      setPendingApply(true);
      setNotice("档案已保存；新会话会读取更新后的子智能体。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const deleteProfile = async (profile: TauriSubagentProfile) => {
    if (busyRef.current || !profile.editable || !profile.revision) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const next = await changeTauriSubagentSettings({ workspaceRoot, action: "delete_profile", name: profile.name, value: "", number: 0, scope: profile.scope === "project" ? "project" : "global", revision: profile.revision });
      setView(next);
      setDeleteConfirm("");
      setPendingApply(true);
      setNotice("档案已删除；新会话不会再发现它。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  if (loading) return <div className="tauri-settings-loading">加载中…</div>;
  if (!view) return <div className="tauri-settings-section"><p className="tauri-diagnostic-error" role="alert">{error || "无法读取子智能体设置"}</p><button type="button" className="tauri-settings-button" onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>重试读取</button></div>;

  const models = [...new Set([view.subagentModel, ...view.profiles.map(item => item.configuredModel), ...view.profiles.map(item => item.model || ""), ...view.modelRefs].filter(Boolean))].sort();
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
    <h3>可发现的子智能体</h3><p>共 {view.profiles.length} 个档案。可以创建全局或项目档案；外部技能与内置档案保持只读。</p>
    <div className="tauri-subagent-toolbar"><input className="tauri-settings-input" type="search" aria-label="搜索子智能体" placeholder="搜索子智能体" value={query} onChange={event => setQuery(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy} onClick={beginCreate}>新建档案</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>刷新</button></div>
    {(creating || editing) && <form className="tauri-subagent-editor" onSubmit={event => { event.preventDefault(); void saveProfile(); }}>
      <h4>{editing ? `编辑 ${editing.name}` : "新建子智能体档案"}</h4>
      <div className="tauri-subagent-editor-grid"><label>名称<input className="tauri-settings-input" aria-label="档案名称" value={profileDraft.name} maxLength={64} disabled={busy || Boolean(editing)} onChange={event => setProfileDraft(draft => ({ ...draft, name: event.target.value }))} /></label><label>保存范围<select className="tauri-settings-input" aria-label="档案保存范围" value={profileScope} disabled={busy || Boolean(editing)} onChange={event => setProfileScope(event.target.value as "global" | "project")}><option value="global">全局</option>{workspaceRoot && <option value="project">当前项目</option>}</select></label></div>
      <label>说明<input className="tauri-settings-input" aria-label="档案说明" value={profileDraft.description} maxLength={512} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, description: event.target.value }))} /></label>
      <label>系统提示词<textarea className="tauri-settings-input" aria-label="档案系统提示词" value={profileDraft.systemPrompt} maxLength={65536} disabled={busy} rows={8} onChange={event => setProfileDraft(draft => ({ ...draft, systemPrompt: event.target.value }))} /></label>
      <div className="tauri-subagent-editor-grid"><label>模型<select className="tauri-settings-input" aria-label="档案模型" value={profileDraft.model} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, model: event.target.value, effort: "" }))}><option value="">继承默认值</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select></label><label>推理等级<select className="tauri-settings-input" aria-label="档案推理等级" value={profileDraft.effort} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, effort: event.target.value }))}>{effortOptions(profileDraft.model || view.subagentModel || view.defaultModel, profileDraft.effort).map(value => <option key={value} value={value}>{effortLabel(value)}</option>)}</select></label></div>
      <div className="tauri-subagent-editor-grid"><label>颜色<select className="tauri-settings-input" aria-label="档案颜色" value={profileDraft.color} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, color: event.target.value }))}>{PROJECT_COLOR_OPTIONS.map(option => <option key={option.key || "default"} value={option.key}>{option.key || "默认"}</option>)}</select></label><label>允许的工具<small>留空表示所有工具；多个名称用逗号或换行分隔。</small><textarea className="tauri-settings-input" aria-label="档案允许的工具" value={toolsDraft} disabled={busy} rows={3} onChange={event => setToolsDraft(event.target.value)} /></label></div>
      <label className="tauri-subagent-editor-check"><input type="checkbox" checked={profileDraft.readOnly} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, readOnly: event.target.checked }))} />只读执行</label>
      <div className="tauri-settings-actions"><button type="submit" className="tauri-settings-button" disabled={busy || !profileDraft.name.trim() || !profileDraft.description.trim() || !profileDraft.systemPrompt.trim()}>保存档案</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { setCreating(false); setEditing(null); }}>取消</button></div>
    </form>}
    <div className="tauri-subagent-profiles">{filteredProfiles.length ? filteredProfiles.map(profile => <div className="tauri-subagent-profile" key={`${profile.scope}:${profile.name}`}><div className="tauri-subagent-profile-heading"><strong>{profile.name}</strong><small>{profile.scope} · {profile.invocation || `/${profile.name}`}</small></div>{profile.description && <p>{profile.description}</p>}<div className="tauri-subagent-profile-controls"><label>模型<select className="tauri-settings-input" aria-label={`${profile.name} 模型`} value={profile.configuredModel || ""} disabled={busy} onChange={event => void change("profile_model", event.target.value, 0, profile.name)}><option value="">继承默认值</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select></label><label>推理等级<select className="tauri-settings-input" aria-label={`${profile.name} 推理等级`} value={profile.configuredEffort || ""} disabled={busy} onChange={event => void change("profile_effort", event.target.value, 0, profile.name)}>{effortOptions(profile.configuredModel || view.subagentModel || view.defaultModel, profile.configuredEffort).map(value => <option key={value} value={value}>{value ? value : "继承默认值"}</option>)}</select></label></div>{profile.editable ? <div className="tauri-subagent-profile-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => beginEdit(profile)}>编辑档案</button>{deleteConfirm === `${profile.scope}:${profile.name}` ? <><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void deleteProfile(profile)}>确认删除</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDeleteConfirm("")}>取消</button></> : <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDeleteConfirm(`${profile.scope}:${profile.name}`)}>删除档案</button>}</div> : profile.editReason && <small className="tauri-subagent-profile-readonly">此档案由技能文件或外部来源管理，不能在这里编辑。</small>}</div>) : <p>{query ? "没有匹配的子智能体。" : "当前工作区没有可发现的子智能体。"}</p>}</div>
    {pendingApply && currentSessionState && onApplyToCurrentSession && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button></div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
