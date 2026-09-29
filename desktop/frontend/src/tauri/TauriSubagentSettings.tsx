import { useEffect, useRef, useState, type CSSProperties } from "react";
import { cancelTauriSubagentProfileTry, changeTauriSubagentSettings, tauriMessageFrom, tauriSubagentProfileTryStatus, tauriSubagentSettings, tryTauriSubagentProfile, type TauriSubagentChange, type TauriSubagentProfile, type TauriSubagentProfileInput, type TauriSubagentSettings } from "../lib/tauriBridge";
import { PROJECT_COLOR_OPTIONS, projectColorValue } from "../lib/projectColors";
import { useT } from "../lib/i18n";
import { CopyButton } from "../components/CopyButton";
const emptyProfile = (): TauriSubagentProfileInput => ({ name: "", description: "", systemPrompt: "", color: "", model: "", effort: "", allowedTools: [], readOnly: false });

export function TauriSubagentSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession, onUseInChat, defaultsOnly = false }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
  onUseInChat?: (command: string) => void;
  defaultsOnly?: boolean;
}) {
  const t = useT();
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
  const [tryTask, setTryTask] = useState("");
  const [tryRunning, setTryRunning] = useState(false);
  const [tryResult, setTryResult] = useState("");
  const [tryError, setTryError] = useState("");
  const tryCancelledRef = useRef(false);
  const tryRunningRef = useRef(false);
  const tryGenerationRef = useRef(0);
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

  useEffect(() => () => {
    tryGenerationRef.current += 1;
    if (tryRunningRef.current) void cancelTauriSubagentProfileTry().catch(() => {});
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
      setNotice(t("settings.subagents.saved"));
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
        setNotice(t("settings.subagents.applied"));
      } else setError(t("settings.subagents.applyFailed"));
    } catch { setError(t("settings.subagents.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const beginCreate = () => {
    if (tryRunning) return;
    setEditing(null);
    setCreating(true);
    setProfileScope(workspaceRoot ? "project" : "global");
    setProfileDraft(emptyProfile());
    setToolsDraft("");
    setTryTask("");
    setTryResult("");
    setTryError("");
    setError("");
  };

  const beginEdit = (profile: TauriSubagentProfile) => {
    if (tryRunning || !profile.editable || !profile.revision) return;
    setCreating(false);
    setEditing({ name: profile.name, scope: profile.scope === "project" ? "project" : "global", revision: profile.revision });
    setProfileScope(profile.scope === "project" ? "project" : "global");
    setProfileDraft({ name: profile.name, description: profile.description, systemPrompt: profile.body || "", color: profile.color || "", model: profile.model || "", effort: profile.effort || "", allowedTools: profile.allowedTools || [], readOnly: Boolean(profile.readOnly) });
    setToolsDraft((profile.allowedTools || []).join(", "));
    setTryTask("");
    setTryResult("");
    setTryError("");
    setError("");
  };

  const saveProfile = async () => {
    if (busyRef.current || tryRunning) return;
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
      setNotice(t("settings.subagents.profileSaved"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const runTry = async () => {
    const task = tryTask.trim();
    const systemPrompt = profileDraft.systemPrompt.trim();
    if (tryRunning || busyRef.current || !task || !systemPrompt) return;
    tryCancelledRef.current = false;
    tryRunningRef.current = true;
    const generation = ++tryGenerationRef.current;
    setTryRunning(true);
    setTryError("");
    setTryResult("");
    const input = {
      ...profileDraft,
      systemPrompt,
      allowedTools: toolsDraft.split(/[,\n]/).map(value => value.trim()).filter(Boolean),
    };
    let polling = false;
    let pollPending = false;
    const pollStatus = async () => {
      if (pollPending) return;
      pollPending = true;
      try {
        const status = await tauriSubagentProfileTryStatus();
        if (generation === tryGenerationRef.current && !tryCancelledRef.current && status.output) setTryResult(status.output);
      } catch {
        // The awaited run request remains authoritative for failures.
      } finally { pollPending = false; }
    };
    polling = true;
    void pollStatus();
    const pollTimer = window.setInterval(() => { if (polling) void pollStatus(); }, 300);
    try {
      setTryResult(await tryTauriSubagentProfile(workspaceRoot, input, task));
    } catch (err) {
      if (generation === tryGenerationRef.current && !tryCancelledRef.current) setTryError(tauriMessageFrom(err));
    } finally {
      polling = false;
      window.clearInterval(pollTimer);
      if (generation === tryGenerationRef.current) {
        tryCancelledRef.current = false;
        tryRunningRef.current = false;
        setTryRunning(false);
      }
    }
  };

  const cancelTry = async () => {
    tryCancelledRef.current = true;
    try { await cancelTauriSubagentProfileTry(); }
    catch (err) {
      tryCancelledRef.current = false;
      setTryError(tauriMessageFrom(err));
    }
  };

  const closeEditor = () => {
    if (tryRunning) void cancelTry();
    setCreating(false);
    setEditing(null);
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
      setNotice(t("settings.subagents.profileDeleted"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  if (loading) return <div className="tauri-settings-loading">{t("common.loading")}</div>;
  if (!view) return <div className="tauri-settings-section"><p className="tauri-diagnostic-error" role="alert">{error || t("settings.subagents.readFailed")}</p><button type="button" className="tauri-settings-button" onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>{t("settings.subagents.retry")}</button></div>;

  const models = [...new Set([view.subagentModel, ...view.profiles.map(item => item.configuredModel), ...view.profiles.map(item => item.model || ""), ...view.modelRefs].filter(Boolean))].sort();
  const effortOptions = (model: string, current: string) => [...new Set(["", ...(view.modelEfforts?.[model] || []).filter(level => level !== "auto"), current].filter((level, index) => index === 0 || Boolean(level)))];
  const defaultEffortOptions = effortOptions(view.subagentModel || view.defaultModel, view.subagentEffort);
  const filteredProfiles = view.profiles.filter(profile => `${profile.name} ${profile.description} ${profile.scope} ${(profile.allowedTools ?? []).join(" ")}`.toLowerCase().includes(query.trim().toLowerCase()));
  const builtins = filteredProfiles.filter(profile => profile.scope === "builtin");
  const customProfiles = filteredProfiles.filter(profile => (profile.scope === "project" || profile.scope === "global") && profile.invocationMode === "manual");
  const externalProfiles = filteredProfiles.filter(profile => profile.scope === "custom" && profile.invocationMode === "manual");
  const effortLabel = (value: string) => value || t("settings.subagents.inheritEffort");
  const scopeLabel = (value: string) => {
    if (value === "builtin") return t("caps.skillScopeBuiltin");
    if (value === "project") return t("caps.skillScopeProject");
    if (value === "custom") return t("caps.skillScopeCustom");
    if (value === "global") return t("caps.skillScopeGlobal");
    return value;
  };
  const colorLabel = (key: string) => t("settings.subagents.color." + (key || "default") as Parameters<typeof t>[0]);
  const renderProfile = (profile: TauriSubagentProfile, kind: "builtin" | "custom" | "external") => {
    const invocation = profile.invocation || "/" + profile.name;
    const command = `${invocation} `;
    const invocationExample = t("subagents.invocationExample", { name: invocation.replace(/^\//, "") });
    const allowedTools = profile.allowedTools ?? [];
    const effectiveModel = profile.configuredModel || view.subagentModel || view.defaultModel || t("common.auto");
    const effectiveEffort = profile.configuredEffort || view.subagentEffort || t("common.auto");
    const color = projectColorValue(profile.color);
    const builtinDescription = profile.name === "explore" ? t("subagents.builtinExploreDescription")
      : profile.name === "research" ? t("subagents.builtinResearchDescription")
        : profile.name === "review" ? t("subagents.builtinReviewDescription")
          : profile.name === "security-review" ? t("subagents.builtinSecurityReviewDescription") : profile.description;
    return <article className={`tauri-subagent-profile tauri-subagent-profile--${kind}`} key={profile.scope + ":" + profile.name}>
      <div className="tauri-subagent-profile-heading">
        <strong>{kind === "custom" && color && <i className="tauri-subagent-color-dot" style={{ "--project-accent": color } as CSSProperties} aria-hidden="true" />}{invocation}</strong>
        <div className="tauri-subagent-profile-badges"><span className={`tauri-subagent-profile-badge tauri-subagent-profile-badge--${profile.scope}`}>{scopeLabel(profile.scope)}</span>{profile.model && <span className="tauri-subagent-profile-badge">{profile.model}</span>}<span className="tauri-subagent-profile-badge" title={allowedTools.length ? allowedTools.join(", ") : t("subagents.allTools")}>{allowedTools.length ? t("subagents.toolCount", { n: allowedTools.length }) : t("subagents.allTools")}</span>{kind === "external" && <span className="tauri-subagent-profile-badge">{t("subagents.externalManaged")}</span>}</div>
      </div>
      {(kind === "builtin" ? builtinDescription : profile.description) && <p>{kind === "builtin" ? builtinDescription : profile.description}</p>}
      <div className="tauri-subagent-invocation"><span>{t("subagents.invocationLabel")}</span><code>{invocationExample}</code><CopyButton text={invocationExample} label={t("subagents.copyInvocation")} className="tauri-subagent-copy" /><button type="button" className="tauri-settings-button" disabled={!onUseInChat} onClick={() => onUseInChat?.(command)}>{t("subagents.useInChat")}</button></div>
      {kind === "builtin" && <>
        <div className="tauri-subagent-profile-controls"><label>{t("settings.subagents.model")}<select className="tauri-settings-input" aria-label={profile.name + " " + t("settings.subagents.model")} value={profile.configuredModel || ""} disabled={busy} onChange={event => void change("profile_model", event.target.value, 0, profile.name)}><option value="">{t("settings.subagents.inheritDefault")}</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select><small className="tauri-subagent-effective-value">{t("subagents.effectiveValue", { value: effectiveModel })}</small></label><label>{t("settings.subagents.effort")}<select className="tauri-settings-input" aria-label={profile.name + " " + t("settings.subagents.effort")} value={profile.configuredEffort || ""} disabled={busy} onChange={event => void change("profile_effort", event.target.value, 0, profile.name)}>{effortOptions(profile.configuredModel || view.subagentModel || view.defaultModel, profile.configuredEffort).map(value => <option key={value} value={value}>{effortLabel(value)}</option>)}</select><small className="tauri-subagent-effective-value">{t("subagents.effectiveValue", { value: effectiveEffort })}</small></label></div>
        <small className="tauri-subagent-profile-readonly">{profile.configuredModel || profile.configuredEffort ? t("subagents.overridden") : t("subagents.inherited")}</small>
      </>}
      {kind === "custom" && profile.editable ? <div className="tauri-subagent-profile-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => beginEdit(profile)}>{t("settings.subagents.editProfileAction")}</button>{deleteConfirm === profile.scope + ":" + profile.name ? <><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void deleteProfile(profile)}>{t("settings.subagents.confirmDelete")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDeleteConfirm("")}>{t("common.cancel")}</button></> : <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDeleteConfirm(profile.scope + ":" + profile.name)}>{t("settings.subagents.deleteProfile")}</button>}</div> : kind !== "builtin" && <small className="tauri-subagent-profile-readonly">{kind === "external" ? t("subagents.externalManagedHint") : profile.editReason ? t("settings.subagents.readOnlyReason") : ""}</small>}
    </article>;
  };
  return <div className={`tauri-settings-section tauri-subagent-settings${defaultsOnly ? " tauri-subagent-settings--defaults-only" : ""}`}>
    {!defaultsOnly && <><h3>{t("settings.subagents.defaultsTitle")}</h3><p>{t("settings.subagents.defaultsDescription")}</p></>}
    <div className="tauri-settings-field">
      <span className="tauri-settings-field-label">{t("settings.subagents.defaultModel")}<small>{t("settings.subagents.defaultModelHint", { model: view.defaultModel || t("settings.subagents.notSet") })}</small></span>
      <select className="tauri-settings-input" aria-label={t("settings.subagents.defaultModelAria")} value={view.subagentModel} disabled={busy} onChange={event => void change("model", event.target.value)}><option value="">{t("settings.subagents.followMain")}</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select>
    </div>
    <div className="tauri-settings-field">
      <span className="tauri-settings-field-label">{t("settings.subagents.defaultEffort")}<small>{t("settings.subagents.effortHint")}</small></span>
      <select className="tauri-settings-input" aria-label={t("settings.subagents.defaultEffortAria")} value={view.subagentEffort || ""} disabled={busy} onChange={event => void change("effort", event.target.value)}>{defaultEffortOptions.map(value => <option key={value} value={value}>{effortLabel(value)}</option>)}</select>
    </div>
    <div className="tauri-settings-field">
      <span className="tauri-settings-field-label">{t("settings.subagents.depth")}<small>{t("settings.subagents.depthHint")}</small></span>
      <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.subagents.depthAria")}>{[1, 2].map(depth => <button key={depth} type="button" role="radio" aria-checked={view.maxDepth === depth} className={"tauri-settings-radio" + (view.maxDepth === depth ? " is-active" : "")} disabled={busy} onClick={() => void change("depth", "", depth)}>{t("settings.subagents.levels", { count: depth })}</button>)}</div>
    </div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.subagents.concurrency")}<small>{t("settings.subagents.concurrencyHint")}</small></span><select className="tauri-settings-input" aria-label={t("settings.subagents.concurrencyAria")} value={view.maxConcurrency} disabled={busy} onChange={event => void change("concurrency", "", Number(event.target.value))}>{Array.from({ length: 32 }, (_, index) => index + 1).map(value => <option key={value} value={value}>{value}</option>)}</select></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.subagents.writers")}<small>{t("settings.subagents.writersHint")}</small></span><select className="tauri-settings-input" aria-label={t("settings.subagents.writersAria")} value={view.maxParallelWriters} disabled={busy} onChange={event => void change("writers", "", Number(event.target.value))}>{Array.from({ length: view.maxConcurrency }, (_, index) => index + 1).map(value => <option key={value} value={value}>{value}</option>)}</select></div>
    {!defaultsOnly && <>
    <h3>{t("settings.subagents.profilesTitle")}</h3>
    <p>{t("settings.subagents.profilesDescription", { count: view.profiles.length })}</p>
    <div className="tauri-subagent-toolbar"><input className="tauri-settings-input" type="search" aria-label={t("settings.subagents.searchAria")} placeholder={t("settings.subagents.searchPlaceholder")} value={query} onChange={event => setQuery(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy || tryRunning} onClick={beginCreate}>{t("settings.subagents.createProfile")}</button><button type="button" className="tauri-settings-button" disabled={busy || tryRunning} onClick={() => { setLoading(true); void tauriSubagentSettings(workspaceRoot).then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false)); }}>{t("settings.subagents.refresh")}</button></div>
    {(creating || editing) && <form className="tauri-subagent-editor" onSubmit={event => { event.preventDefault(); void saveProfile(); }}>
      <h4>{editing ? t("settings.subagents.editProfile", { name: editing.name }) : t("settings.subagents.newProfile")}</h4>
      <div className="tauri-subagent-editor-grid"><label>{t("settings.subagents.name")}<input className="tauri-settings-input" aria-label={t("settings.subagents.nameAria")} value={profileDraft.name} maxLength={64} disabled={busy || Boolean(editing)} onChange={event => setProfileDraft(draft => ({ ...draft, name: event.target.value }))} /></label><label>{t("settings.subagents.scope")}<select className="tauri-settings-input" aria-label={t("settings.subagents.scopeAria")} value={profileScope} disabled={busy || Boolean(editing)} onChange={event => setProfileScope(event.target.value as "global" | "project")}><option value="global">{t("settings.skills.scope.global")}</option>{workspaceRoot && <option value="project">{t("settings.skills.scope.project")}</option>}</select></label></div>
      <label>{t("settings.subagents.description")}<input className="tauri-settings-input" aria-label={t("settings.subagents.descriptionAria")} value={profileDraft.description} maxLength={512} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, description: event.target.value }))} /></label>
      <label>{t("settings.subagents.systemPrompt")}<textarea className="tauri-settings-input" aria-label={t("settings.subagents.systemPromptAria")} value={profileDraft.systemPrompt} maxLength={65536} disabled={busy} rows={8} onChange={event => setProfileDraft(draft => ({ ...draft, systemPrompt: event.target.value }))} /></label>
      <div className="tauri-subagent-editor-grid"><label>{t("settings.subagents.model")}<select className="tauri-settings-input" aria-label={t("settings.subagents.modelAria")} value={profileDraft.model} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, model: event.target.value, effort: "" }))}><option value="">{t("settings.subagents.inheritDefault")}</option>{models.map(model => <option key={model} value={model}>{model}</option>)}</select></label><label>{t("settings.subagents.effort")}<select className="tauri-settings-input" aria-label={t("settings.subagents.effortAria")} value={profileDraft.effort} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, effort: event.target.value }))}>{effortOptions(profileDraft.model || view.subagentModel || view.defaultModel, profileDraft.effort).map(value => <option key={value} value={value}>{effortLabel(value)}</option>)}</select></label></div>
      <div className="tauri-subagent-editor-grid"><label>{t("settings.subagents.color")}<select className="tauri-settings-input" aria-label={t("settings.subagents.colorAria")} value={profileDraft.color} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, color: event.target.value }))}>{PROJECT_COLOR_OPTIONS.map(option => <option key={option.key || "default"} value={option.key}>{colorLabel(option.key)}</option>)}</select></label><label>{t("settings.subagents.allowedTools")}<small>{t("settings.subagents.allowedToolsHint")}</small><textarea className="tauri-settings-input" aria-label={t("settings.subagents.allowedToolsAria")} value={toolsDraft} disabled={busy} rows={3} onChange={event => setToolsDraft(event.target.value)} /></label></div>
      <label className="tauri-subagent-editor-check"><input type="checkbox" checked={profileDraft.readOnly} disabled={busy} onChange={event => setProfileDraft(draft => ({ ...draft, readOnly: event.target.checked }))} />{t("settings.subagents.readOnly")}</label>
      <div className="tauri-subagent-try">
        <label htmlFor="tauri-subagent-try-task">{t("subagents.tryIt")}</label>
        <div className="tauri-subagent-try-row"><input id="tauri-subagent-try-task" className="tauri-settings-input" value={tryTask} maxLength={16384} disabled={tryRunning} placeholder={t("subagents.tryItPlaceholder")} onChange={event => setTryTask(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={!tryRunning && (!profileDraft.systemPrompt.trim() || !tryTask.trim() || busy)} onClick={() => { if (tryRunning) void cancelTry(); else void runTry(); }}>{tryRunning ? t("subagents.cancelRun") : t("subagents.run")}</button></div>
        {tryError && <p className="tauri-diagnostic-error" role="alert">{tryError}</p>}
        {tryResult && <pre className="tauri-subagent-try-result" aria-label={t("subagents.tryIt")}>{tryResult}</pre>}
      </div>
      <div className="tauri-settings-actions"><button type="submit" className="tauri-settings-button" disabled={busy || tryRunning || !profileDraft.name.trim() || !profileDraft.description.trim() || !profileDraft.systemPrompt.trim()}>{t("settings.subagents.saveProfile")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={closeEditor}>{t("common.cancel")}</button></div>
    </form>}
    <div className="tauri-subagent-profile-groups">
      {builtins.length > 0 && <section className="tauri-subagent-profile-group"><header><h4>{t("subagents.builtinTitle")}</h4><p>{t("subagents.builtinHint")}</p></header><div className="tauri-subagent-profiles">{builtins.map(profile => renderProfile(profile, "builtin"))}</div></section>}
      {customProfiles.length > 0 && <section className="tauri-subagent-profile-group"><header><h4>{t("subagents.customTitle")}</h4><p>{t("subagents.customHint")}</p></header><div className="tauri-subagent-profiles">{customProfiles.map(profile => renderProfile(profile, "custom"))}</div></section>}
      {externalProfiles.length > 0 && <section className="tauri-subagent-profile-group"><header><h4>{t("subagents.externalManaged")}</h4><p>{t("subagents.externalManagedHint")}</p></header><div className="tauri-subagent-profiles">{externalProfiles.map(profile => renderProfile(profile, "external"))}</div></section>}
      {builtins.length + customProfiles.length + externalProfiles.length === 0 && <p>{query ? t("settings.subagents.noMatches") : t("settings.subagents.noneDiscovered")}</p>}
    </div>
    </>}
    {pendingApply && currentSessionState && onApplyToCurrentSession && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} title={currentSessionState !== "idle" ? t("common.busyHint") : currentSessionHasAttachments ? t("settings.previewProvider.resolveAttachments") : undefined} onClick={() => void applyCurrent()}>{t("settings.permission.applyCurrent")}</button></div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
