import { useCallback, useEffect, useRef, useState } from "react";
import { writeClipboardText } from "../lib/clipboard";
import { changeTauriHooksSettings, tauriHooksSettings, tauriPluginSettings, tauriMessageFrom, type TauriHooksSettings as HooksView, type TauriPluginItem } from "../lib/tauriBridge";
import { useT, type Translator } from "../lib/i18n";

type HookScope = "global" | "project";

function parseHooksEditor(text: string, events: string[], t: Translator): Record<string, unknown> {
  const parsed: unknown = JSON.parse(text);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error(t("settings.hooks.error.object"));
  const root = parsed as Record<string, unknown>;
  const hooks = root.hooks;
  if (!hooks || typeof hooks !== "object" || Array.isArray(hooks)) throw new Error(t("settings.hooks.error.hooksObject"));
  const valid = new Set(events);
  for (const [event, entries] of Object.entries(hooks)) {
    if (!valid.has(event)) throw new Error(t("settings.hooks.error.unknownEvent", { event }));
    if (!Array.isArray(entries)) throw new Error(t("settings.hooks.error.eventArray", { event }));
    for (const entry of entries) {
      if (!entry || typeof entry !== "object" || Array.isArray(entry) || typeof (entry as Record<string, unknown>).command !== "string" || !(entry as Record<string, unknown>).command?.toString().trim()) {
        throw new Error(t("settings.hooks.error.commandRequired", { event }));
      }
    }
  }
  return hooks as Record<string, unknown>;
}

function formatHooks(hooks: Record<string, unknown>): string {
  return JSON.stringify({ hooks }, null, 2);
}

