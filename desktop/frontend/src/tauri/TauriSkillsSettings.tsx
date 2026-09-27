import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriSkillsSettings, chooseTauriSkillSourceDirectory, tauriMessageFrom, tauriSkillsSettings, type TauriSkillsChange, type TauriSkillsSettings } from "../lib/tauriBridge";

const sourceStatusLabels: Record<string, string> = {
  ok: "可读取",
  missing: "目录尚未创建",
  "not-directory": "路径不是目录",
  unreadable: "目录无法读取",
};

export function TauriSkillsSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriSkillsSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [query, setQuery] = useState("");
  const [sourcePath, setSourcePath] = useState("");
  const [scope, setScope] = useState<"global" | "project">("global");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);
  const reloadRequest = useRef(0);

  const reload = () => {
    const request = ++reloadRequest.current;
    setLoading(true);
    setView(null);
    setError("");
    void tauriSkillsSettings(workspaceRoot).then(next => { if (request === reloadRequest.current) setView(next); })
      .catch(err => { if (request === reloadRequest.current) setError(tauriMessageFrom(err)); })
      .finally(() => { if (request === reloadRequest.current) setLoading(false); });
  };
  useEffect(() => {
    if (!workspaceRoot) setScope("global");
    reload();
    return () => { reloadRequest.current += 1; };
  }, [workspaceRoot]);

  const change = async (action: TauriSkillsChange["action"], enabled = false, name = "", path = "") => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriSkillsSettings({ workspaceRoot, scope, action, enabled, name, path });
      setView(next);
      setPendingApply(true);
      setNotice(`已保存到${scope === "project" ? "当前项目" : "全局"}配置；新会话会读取更新后的技能设置。`);
      if (action === "add_source") setSourcePath("");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const addSource = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (sourcePath.trim()) void change("add_source", true, "", sourcePath.trim());
  };

  const chooseSource = async () => {
    try {
      const path = await chooseTauriSkillSourceDirectory();
      if (path) setSourcePath(path);
    } catch (err) { setError(tauriMessageFrom(err)); }
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice("当前会话已重新载入技能设置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const filtered = view?.skills.filter(item => `${item.name} ${item.invocation} ${item.description} ${item.scope} ${item.sourcePath}`.toLowerCase().includes(query.trim().toLowerCase())) ?? [];
  const implicitEnabled = scope === "global" ? (view?.globalAllowImplicitInvocation ?? view?.allowImplicitInvocation) : view?.allowImplicitInvocation;
  return <div className="tauri-settings-section tauri-skills-settings">
    <h3>Agent Skills</h3>
    <p>查看当前工作区可发现的技能。选择保存范围后，开关和来源会写入对应配置；新会话读取更新后的值。</p>
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">保存范围<small>项目设置仅影响当前工作区，可覆盖全局技能设置。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="技能设置保存范围"><button type="button" role="radio" aria-checked={scope === "global"} className={`tauri-settings-radio${scope === "global" ? " is-active" : ""}`} onClick={() => setScope("global")}>全局</button><button type="button" role="radio" aria-checked={scope === "project"} className={`tauri-settings-radio${scope === "project" ? " is-active" : ""}`} disabled={!workspaceRoot} onClick={() => setScope("project")}>当前项目</button></div></div>
      {scope === "global" && (view.projectOverrides?.implicit || view.projectOverrides?.skills || view.projectOverrides?.sources) && <p role="note">当前项目覆盖了部分全局技能设置。这里的开关显示所选范围的值；切到“当前项目”可查看最终生效值。</p>}
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">允许自动调用<small>关闭后仍可明确输入 /技能名 调用。</small></span><label><input type="checkbox" role="switch" aria-label="允许自动调用技能" checked={Boolean(implicitEnabled)} disabled={busy} onChange={event => void change("implicit", event.target.checked)} /> {implicitEnabled ? "已开启" : "已关闭"}</label></div>
      <h3>技能来源</h3>
      <p>启停来源只改变发现范围，不删除目录中的文件。数量是该目录扫描到的技能文件数；同名覆盖、单项停用后，可用数可能更少。</p>
      <div className="tauri-skills-list">{view.sources.map(source => <div className="tauri-skills-item" key={source.path}><div><strong>{source.path}</strong><small>{source.scope} · {source.skillCount ?? 0} 个技能 · {sourceStatusLabels[source.status] ?? source.status}{source.configuredGlobal ? " · 全局自定义" : ""}{source.configuredProject ? " · 项目自定义" : ""}</small>{source.status !== "ok" && source.status !== "missing" && <span className="tauri-skills-warning">此来源当前无法发现技能，请检查目录或权限。</span>}</div><label><input type="checkbox" aria-label={`启用技能来源 ${source.path}`} checked={scope === "global" ? (source.globalEnabled ?? source.enabled) : source.enabled} disabled={busy} onChange={event => void change("source", event.target.checked, "", source.path)} /></label>{(scope === "global" ? (source.configuredGlobal ?? source.configured) : source.configuredProject) && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void change("remove_source", false, "", source.path)}>移除</button>}</div>)}</div>
      <form className="tauri-skills-add" onSubmit={addSource}><input className="tauri-settings-input" aria-label="新增技能来源目录" value={sourcePath} maxLength={4096} disabled={busy} placeholder="绝对目录路径" onChange={event => setSourcePath(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void chooseSource()}>选择目录</button><button type="submit" className="tauri-settings-button" disabled={busy || !sourcePath.trim()}>添加来源</button></form>
      <h3>技能列表</h3>
      <p>共 {view.skills.length} 个可发现技能。单项开关按技能名生效，同名技能会一起受影响。</p>
      <input className="tauri-settings-input tauri-skills-search" type="search" aria-label="搜索技能" value={query} placeholder="搜索技能" onChange={event => setQuery(event.target.value)} />
      <div className="tauri-skills-list">{filtered.length ? filtered.map((item, index) => <div className="tauri-skills-item" key={`${item.name}-${item.sourcePath}-${index}`}><div><strong>{item.invocation || item.name}</strong><span>{item.description}</span><small>{item.scope} · {item.runAs}{item.sourcePath && item.sourcePath !== "(builtin)" ? ` · ${item.sourcePath}` : ""}</small>{item.requires?.length > 0 && <small>声明依赖：{item.requires.join("、")}（调用时检查）</small>}</div><label><input type="checkbox" aria-label={`启用技能 ${item.name}`} checked={scope === "global" ? (item.globalEnabled ?? item.enabled) : item.enabled} disabled={busy} onChange={event => void change("skill", event.target.checked, item.name)} /></label></div>) : <p>{query ? "没有匹配的技能" : "当前工作区没有可发现的技能"}</p>}</div>
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={reload}>刷新发现结果</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
