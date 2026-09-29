import { useEffect, useRef, useState, type FormEvent } from "react";
import { archiveTauriSkill, changeTauriSkillsSettings, chooseTauriSkillSourceDirectory, installTauriSkill, planTauriSkillInstall, restoreTauriSkill, tauriCapabilityDiagnostics, tauriMessageFrom, tauriSkillsSettings, type TauriArchivedSkill, type TauriSkillInstallPlan, type TauriSkillInstallRequest, type TauriSkillItem, type TauriSkillSource, type TauriSkillsChange, type TauriSkillsSettings } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";
import type { DictKey } from "../locales/en";
import type { CapabilityDiagnosticsReport } from "../lib/types";
import { assessSkillRequirements, type SkillRequirementState } from "./tauriSkillReadiness";

const sourceStatusKeys: Record<string, DictKey> = {
  ok: "settings.skills.status.readable",
  missing: "settings.skills.status.missing",
  "not-directory": "settings.skills.status.notDirectory",
  unreadable: "settings.skills.status.unreadable",
};

const readinessKeys: Record<SkillRequirementState, DictKey> = {
  ready: "settings.skills.requirement.ready",
  missing: "settings.skills.requirement.missing",
  disabled: "settings.skills.requirement.disabled",
  failed: "settings.skills.requirement.failed",
  unknown: "settings.skills.requirement.unknown",
};

