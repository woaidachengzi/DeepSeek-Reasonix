import { useCallback, useEffect, useRef, useState } from "react";
import { cancelTauriMCPOAuth, clearTauriMCPAuthentication, deleteTauriMCPServer, resolveTauriMCPMarketplace, saveTauriMCPServer, searchTauriMCPMarketplace, setTauriMCPServerEnabled, startTauriMCPOAuth, tauriMCPOAuthStatus, tauriMCPRuntimeAction, tauriMCPServers, tauriMessageFrom, type TauriMCPOAuthFlow, type TauriMCPMarketplace, type TauriMCPMarketplaceEntry, type TauriMCPServer } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";
import { emptyMCPDraft, mcpDraftForEditing, mcpDraftFromMarketplace, mcpDraftToInput, mcpTransportSummary, type MCPDraft } from "./tauriMCPServers";

function runtimeSummary(server: TauriMCPServer, t: ReturnType<typeof useT>): string {
  if (server.runtimeStatus === "connected") return server.enabled ? t("settings.mcp.runtimeConnected", { count: server.toolCount ?? 0 }) : t("settings.mcp.runtimeConnectedPendingDisable");
  if (server.runtimeStatus === "initializing") return t("settings.mcp.runtimeConnecting");
  if (server.runtimeStatus === "failed") return t("settings.mcp.runtimeFailed");
  return server.enabled ? t("settings.mcp.runtimeOnDemand") : t("caps.pluginDisabled");
}

function localizedMCPError(cause: unknown, t: ReturnType<typeof useT>): string {
  const message = tauriMessageFrom(cause);
  const source = cause instanceof Error ? cause.message : message;
  if (source.includes("此目录条目需要手动配置")) return t("settings.mcp.error.manualRegistryEntry");
  if (source.includes("凭据需要用 KEY=value")) return t("settings.mcp.error.keyValue", { line: source.split("：").slice(-1)[0] ?? "" });
  if (source === "服务器名称不能为空") return t("settings.mcp.error.nameRequired");
  if (source === "stdio 服务器需要填写命令") return t("settings.mcp.error.commandRequired");
  if (source === "http/sse 服务器需要填写 URL") return t("settings.mcp.error.urlRequired");
  if (source === "参数 JSON 格式无效") return t("settings.mcp.error.invalidArgsJson");
  if (source === "参数必须是字符串数组") return t("settings.mcp.error.argsMustBeStrings");
  if (source === "settings.mcp.error.startupTimeout") return t("settings.mcp.error.startupTimeout");
  if (source === "settings.mcp.error.callTimeout") return t("settings.mcp.error.callTimeout");
  if (source === "settings.mcp.error.toolTimeoutJson") return t("settings.mcp.error.toolTimeoutJson");
  if (source === "settings.mcp.error.toolTimeoutObject") return t("settings.mcp.error.toolTimeoutObject");
  if (source === "settings.mcp.error.toolTimeoutValues") return t("settings.mcp.error.toolTimeoutValues");
  return message;
}

function credentialHint(server: TauriMCPServer, t: ReturnType<typeof useT>): string {
  const parts = [
    server.envKeys?.length ? t("settings.mcp.environmentKeys", { keys: server.envKeys.join(", ") }) : "",
    server.headerKeys?.length ? t("settings.mcp.headerKeys", { keys: server.headerKeys.join(", ") }) : "",
  ].filter(Boolean);
  return parts.length ? t("settings.mcp.credentialsWriteOnly", { details: parts.join(" · ") }) : "";
}

