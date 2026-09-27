import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriSkillsSettings, chooseTauriSkillSourceDirectory, installTauriSkill, planTauriSkillInstall, tauriMessageFrom, tauriSkillsSettings, type TauriSkillInstallPlan, type TauriSkillInstallRequest, type TauriSkillsChange, type TauriSkillsSettings } from "../lib/tauriBridge";

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
  const [installSource, setInstallSource] = useState("");
  const [installPlan, setInstallPlan] = useState<{ request: TauriSkillInstallRequest; detail: TauriSkillInstallPlan } | null>(null);
  const [acceptRisk, setAcceptRisk] = useState(false);
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
    setInstallPlan(null);
    setAcceptRisk(false);
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

  const chooseInstallSource = async () => {
    try {
      const path = await chooseTauriSkillSourceDirectory();
      if (path) { setInstallSource(path); setInstallPlan(null); setAcceptRisk(false); }
    } catch (err) { setError(tauriMessageFrom(err)); }
  };

  const reviewInstall = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!installSource.trim() || busyRef.current) return;
    const request: TauriSkillInstallRequest = { source: installSource.trim(), scope, workspaceRoot, planId: "", acceptRisk: false };
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    setInstallPlan(null);
    setAcceptRisk(false);
    try { setInstallPlan({ request, detail: await planTauriSkillInstall(request) }); }
    catch (err) { setError(`无法预览技能安装：${tauriMessageFrom(err)}`); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const installReviewed = async () => {
    if (!installPlan || busyRef.current) return;
    if (installPlan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await installTauriSkill({ ...installPlan.request, planId: installPlan.detail.planId, acceptRisk });
      setView(result.settings);
      setInstallPlan(null);
      if (result.status !== "failed") { setInstallSource(""); setPendingApply(true); }
      setNotice(result.status === "done" ? "技能已复制到 Preview；新会话会发现它们。" : `${result.status === "failed" ? "技能未安装" : "部分技能已安装"}；失败：${result.failedNames.join("、") || "未知技能"}。请检查来源并重新预览。`);
    } catch (err) { setError(`安装失败或来源已变化，请重新预览：${tauriMessageFrom(err)}`); }
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
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">保存范围<small>项目设置仅影响当前工作区，可覆盖全局技能设置。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="技能设置保存范围"><button type="button" role="radio" aria-checked={scope === "global"} className={`tauri-settings-radio${scope === "global" ? " is-active" : ""}`} disabled={busy} onClick={() => { setScope("global"); setInstallPlan(null); }}>全局</button><button type="button" role="radio" aria-checked={scope === "project"} className={`tauri-settings-radio${scope === "project" ? " is-active" : ""}`} disabled={busy || !workspaceRoot} onClick={() => { setScope("project"); setInstallPlan(null); }}>当前项目</button></div></div>
      {scope === "global" && (view.projectOverrides?.implicit || view.projectOverrides?.skills || view.projectOverrides?.sources) && <p role="note">当前项目覆盖了部分全局技能设置。这里的开关显示所选范围的值；切到“当前项目”可查看最终生效值。</p>}
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">允许自动调用<small>关闭后仍可明确输入 /技能名 调用。</small></span><label><input type="checkbox" role="switch" aria-label="允许自动调用技能" checked={Boolean(implicitEnabled)} disabled={busy} onChange={event => void change("implicit", event.target.checked)} /> {implicitEnabled ? "已开启" : "已关闭"}</label></div>
      <h3>安装技能</h3>
      <p>从本地技能目录、Markdown 文件或公开 GitHub 仓库复制到所选范围。预览会列出技能名称、目标目录和风险；确认前不会写入文件。</p>
      <form className="tauri-skill-install-form" onSubmit={event => void reviewInstall(event)}><input className="tauri-settings-input" aria-label="技能安装来源" value={installSource} maxLength={4096} disabled={busy} placeholder="绝对路径或 https://github.com/…" onChange={event => { setInstallSource(event.target.value); setInstallPlan(null); setAcceptRisk(false); }} /><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void chooseInstallSource()}>选择目录</button><button type="submit" className="tauri-settings-button" disabled={busy || !installSource.trim()}>预览技能安装</button></form>
      {installPlan && <section className="tauri-skill-install-plan" aria-label="技能安装预览"><h4>安装预览 · {installPlan.request.scope === "project" ? "当前项目" : "全局"}</h4><div className="tauri-skill-install-plan__list">{installPlan.detail.actions.map(action => <div key={`${action.name}-${action.target}`}><strong>{action.name}</strong><small>{action.riskLevel === "high" ? "高风险" : action.riskLevel === "medium" ? "中风险" : "低风险"}</small><code>{action.target}</code></div>)}</div>{installPlan.detail.warningCount > 0 && <div className="tauri-install-warnings"><strong>{installPlan.detail.warningCount} 项来源兼容提示</strong>{installPlan.detail.warnings.map((warning, index) => <p key={index}>{warning}</p>)}{installPlan.detail.warningCount > installPlan.detail.warnings.length && <p>其余提示未显示；请直接检查来源文件。</p>}</div>}{installPlan.detail.actions.some(action => action.riskLevel === "high") && <label><input type="checkbox" checked={acceptRisk} disabled={busy} onChange={event => setAcceptRisk(event.target.checked)} />我已了解高风险技能的来源与执行后果</label>}<div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setInstallPlan(null)}>取消</button><button type="button" className="tauri-settings-button" disabled={busy || installPlan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk} onClick={() => void installReviewed()}>安装已预览技能</button></div></section>}
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