export function TauriSkillsSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [view, setView] = useState<TauriSkillsSettings | null>(null);
  const [capabilityReport, setCapabilityReport] = useState<CapabilityDiagnosticsReport | null>(null);
  const [readinessCheckFailed, setReadinessCheckFailed] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [query, setQuery] = useState("");
  const [sourcePath, setSourcePath] = useState("");
  const [installSource, setInstallSource] = useState("");
  const [installPlan, setInstallPlan] = useState<{ request: TauriSkillInstallRequest; detail: TauriSkillInstallPlan } | null>(null);
  const [acceptRisk, setAcceptRisk] = useState(false);
  const [archiveTarget, setArchiveTarget] = useState("");
  const [scope, setScope] = useState<"global" | "project">("global");
  const [expandedSources, setExpandedSources] = useState<Set<string>>(() => new Set());
  const [allSourceSkills, setAllSourceSkills] = useState<Set<string>>(() => new Set());
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);
  const reloadRequest = useRef(0);

  const reload = () => {
    const request = ++reloadRequest.current;
    setLoading(true);
    setView(null);
    setCapabilityReport(null);
    setReadinessCheckFailed(false);
    setError("");
    const readiness = workspaceRoot
      ? tauriCapabilityDiagnostics(workspaceRoot, true).then(report => ({ report, failed: false })).catch(() => ({ report: null, failed: true }))
      : Promise.resolve({ report: null, failed: false });
    void Promise.all([tauriSkillsSettings(workspaceRoot), readiness]).then(([next, dependencyStatus]) => {
      if (request !== reloadRequest.current) return;
      setView(next);
      setCapabilityReport(dependencyStatus.report);
      setReadinessCheckFailed(dependencyStatus.failed);
    })
      .catch(err => { if (request === reloadRequest.current) setError(tauriMessageFrom(err)); })
      .finally(() => { if (request === reloadRequest.current) setLoading(false); });
  };

  const refreshReadiness = async () => {
    const request = reloadRequest.current;
    if (!workspaceRoot) {
      setCapabilityReport(null);
      setReadinessCheckFailed(false);
      return;
    }
    setReadinessCheckFailed(false);
    try {
      const report = await tauriCapabilityDiagnostics(workspaceRoot, true);
      if (request === reloadRequest.current) setCapabilityReport(report);
    } catch {
      if (request === reloadRequest.current) {
        setCapabilityReport(null);
        setReadinessCheckFailed(true);
      }
    }
  };
  useEffect(() => {
    if (!workspaceRoot) setScope("global");
    setExpandedSources(new Set());
    setAllSourceSkills(new Set());
    setInstallPlan(null);
    setAcceptRisk(false);
    setArchiveTarget("");
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
      void refreshReadiness();
      setPendingApply(true);
      setNotice(t("settings.skills.saved", { scope: t(scope === "project" ? "settings.skills.scope.project" : "settings.skills.scope.global") }));
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
    catch (err) { setError(t("settings.skills.previewFailed", { reason: tauriMessageFrom(err) })); }
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
      void refreshReadiness();
      setInstallPlan(null);
      if (result.status !== "failed") { setInstallSource(""); setPendingApply(true); }
      setNotice(result.status === "done" ? t("settings.skills.installed") : t(result.status === "failed" ? "settings.skills.installFailed" : "settings.skills.installPartial", { names: result.failedNames.join(", ") || t("settings.skills.unknownSkill") }));
    } catch (err) { setError(t("settings.skills.installRequestFailed", { reason: tauriMessageFrom(err) })); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const archiveSkill = async (item: TauriSkillItem) => {
    if (!item.archiveRevision || busyRef.current || (item.scope !== "global" && item.scope !== "project")) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await archiveTauriSkill({ name: item.name, scope: item.scope, workspaceRoot, archiveId: "", revision: item.archiveRevision });
      setView(result.settings);
      void refreshReadiness();
      setArchiveTarget("");
      setPendingApply(true);
      setNotice(t("settings.skills.archived", { path: result.backupPath }));
    } catch (err) { setError(t("settings.skills.archiveFailed", { reason: tauriMessageFrom(err) })); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const restoreSkill = async (item: TauriArchivedSkill) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await restoreTauriSkill({ name: item.name, scope: item.scope, workspaceRoot, archiveId: item.archiveId, revision: item.revision });
      setView(result.settings);
      void refreshReadiness();
      setPendingApply(true);
      setNotice(t("settings.skills.restored", { name: item.name }));
    } catch (err) { setError(t("settings.skills.restoreFailed", { reason: tauriMessageFrom(err) })); }
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
        setNotice(t("settings.skills.applied"));
      } else setError(t("settings.skills.applyFailed"));
    } catch { setError(t("settings.skills.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const filtered = view?.skills.filter(item => `${item.name} ${item.invocation} ${item.description} ${item.scope} ${item.sourcePath}`.toLowerCase().includes(query.trim().toLowerCase())) ?? [];
  const skillsForSource = (source: TauriSkillSource) => view?.skills.filter(item => skillPathBelongsToSource(item.sourcePath, source.path)) ?? [];
  const archivedInScope = view?.archivedSkills?.filter(item => item.scope === scope) ?? [];
  const implicitEnabled = scope === "global" ? (view?.globalAllowImplicitInvocation ?? view?.allowImplicitInvocation) : view?.allowImplicitInvocation;
  const scopeLabel = (value: string) => t(value === "project" ? "settings.skills.scope.project" : value === "global" ? "settings.skills.scope.global" : value === "custom" ? "settings.skills.scope.custom" : "settings.skills.scope.builtin");
  const runAsLabel = (value: string) => t(value === "subagent" ? "settings.skills.runAs.subagent" : "settings.skills.runAs.inline");
  const riskLabel = (value: string) => t(value === "high" ? "settings.skills.risk.high" : value === "medium" ? "settings.skills.risk.medium" : "settings.skills.risk.low");
  return <div className="tauri-settings-section tauri-skills-settings">
    <h3>{t("settings.skills.title")}</h3>
    <p>{t("settings.skills.description")}</p>
    {loading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : view ? <>
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">{t("settings.skills.scope.label")}<small>{t("settings.skills.scope.hint")}</small></span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.skills.scope.ariaLabel")}>
          <button type="button" role="radio" aria-checked={scope === "global"} className={"tauri-settings-radio" + (scope === "global" ? " is-active" : "")} disabled={busy} onClick={() => { setScope("global"); setInstallPlan(null); setArchiveTarget(""); }}>{t("settings.skills.scope.global")}</button>
          <button type="button" role="radio" aria-checked={scope === "project"} className={"tauri-settings-radio" + (scope === "project" ? " is-active" : "")} disabled={busy || !workspaceRoot} onClick={() => { setScope("project"); setInstallPlan(null); setArchiveTarget(""); }}>{t("settings.skills.scope.project")}</button>
        </div>
      </div>
      {scope === "global" && (view.projectOverrides?.implicit || view.projectOverrides?.skills || view.projectOverrides?.sources) && <p role="note">{t("settings.skills.projectOverride")}</p>}
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">{t("settings.skills.implicit")}<small>{t("settings.skills.implicitHint")}</small></span>
        <label><input type="checkbox" role="switch" aria-label={t("settings.skills.implicitAria")} checked={Boolean(implicitEnabled)} disabled={busy} onChange={event => void change("implicit", event.target.checked)} /> {implicitEnabled ? t("settings.skills.enabled") : t("settings.skills.disabled")}</label>
      </div>
      <h3>{t("settings.skills.installTitle")}</h3>
      <p>{t("settings.skills.installDescription")}</p>
      <form className="tauri-skill-install-form" onSubmit={event => void reviewInstall(event)}>
        <input className="tauri-settings-input" aria-label={t("settings.skills.installSourceAria")} value={installSource} maxLength={4096} disabled={busy} placeholder={t("settings.skills.installSourcePlaceholder")} onChange={event => { setInstallSource(event.target.value); setInstallPlan(null); setAcceptRisk(false); }} />
        <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void chooseInstallSource()}>{t("settings.skills.chooseDirectory")}</button>
        <button type="submit" className="tauri-settings-button" disabled={busy || !installSource.trim()}>{t("settings.skills.previewInstall")}</button>
      </form>
      {installPlan && <section className="tauri-skill-install-plan" aria-label={t("settings.skills.installPreviewAria")}>
        <h4>{t("settings.skills.installPreview", { scope: scopeLabel(installPlan.request.scope) })}</h4>
        <div className="tauri-skill-install-plan__list">{installPlan.detail.actions.map(action => <div key={action.name + "-" + action.target}><strong>{action.name}</strong><small>{riskLabel(action.riskLevel)}</small><code>{action.target}</code></div>)}</div>
        {installPlan.detail.warningCount > 0 && <div className="tauri-install-warnings"><strong>{t("settings.skills.compatibilityWarnings", { count: installPlan.detail.warningCount })}</strong>{installPlan.detail.warnings.map((warning, index) => <p key={index}>{warning}</p>)}{installPlan.detail.warningCount > installPlan.detail.warnings.length && <p>{t("settings.skills.moreWarnings")}</p>}</div>}
        {installPlan.detail.actions.some(action => action.riskLevel === "high") && <label><input type="checkbox" checked={acceptRisk} disabled={busy} onChange={event => setAcceptRisk(event.target.checked)} />{t("settings.skills.acceptHighRisk")}</label>}
        <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setInstallPlan(null)}>{t("common.cancel")}</button><button type="button" className="tauri-settings-button" disabled={busy || (installPlan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk)} onClick={() => void installReviewed()}>{t("settings.skills.installReviewed")}</button></div>
      </section>}
      <h3>{t("settings.skills.sourcesTitle")}</h3>
      <p>{t("settings.skills.sourcesDescription")}</p>
      <div className="tauri-skills-list">{view.sources.map(source => {
        const sourceSkills = skillsForSource(source);
        const expanded = expandedSources.has(source.path);
        const showAll = allSourceSkills.has(source.path);
        const visibleSkills = showAll ? sourceSkills : sourceSkills.slice(0, 5);
        return <div className="tauri-skills-item tauri-skills-source-item" key={source.path}>
          <div className="tauri-skills-source-item__top">
            <div><strong>{source.path}</strong><small>{scopeLabel(source.scope)} · {t("settings.skills.count", { count: source.skillCount ?? 0 })} · {t(sourceStatusKeys[source.status] ?? "settings.skills.status.unknown", { status: source.status })}{source.configuredGlobal ? " · " + t("settings.skills.customGlobal") : ""}{source.configuredProject ? " · " + t("settings.skills.customProject") : ""}</small>{source.status !== "ok" && source.status !== "missing" && <span className="tauri-skills-warning">{t("settings.skills.sourceUnavailable")}</span>}</div>
            <div className="tauri-skills-source-item__actions"><label><input type="checkbox" aria-label={t("settings.skills.sourceEnabledAria", { path: source.path })} checked={scope === "global" ? (source.globalEnabled ?? source.enabled) : source.enabled} disabled={busy} onChange={event => void change("source", event.target.checked, "", source.path)} /></label>
              {(scope === "global" ? (source.configuredGlobal ?? source.configured) : source.configuredProject) && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void change("remove_source", false, "", source.path)}>{t("settings.skills.remove")}</button>}
            </div>
          </div>
          {sourceSkills.length > 0 && <div className="tauri-skill-source-inventory">
            <button type="button" className="tauri-skill-source-inventory__toggle" data-skill-source={source.path} aria-expanded={expanded} onClick={() => setExpandedSources(current => { const next = new Set(current); if (next.has(source.path)) next.delete(source.path); else next.add(source.path); return next; })}>{expanded ? t("caps.hideSkills") : t("caps.showSkills")} · {sourceSkills.length}</button>
            {expanded && <div className="tauri-skill-source-inventory__items">{visibleSkills.map((item, index) => <article key={`${item.name}:${item.scope}:${index}`}><strong>{item.invocation || item.name}</strong>{item.description && <span>{item.description}</span>}<small>{scopeLabel(item.scope)} · {runAsLabel(item.runAs)}</small></article>)}
              {sourceSkills.length > 5 && <button type="button" className="tauri-skill-source-inventory__more" onClick={() => setAllSourceSkills(current => { const next = new Set(current); if (next.has(source.path)) next.delete(source.path); else next.add(source.path); return next; })}>{showAll ? t("common.collapse") : t("caps.skillRootShowAllSkills", { count: sourceSkills.length })}</button>}
            </div>}
          </div>}
        </div>;
      })}</div>
      <form className="tauri-skills-add" onSubmit={addSource}>
        <input className="tauri-settings-input" aria-label={t("settings.skills.addSourceAria")} value={sourcePath} maxLength={4096} disabled={busy} placeholder={t("settings.skills.directoryPath")} onChange={event => setSourcePath(event.target.value)} />
        <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void chooseSource()}>{t("settings.skills.chooseDirectory")}</button>
        <button type="submit" className="tauri-settings-button" disabled={busy || !sourcePath.trim()}>{t("settings.skills.addSource")}</button>
      </form>
      <h3>{t("settings.skills.listTitle")}</h3>
      <p>{t("settings.skills.listDescription", { count: view.skills.length })}</p>
      <input className="tauri-settings-input tauri-skills-search" type="search" aria-label={t("settings.skills.searchAria")} value={query} placeholder={t("settings.skills.searchPlaceholder")} onChange={event => setQuery(event.target.value)} />
      <div className="tauri-skills-list">{filtered.length ? filtered.map((item, index) => <div className="tauri-skills-item" key={item.name + "-" + item.sourcePath + "-" + index}>
        <div><strong>{item.invocation || item.name}</strong><span>{item.description}</span><small>{scopeLabel(item.scope)} · {runAsLabel(item.runAs)}{item.sourcePath && item.sourcePath !== "(builtin)" ? " · " + item.sourcePath : ""}</small>{item.requires?.length > 0 && <><small>{t("settings.skills.declaredRequirements", { requirements: item.requires.join(", ") })}</small><div className="tauri-skill-readiness">{assessSkillRequirements(item.requires, capabilityReport).map(({ requirement, state }) => <small key={requirement} className={state === "ready" ? "tauri-skill-readiness__ready" : "tauri-skills-warning"}>{t(readinessKeys[state], { requirement })}</small>)}{readinessCheckFailed && <small className="tauri-skills-warning">{t("settings.skills.readinessUnavailable")}</small>}</div></>}{archiveTarget === item.scope + ":" + item.name && <small className="tauri-skills-warning">{t("settings.skills.archivePrompt")}</small>}</div>
        <label><input type="checkbox" aria-label={t("settings.skills.skillEnabledAria", { name: item.name })} checked={scope === "global" ? (item.globalEnabled ?? item.enabled) : item.enabled} disabled={busy} onChange={event => void change("skill", event.target.checked, item.name)} /></label>
        {item.archiveRevision && item.scope === scope && (archiveTarget === item.scope + ":" + item.name
          ? <div className="tauri-skills-archive-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setArchiveTarget("")}>{t("common.cancel")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void archiveSkill(item)}>{t("settings.skills.confirmArchive")}</button></div>
          : <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setArchiveTarget(item.scope + ":" + item.name)}>{t("settings.skills.archive")}</button>)}
      </div>) : <p>{query ? t("settings.skills.noMatches") : t("settings.skills.noneDiscovered")}</p>}</div>
      {archivedInScope.length > 0 && <><h3>{t("settings.skills.archivedTitle")}</h3><p>{t("settings.skills.archivedDescription")}</p><div className="tauri-skills-list">{archivedInScope.map(item => <div className="tauri-skills-item" key={item.scope + ":" + item.archiveId}><div><strong>{item.name}</strong><small>{scopeLabel(item.scope)} · {item.path}</small></div><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void restoreSkill(item)}>{t("settings.skills.restore")}</button></div>)}</div></>}
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={reload}>{t("settings.skills.refresh")}</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} title={currentSessionState !== "idle" ? t("common.busyHint") : currentSessionHasAttachments ? t("settings.previewProvider.resolveAttachments") : undefined} onClick={() => void applyCurrent()}>{t("settings.permission.applyCurrent")}</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>{t("settings.skills.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}

function skillPathBelongsToSource(skillPath: string, sourcePath: string): boolean {
  const normalize = (value: string) => value.replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase();
  const root = normalize(sourcePath);
  const path = normalize(skillPath);
  return Boolean(root && path && path.startsWith(root + "/"));
}