export function TauriMCPSettings({ workspaceRoot, sessionId, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  sessionId?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [servers, setServers] = useState<TauriMCPServer[]>([]);
  const [notice, setNotice] = useState("");
  const [draft, setDraft] = useState<MCPDraft | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pendingApply, setPendingApply] = useState(false);
  const [status, setStatus] = useState("");
  const [runtimeActionName, setRuntimeActionName] = useState("");
  const [authFlow, setAuthFlow] = useState<TauriMCPOAuthFlow | null>(null);
  const [marketplaceOpen, setMarketplaceOpen] = useState(false);
  const [marketplaceQuery, setMarketplaceQuery] = useState("");
  const [marketplace, setMarketplace] = useState<TauriMCPMarketplace | null>(null);
  const generation = useRef(0);
  const mutationBusy = useRef(false);

  const refresh = useCallback(async () => {
    const request = ++generation.current;
    setLoading(true);
    try {
      const result = await tauriMCPServers(workspaceRoot);
      if (request === generation.current) {
        setServers(result);
        setNotice("");
      }
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }, [workspaceRoot]);

  useEffect(() => {
    setDraft(null);
    mutationBusy.current = false;
    setBusy(false);
    setServers([]);
    setNotice("");
    setPendingApply(false);
    setStatus("");
    setMarketplaceOpen(false);
    setMarketplace(null);
    void refresh();
    return () => { generation.current += 1; };
  }, [refresh]);

  useEffect(() => {
    if (!authFlow || !sessionId) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const result = await tauriMCPOAuthStatus(sessionId, authFlow.flowId);
        if (!active) return;
        if (result.status === "pending") {
          timer = setTimeout(() => void poll(), 1200);
          return;
        }
        setAuthFlow(null);
        if (result.status === "complete") {
          setStatus(t("settings.mcp.authorizationComplete"));
          setServers(await tauriMCPServers(workspaceRoot));
        } else if (result.status === "failed") setNotice(t("settings.mcp.authorizationFailed"));
        else setStatus(t("settings.mcp.authorizationCanceled"));
      } catch {
        if (active) setAuthFlow(null);
      }
    };
    timer = setTimeout(() => void poll(), 1200);
    return () => { active = false; if (timer) clearTimeout(timer); };
  }, [authFlow, sessionId, workspaceRoot, t]);

  const save = async () => {
    if (!draft || mutationBusy.current) return;
    let input;
    try {
      input = mcpDraftToInput(draft);
    } catch (cause) {
      setNotice(localizedMCPError(cause, t));
      return;
    }
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await saveTauriMCPServer(input, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        setDraft(null);
        setNotice("");
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const browseMarketplace = async (query: string) => {
    if (mutationBusy.current) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    setMarketplaceOpen(true);
    setNotice("");
    try {
      const result = await searchTauriMCPMarketplace(query);
      if (request === generation.current) setMarketplace(result);
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const prepareMarketplace = async (entry: TauriMCPMarketplaceEntry) => {
    if (mutationBusy.current || marketplace?.cached) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    setNotice("");
    try {
      const current = await resolveTauriMCPMarketplace(entry.name);
      const next = mcpDraftFromMarketplace(current, servers, workspaceRoot ? "project" : "global");
      if (request === generation.current) {
        setDraft(next);
        setMarketplaceOpen(false);
        setStatus(t("settings.mcp.registryDraftLoaded", { name: current.title || current.name }));
      }
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const remove = async (server: TauriMCPServer) => {
    if (mutationBusy.current || !window.confirm(t("settings.mcp.confirmDelete", { name: server.name }))) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await deleteTauriMCPServer(server.name, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        if (draft?.name === server.name) setDraft(null);
        setNotice("");
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const toggle = async (server: TauriMCPServer) => {
    if (mutationBusy.current) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    try {
      const result = await setTauriMCPServerEnabled(server.name, !server.enabled, workspaceRoot);
      if (request === generation.current) {
        setServers(result.servers);
        setNotice("");
        setStatus(t(server.enabled ? "settings.mcp.serverDisabled" : "settings.mcp.serverEnabled", { name: server.name }));
        setPendingApply(true);
      }
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setBusy(false);
      }
    }
  };

  const changeRuntimeConnection = async (server: TauriMCPServer, action: "connect" | "disconnect") => {
    if (!sessionId || mutationBusy.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    const request = generation.current;
    mutationBusy.current = true;
    setBusy(true);
    setRuntimeActionName(server.name);
    setNotice("");
    try {
      const result = await tauriMCPRuntimeAction(sessionId, server.name, action);
      if (request !== generation.current) return;
      const refreshedServers = await tauriMCPServers(workspaceRoot);
      if (request !== generation.current) return;
      setServers(refreshedServers);
      setStatus(action === "connect"
        ? t("settings.mcp.runtimeConnected", { count: result.toolCount })
        : t("settings.mcp.runtimeDisconnected", { name: server.name }));
    } catch (cause) {
      if (request === generation.current) setNotice(localizedMCPError(cause, t));
    } finally {
      if (request === generation.current) {
        mutationBusy.current = false;
        setRuntimeActionName("");
        setBusy(false);
      }
    }
  };

  const authorizeServer = async (server: TauriMCPServer) => {
    if (!sessionId || !server.nativeOAuthEligible || mutationBusy.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    mutationBusy.current = true;
    setBusy(true);
    setNotice("");
    setStatus("");
    try {
      setAuthFlow(await startTauriMCPOAuth(sessionId, server.name));
    } catch {
      setNotice(t("settings.mcp.authorizationFailed"));
    } finally {
      mutationBusy.current = false;
      setBusy(false);
    }
  };

  const cancelAuthorization = async () => {
    if (!sessionId || !authFlow || mutationBusy.current) return;
    mutationBusy.current = true;
    setBusy(true);
    try {
      await cancelTauriMCPOAuth(sessionId, authFlow.flowId);
      setAuthFlow(null);
      setStatus(t("settings.mcp.authorizationCanceled"));
    } catch {
      setNotice(t("settings.mcp.authorizationFailed"));
    } finally {
      mutationBusy.current = false;
      setBusy(false);
    }
  };

  const clearAuthentication = async (server: TauriMCPServer) => {
    if (!sessionId || !server.authenticationSaved || mutationBusy.current || authFlow || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    if (!window.confirm(t("settings.mcp.clearAuthConfirm", { name: server.name }))) return;
    mutationBusy.current = true;
    setBusy(true);
    setNotice("");
    try {
      await clearTauriMCPAuthentication(sessionId, server.name);
      setServers(await tauriMCPServers(workspaceRoot));
      setPendingApply(false);
      setStatus(t("settings.mcp.clearAuthDone"));
    } catch (cause) {
      setNotice(localizedMCPError(cause, t));
    } finally {
      mutationBusy.current = false;
      setBusy(false);
    }
  };

  const applyCurrent = async () => {
    if (mutationBusy.current || currentSessionState !== "idle" || currentSessionHasAttachments || !onApplyToCurrentSession) return;
    mutationBusy.current = true;
    setBusy(true);
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setStatus(t("settings.mcp.currentApplied"));
        setNotice("");
      } else setNotice(t("settings.mcp.applyFailed"));
    } catch (cause) { setNotice(localizedMCPError(cause, t)); }
    finally { mutationBusy.current = false; setBusy(false); }
  };

  return <section className="tauri-mcp-settings" aria-label={t("settings.mcp.ariaLabel")}>
    <div className="tauri-mcp-settings__heading">
      <div><h3>{t("settings.mcp.title")}</h3><p>{t("settings.mcp.description", { config: "reasonix.toml" })}</p></div>
      <div className="tauri-mcp-settings__actions">
        <button type="button" className="tauri-settings-button" onClick={() => void refresh()} disabled={busy || loading}>{t("settings.mcp.refresh")}</button>
        {marketplaceOpen ? <button type="button" className="tauri-settings-button" onClick={() => setMarketplaceOpen(false)} disabled={busy}>{t("settings.mcp.backToServers")}</button> : <><button type="button" className="tauri-settings-button" onClick={() => void browseMarketplace("")} disabled={busy || draft !== null}>{t("settings.mcp.browseRegistry")}</button><button type="button" className="tauri-settings-button" onClick={() => setDraft(emptyMCPDraft(workspaceRoot ? "project" : "global"))} disabled={busy || draft !== null}>{t("common.add")}</button></>}
        {pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" onClick={() => void applyCurrent()} disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments}>{t("settings.mcp.applyCurrent")}</button>}
      </div>
    </div>
    {notice && <p className="tauri-diagnostic-error" role="alert">{notice}</p>}
    {status && <p className="tauri-settings-hint" role="status">{status}</p>}
    {authFlow?.status === "pending" && <p className="tauri-settings-hint" role="status">{t("settings.mcp.authorizing")} <button type="button" className="tauri-settings-button" onClick={() => void cancelAuthorization()} disabled={busy}>{t("settings.mcp.cancelAuthorization")}</button></p>}
    {marketplaceOpen ? <div className="tauri-mcp-marketplace">
      <h4>{t("settings.mcp.registryTitle")}</h4><p>{t("settings.mcp.registryDescription")}</p>
      <form className="tauri-mcp-marketplace__search" onSubmit={event => { event.preventDefault(); void browseMarketplace(marketplaceQuery); }}><input type="search" aria-label={t("settings.mcp.searchRegistryLabel")} placeholder={t("settings.mcp.searchRegistryPlaceholder")} value={marketplaceQuery} onChange={event => setMarketplaceQuery(event.target.value)} /><button type="submit" disabled={busy}>{t("settings.mcp.search")}</button></form>
      {marketplace?.cached && <p className="tauri-settings-hint" role="status">{t("settings.mcp.offlineCache")}</p>}
      {busy && !marketplace ? <p className="tauri-settings-loading">{t("settings.mcp.searchingRegistry")}</p> : marketplace && marketplace.servers.length === 0 ? <p className="tauri-settings-empty">{t("settings.mcp.registryNoResults")}</p> : marketplace && <ul className="tauri-mcp-marketplace__list">{marketplace.servers.map(entry => <li key={entry.name}><div><strong>{entry.title || entry.name}</strong><small>{entry.name}{entry.version ? ` · ${entry.version}` : ""}{entry.transport ? ` · ${entry.transport}` : ""}</small>{entry.description && <p>{entry.description}</p>}{!entry.installable && <small>{entry.unavailableReason || t("settings.mcp.manualConfiguration")}</small>}</div>{entry.installable && <button type="button" onClick={() => void prepareMarketplace(entry)} disabled={busy || marketplace.cached}>{t("settings.mcp.configure")}</button>}</li>)}</ul>}
    </div> : loading ? <p className="tauri-settings-loading">{t("settings.mcp.loadingServers")}</p> : servers.length === 0 ? <p className="tauri-settings-empty">{t("settings.mcp.noServers")}</p> : <ul className="tauri-mcp-list">{servers.map(server => <li key={`${server.scope}:${server.name}`}>
      <div className="tauri-mcp-list__row"><strong>{server.name}</strong><span className="tauri-mcp-list__meta">{server.scope === "project" ? t("caps.projectServerBadge") : server.scope === "global" ? t("caps.sourceUser") : server.scope} · {server.type}</span></div>
      <small className={`tauri-mcp-list__runtime${server.runtimeStatus === "failed" ? " is-error" : ""}`}>{runtimeSummary(server, t)}{server.errorKind ? ` · ${server.errorKind}` : ""}</small>
      <code className="tauri-mcp-list__transport">{mcpTransportSummary(server) || "—"}</code>
      {(server.startupTimeoutSeconds || server.callTimeoutSeconds || Object.keys(server.toolTimeoutSeconds ?? {}).length > 0) && <small>{t("settings.mcp.timeoutSummary", { startup: server.startupTimeoutSeconds ?? 0, call: server.callTimeoutSeconds ?? 0, tools: Object.keys(server.toolTimeoutSeconds ?? {}).length })}</small>}
      {server.runtimeStatus === "connected" && (server.toolList?.length ?? 0) > 0 && <details className="tauri-mcp-list__tools"><summary>{t("settings.mcp.viewTools")}</summary><ul>{server.toolList?.map(tool => <li key={tool.name}><strong>{tool.name}</strong>{tool.description && <span>{tool.description}</span>}</li>)}</ul></details>}
      {credentialHint(server, t) && <small>{credentialHint(server, t)}</small>}
      {server.managedByPackage && <small>{t("settings.mcp.managedByPlugin")}</small>}
      <div className="tauri-mcp-list__actions"><button type="button" className="tauri-mcp-list__toggle" role="switch" aria-checked={server.enabled} aria-label={`${server.name} ${server.enabled ? t("caps.pluginEnabled") : t("caps.pluginDisabled")}`} onClick={() => void toggle(server)} disabled={busy}>{server.enabled ? t("caps.pluginEnabled") : t("caps.pluginDisabled")}</button>{sessionId && server.enabled && <>{server.nativeOAuthEligible && <button type="button" onClick={() => void authorizeServer(server)} disabled={busy || Boolean(authFlow) || currentSessionState !== "idle" || currentSessionHasAttachments}>{t("settings.mcp.authorize")}</button>}<button type="button" onClick={() => void changeRuntimeConnection(server, "connect")} disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments}>{runtimeActionName === server.name ? t("settings.mcp.runtimeChanging") : server.runtimeStatus === "connected" ? t("settings.mcp.reconnectCurrent") : t("settings.mcp.connectCurrent")}</button>{server.runtimeStatus === "connected" && <button type="button" onClick={() => void changeRuntimeConnection(server, "disconnect")} disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments}>{t("settings.mcp.disconnectCurrent")}</button>}</>}{sessionId && server.authenticationSaved && !server.managedByPackage && <button type="button" onClick={() => void clearAuthentication(server)} disabled={busy || Boolean(authFlow) || currentSessionState !== "idle" || currentSessionHasAttachments}>{t("settings.mcp.clearAuth")}</button>}<button type="button" onClick={() => setDraft(mcpDraftForEditing(server))} disabled={busy || server.managedByPackage}>{t("common.edit")}</button><button type="button" onClick={() => void remove(server)} disabled={busy || server.managedByPackage}>{t("common.delete")}</button></div>
    </li>)}</ul>}
    {draft && <form className="tauri-mcp-form" onSubmit={event => { event.preventDefault(); void save(); }}>
      <label>{t("settings.mcp.nameLabel")}<input value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} disabled={draft.editing || busy} aria-label={t("settings.mcp.nameLabel")} /></label>
      <label>{t("settings.mcp.scopeLabel")}<select value={draft.scope} onChange={event => setDraft({ ...draft, scope: event.target.value === "project" ? "project" : "global" })} disabled={draft.editing || busy}><option value="project" disabled={!workspaceRoot}>{t("settings.mcp.projectConfigScope", { config: "reasonix.toml" })}</option><option value="global">{t("settings.mcp.globalConfigScope")}</option></select></label>
      <label>{t("settings.mcp.transportLabel")}<select value={draft.type} onChange={event => setDraft({ ...draft, type: event.target.value === "http" ? "http" : event.target.value === "sse" ? "sse" : "stdio" })} disabled={busy}><option value="stdio">stdio</option><option value="http">http</option><option value="sse">sse</option></select></label>
      {draft.type === "stdio" ? <><label>{t("settings.mcp.commandLabel")}<input value={draft.command} onChange={event => setDraft({ ...draft, command: event.target.value })} placeholder="uvx" aria-label={t("settings.mcp.commandLabel")} disabled={busy} /></label><label>{t("settings.mcp.argumentsLabel")}<input value={draft.args} onChange={event => setDraft({ ...draft, args: event.target.value })} placeholder="mcp-server-time" aria-label={t("settings.mcp.argumentsLabel")} disabled={busy} /></label></> : <label>URL<input value={draft.url} onChange={event => setDraft({ ...draft, url: event.target.value })} placeholder="https://…" aria-label="URL" disabled={busy} /></label>}
      <label>{t("settings.mcp.environmentVariables")}<textarea value={draft.env} onChange={event => setDraft({ ...draft, env: event.target.value })} rows={2} placeholder={t("settings.mcp.keyValuePlaceholder")} aria-label={t("settings.mcp.environmentVariables")} disabled={busy} /></label>
      <label>{t("settings.mcp.requestHeaders")}<textarea value={draft.headers} onChange={event => setDraft({ ...draft, headers: event.target.value })} rows={2} placeholder={t("settings.mcp.headerPlaceholder")} aria-label={t("settings.mcp.requestHeaders")} disabled={busy} /></label>
      <label>{t("settings.mcp.startupTimeout")}<input type="number" min={0} step={1} value={draft.startupTimeoutSeconds} onChange={event => setDraft({ ...draft, startupTimeoutSeconds: Number(event.target.value) })} aria-label={t("settings.mcp.startupTimeout")} disabled={busy} /><small>{t("settings.mcp.startupTimeoutHint")}</small></label>
      <label>{t("settings.mcp.callTimeout")}<input type="number" min={0} step={1} value={draft.callTimeoutSeconds} onChange={event => setDraft({ ...draft, callTimeoutSeconds: Number(event.target.value) })} aria-label={t("settings.mcp.callTimeout")} disabled={busy} /><small>{t("settings.mcp.callTimeoutHint")}</small></label>
      <label>{t("settings.mcp.toolTimeouts")}<textarea value={draft.toolTimeoutSeconds} onChange={event => setDraft({ ...draft, toolTimeoutSeconds: event.target.value })} rows={3} placeholder={'{ "search": 120 }'} aria-label={t("settings.mcp.toolTimeouts")} disabled={busy} /><small>{t("settings.mcp.toolTimeoutsHint")}</small></label>
      <div className="tauri-mcp-form__actions"><button type="submit" disabled={busy}>{draft.editing ? t("settings.mcp.saveChanges") : t("settings.mcp.addServer")}</button><button type="button" onClick={() => setDraft(null)} disabled={busy}>{t("common.cancel")}</button></div>
    </form>}
  </section>;
}
