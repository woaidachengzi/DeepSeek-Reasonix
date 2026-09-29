import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriPluginSettings, chooseTauriPluginDirectory, installTauriPlugin, planTauriPluginInstall, removeTauriPlugin, tauriMessageFrom, tauriPluginSettings, type TauriPluginInstallPlan, type TauriPluginItem, type TauriPluginSettings as PluginView } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

export function TauriPluginSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
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
      setNotice(t(enabled ? "settings.plugins.enabledNotice" : "settings.plugins.disabledNotice", { name: item.name }));
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
    } catch (err) { setError(`${t("settings.plugins.previewFailed")}: ${tauriMessageFrom(err)}`); }
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
      setNotice(result.status === "done" ? t("settings.plugins.installedNotice") : t("settings.plugins.partialInstallNotice", { names: result.failedNames.join("、") || t("settings.plugins.unknownPlugin") }));
    } catch (err) { setError(`${t("settings.plugins.installFailed")}: ${tauriMessageFrom(err)}`); }
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
      setNotice(t("settings.plugins.removedNotice", { name: item.name }));
    } catch (err) { setError(`${t("settings.plugins.removeFailed")}: ${tauriMessageFrom(err)}`); }
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
        setNotice(t("settings.plugins.applied"));
      } else setError(t("settings.plugins.applyFailed"));
    } catch { setError(t("settings.plugins.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const filtered = view?.plugins.filter(item => `${item.name} ${item.description} ${item.source}`.toLowerCase().includes(query.trim().toLowerCase())) ?? [];
  return <div className="tauri-settings-section tauri-plugin-settings">
    <h3>{t("settings.plugins.title")}</h3>
    <p>{t("settings.plugins.description")}</p>
    {loading ? <div className="tauri-settings-loading">{t("settings.plugins.loading")}</div> : view ? <>
      <form className="tauri-plugin-install-form" onSubmit={event => void reviewSource(event)}>
        <label htmlFor="tauri-plugin-source">{t("settings.plugins.addPlugin")}<small>{t("settings.plugins.addHint")}</small></label>
        <div><input id="tauri-plugin-source" className="tauri-settings-input" value={source} maxLength={4096} disabled={busy} placeholder={t("settings.plugins.sourcePlaceholder")} onChange={event => { setSource(event.target.value); setPlan(null); setAcceptRisk(false); }} /><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => void chooseSource()}>{t("settings.plugins.chooseDirectory")}</button><button className="tauri-settings-button" type="submit" disabled={busy || !source.trim()}>{t("settings.plugins.previewInstall")}</button></div>
      </form>
      {plan && <section className="tauri-plugin-plan" aria-label={t("settings.plugins.previewAria")}>
        <h4>{t("settings.plugins.previewTitle")}</h4>
        <p>{t("settings.plugins.previewSource", { source: plan.source })}</p>
        {plan.detail.actions.map(action => <div className="tauri-plugin-plan__action" key={action.name}>
          <strong>{action.name}</strong><small>{action.version || t("settings.plugins.unversioned")} · {action.manifestKind} · {t(`settings.plugins.risk.${action.riskLevel}`)}</small>
          <span>{t("settings.plugins.contributionCounts", { skills: action.skills, agents: action.agents, commands: action.commands, hooks: action.hooks, mcp: action.mcpServers, prompts: action.prompts, themes: action.themes })}</span>
          {action.runtime && <p className="tauri-plugin-plan__risk">{t("settings.plugins.runtimeWarning", { command: action.runtimeCommand || t("settings.plugins.unlabeledCommand") })}{action.intercepts.length > 0 && ` ${t("settings.plugins.intercepts", { items: action.intercepts.join("、") })}`}{action.replaces.length > 0 && ` ${t("settings.plugins.replaces", { items: action.replaces.join("、") })}`}</p>}
          {!action.runtime && action.riskLevel === "high" && <p className="tauri-plugin-plan__risk">{t("settings.plugins.highRiskWarning")}</p>}
        </div>)}
        {plan.detail.warningCount > 0 && <div className="tauri-install-warnings"><strong>{t("settings.plugins.compatibilityWarnings", { count: plan.detail.warningCount })}</strong>{plan.detail.warnings.map((warning, index) => <p key={index}>{warning}</p>)}{plan.detail.warningCount > plan.detail.warnings.length && <p>{t("settings.plugins.moreWarnings")}</p>}</div>}
        {plan.detail.actions.some(action => action.riskLevel === "high") && <label className="tauri-plugin-plan__ack"><input type="checkbox" checked={acceptRisk} disabled={busy} onChange={event => setAcceptRisk(event.target.checked)} />{t("settings.plugins.riskAcknowledgement")}</label>}
        <div className="tauri-plugin-plan__actions"><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setPlan(null)}>{t("settings.plugins.cancel")}</button><button className="tauri-settings-button" type="button" disabled={busy || plan.detail.actions.some(action => action.riskLevel === "high") && !acceptRisk} onClick={() => void installReviewed()}>{t("settings.plugins.installReviewed")}</button></div>
      </section>}
      <div className="tauri-plugin-toolbar"><span>{t("settings.plugins.pluginCount", { count: view.plugins.length })}</span><input className="tauri-settings-input" type="search" aria-label={t("settings.plugins.searchAria")} placeholder={t("settings.plugins.searchPlaceholder")} value={query} onChange={event => setQuery(event.target.value)} /><button className="tauri-settings-button" type="button" disabled={busy} onClick={reload}>{t("settings.plugins.refresh")}</button></div>
      <div className="tauri-plugin-list">{filtered.length ? filtered.map(item => <article className="tauri-plugin-card" key={item.name}>
        <div className="tauri-plugin-card__heading"><div><strong>{item.name}</strong><small>{item.version || t("settings.plugins.unversioned")} · {t(`settings.plugins.source.${item.source}`)} · {item.manifestKind || t("settings.plugins.unknownFormat")}</small></div><label><input type="checkbox" role="switch" aria-label={t("settings.plugins.enableAria", { name: item.name })} checked={item.enabled} disabled={busy || item.status !== "ready" && !item.enabled} onChange={event => void setEnabled(item, event.target.checked)} />{item.enabled ? t("settings.plugins.enabled") : t("settings.plugins.disabled")}</label></div>
        {item.description && <p>{item.description}</p>}
        <small className="tauri-plugin-card__root">{item.root}</small>
        <div className="tauri-plugin-card__counts">{item.status === "ready" ? <><span>{t("settings.plugins.skillsCount", { count: item.skills })}</span><span>{t("settings.plugins.agentsCount", { count: item.agents })}</span><span>{t("settings.plugins.commandsCount", { count: item.commands })}</span><span>{t("settings.plugins.hooksCount", { count: item.hooks })}</span><span>{t("settings.plugins.mcpCount", { count: item.mcpServers })}</span>{item.runtime && <span>{t("settings.plugins.runtime")}</span>}</> : <span className="tauri-plugin-card__error">{item.issue || t("settings.plugins.unavailable")}</span>}</div>
        {item.warningCount > 0 && <small>{t("settings.plugins.compatibilityWarnings", { count: item.warningCount })}</small>}
        <div className="tauri-plugin-card__actions">{removeConfirmation === item.name ? <><small>{t("settings.plugins.removeScope")}</small><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setRemoveConfirmation("")}>{t("settings.plugins.cancel")}</button><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => void removeInstalled(item)}>{t("settings.plugins.confirmRemove")}</button></> : <button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setRemoveConfirmation(item.name)}>{t("settings.plugins.remove")}</button>}</div>
      </article>) : <p>{query ? t("settings.plugins.noMatches") : t("settings.plugins.noneInstalled")}</p>}</div>
      {pendingApply && currentSessionState && onApplyToCurrentSession && <button className="tauri-settings-button" type="button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>{t("settings.plugins.applyCurrent")}</button>}
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>{t("settings.plugins.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