export function TauriHooksSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession, onOpenPlugins }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
  onOpenPlugins?: () => void;
}) {
  const t = useT();
  const [scope, setScope] = useState<HookScope>("global");
  const [view, setView] = useState<HooksView | null>(null);
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const [plugins, setPlugins] = useState<TauriPluginItem[]>([]);
  const [pluginsLoading, setPluginsLoading] = useState(true);
  const [pluginsError, setPluginsError] = useState("");
  const busyRef = useRef(false);
  const pluginRequest = useRef(0);

  const reloadPluginHooks = useCallback(() => {
    const current = ++pluginRequest.current;
    setPluginsLoading(true);
    setPluginsError("");
    void tauriPluginSettings().then(next => {
      if (pluginRequest.current === current) setPlugins(next.plugins);
    }).catch(err => {
      if (pluginRequest.current === current) setPluginsError(tauriMessageFrom(err));
    }).finally(() => {
      if (pluginRequest.current === current) setPluginsLoading(false);
    });
  }, []);

  useEffect(() => {
    reloadPluginHooks();
    return () => { pluginRequest.current += 1; };
  }, [reloadPluginHooks]);

  const reload = async (nextScope: HookScope) => {
    setLoading(true);
    setError("");
    try {
      const next = await tauriHooksSettings(nextScope, workspaceRoot);
      setView(next);
      setText(formatHooks(next.hooks));
      setNotice("");
    } catch (err) { setView(null); setError(tauriMessageFrom(err)); }
    finally { setLoading(false); }
  };

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    void tauriHooksSettings(scope, workspaceRoot).then(next => {
      if (!active) return;
      setView(next);
      setText(formatHooks(next.hooks));
    }).catch(err => { if (active) { setView(null); setError(tauriMessageFrom(err)); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [scope, workspaceRoot]);

  const dirty = Boolean(view && text !== formatHooks(view.hooks));
  const format = () => {
    if (!view) return;
    try { setText(formatHooks(parseHooksEditor(text, view.events, t))); setError(""); }
    catch (err) { setError(tauriMessageFrom(err)); }
  };
  const copyPath = async () => {
    if (!view?.path) return;
    try {
      if (!await writeClipboardText(view.path)) throw new Error(t("settings.hooks.clipboardUnavailable"));
      setNotice(t("settings.hooks.pathCopied"));
      setError("");
    } catch { setError(t("settings.hooks.pathCopyFailed")); }
  };
  const save = async () => {
    if (!view || busyRef.current) return;
    let hooks: Record<string, unknown>;
    try { hooks = parseHooksEditor(text, view.events, t); }
    catch (err) { setError(tauriMessageFrom(err)); return; }
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriHooksSettings({ scope, workspaceRoot, revision: view.revision, hooks });
      setView(next);
      setText(formatHooks(next.hooks));
      setPendingApply(true);
      setNotice(t("settings.hooks.savedNotice"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || currentSessionState !== "idle" || currentSessionHasAttachments || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice(t("settings.hooks.applied"));
      } else setError(t("settings.hooks.applyFailed"));
    } catch { setError(t("settings.hooks.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const pluginHookPackages = plugins.filter(plugin => plugin.hooks > 0 || (plugin.hookDetails?.length ?? 0) > 0);

  return <div className="tauri-settings-section tauri-hooks-settings">
    <h3>{t("settings.hooks.scopeTitle")}</h3><p>{t("settings.hooks.description")}</p>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.hooks.scope")}<small>{t("settings.hooks.scopeHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.hooks.scopeAria")}>{(["global", "project"] as const).map(value => <button key={value} type="button" role="radio" aria-checked={scope === value} className={`tauri-settings-radio${scope === value ? " is-active" : ""}`} disabled={busy || (value === "project" && !workspaceRoot)} onClick={() => { if (!dirty || window.confirm(t("settings.hooks.confirmDiscard"))) setScope(value); }}>{value === "global" ? t("settings.hooks.global") : t("settings.hooks.project")}</button>)}</div></div>
    {scope === "project" && !workspaceRoot && <p role="status">{t("settings.hooks.projectRequired")}</p>}
    {loading ? <div className="tauri-settings-loading">{t("settings.hooks.loading")}</div> : view ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.hooks.configFile")}<small>{t("settings.hooks.configHint")}</small></span><div className="tauri-hooks-path-control"><code className="tauri-hooks-path" title={view.path}>{view.path}</code><button type="button" className="tauri-settings-button" disabled={busy || !view.path} onClick={() => void copyPath()}>{t("settings.hooks.copyPath")}</button></div></div>
      <h3>{t("settings.hooks.editorTitle")}</h3><p>{t("settings.hooks.editorDescription")}</p>
      <div className="tauri-hooks-toolbar"><button type="button" className="tauri-settings-button" disabled={busy} onClick={format}>{t("settings.hooks.formatCheck")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void navigator.clipboard?.writeText(text).then(() => setNotice(t("settings.hooks.jsonCopied")), () => setError(t("settings.hooks.copyFailed")))}>{t("settings.hooks.copy")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void navigator.clipboard?.readText().then(value => { setText(value); setError(""); }, () => setError(t("settings.hooks.pasteFailed")))}>{t("settings.hooks.paste")}</button></div>
      <textarea className="tauri-hooks-editor" aria-label={t("settings.hooks.editorAria")} spellCheck={false} value={text} disabled={busy} onChange={event => { setText(event.target.value); setError(""); setNotice(""); }} />
      <p>{t("settings.hooks.availableEvents", { events: view.events.join("、") })}</p>
      <div className="tauri-settings-actions"><span role="status">{dirty ? t("settings.hooks.unsaved") : t("settings.hooks.saved")}</span><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => { setText(formatHooks(view.hooks)); setError(""); }}>{t("settings.hooks.discard")}</button><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => void save()}>{t("settings.hooks.save")}</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { if (!dirty || window.confirm(t("settings.hooks.confirmDiscard"))) void reload(scope); }}>{t("settings.hooks.reload")}</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>{t("settings.hooks.applyCurrent")}</button>}</div>
      <section className="tauri-hooks-plugin-inventory" aria-labelledby="tauri-hooks-plugin-title">
        <div className="tauri-hooks-plugin-heading"><div><h3 id="tauri-hooks-plugin-title">{t("settings.hooks.pluginTitle")}</h3><p>{t("settings.hooks.pluginDescription")}</p></div><div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={pluginsLoading} onClick={reloadPluginHooks}>{t("settings.plugins.refresh")}</button>{onOpenPlugins && <button type="button" className="tauri-settings-button" onClick={onOpenPlugins}>{t("settings.tab.plugins")}</button>}</div></div>
        {pluginsLoading ? <div className="tauri-settings-loading">{t("settings.hooks.loading")}</div> : pluginsError ? <div className="tauri-settings-load-error" role="alert"><span>{pluginsError}</span><button type="button" className="tauri-settings-button" onClick={reloadPluginHooks}>{t("settings.hooks.retry")}</button></div> : pluginHookPackages.length ? <div className="tauri-hooks-plugin-list">{pluginHookPackages.map(plugin => <article className="tauri-hooks-plugin" key={plugin.name}>
          <div className="tauri-hooks-plugin__header"><strong>{plugin.name}</strong><span className={plugin.enabled ? "is-enabled" : "is-disabled"}>{t(plugin.enabled ? "settings.plugins.enabled" : "settings.plugins.disabled")}</span><small>{t("settings.plugins.hooksCount", { count: plugin.hooks })}</small></div>
          {plugin.hookDetails?.length ? <ul>{plugin.hookDetails.map((hook, index) => <li key={`${hook.event}-${hook.match ?? ""}-${index}`}><strong>{hook.event}</strong>{hook.match && <code>{hook.match}</code>}{hook.description && <span>{hook.description}</span>}{hook.contextFile && <small>{hook.contextFile}</small>}</li>)}</ul> : <p>{t("settings.hooks.pluginDetailsUnavailable")}</p>}
        </article>)}</div> : <p role="status">{t("settings.hooks.pluginNone")}</p>}
      </section>
    </> : <button type="button" className="tauri-settings-button" onClick={() => void reload(scope)}>{t("settings.hooks.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
