import { useCallback, useEffect, useState } from "react";
import { Activity, Bot, RefreshCw, ShieldAlert } from "lucide-react";
import { changeTauriBotSettings, tauriBotRuntimeStatus, tauriBotSettings, tauriMessageFrom, tauriProviderSummary, type TauriBotRuntimeStatus, type TauriBotSettings, type TauriBotSettingsChange, type TauriProviderSummary } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

export function TauriBotSettings() {
  const t = useT();
  const [status, setStatus] = useState<TauriBotRuntimeStatus | null>(null);
  const [settings, setSettings] = useState<TauriBotSettings | null>(null);
  const [providerSummary, setProviderSummary] = useState<TauriProviderSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [credentialDrafts, setCredentialDrafts] = useState<Record<string, { identity: string; secret: string }>>({});
  const [accessDrafts, setAccessDrafts] = useState<Record<string, string>>({});
  const [workspaceDrafts, setWorkspaceDrafts] = useState<Record<string, string>>({});

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [nextSettings, nextStatus, nextProviders] = await Promise.all([tauriBotSettings(), tauriBotRuntimeStatus(), tauriProviderSummary()]);
      setSettings(nextSettings);
      setStatus(nextStatus);
      setProviderSummary(nextProviders);
    }
    catch (err) { setError(tauriMessageFrom(err)); }
    finally { setLoading(false); }
  }, []);

  const change = async (action: "set_enabled" | "set_channel_enabled", enabled: boolean, channelId?: string) => {
    if (saving) return;
    setSaving(true); setError("");
    try {
      const next = await changeTauriBotSettings(action === "set_enabled"
        ? { action, enabled }
        : { action, enabled, channelId: channelId ?? "" });
      setSettings(next);
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveAccessPolicy = async (change: Extract<TauriBotSettingsChange, { action: "set_pairing" }>) => {
    if (saving) return;
    setSaving(true); setError("");
    try {
      const next = await changeTauriBotSettings(change);
      setSettings(next);
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveAllowAll = async (enabled: boolean) => {
    if (saving || (enabled && !window.confirm(t("settings.bots.allowAllConfirm")))) return;
    setSaving(true); setError("");
    try {
      setSettings(await changeTauriBotSettings({ action: "set_allow_all", enabled }));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveChannelAccess = async (change: Extract<TauriBotSettingsChange, { action: "set_channel_access_mode" | "set_channel_pairing" }>) => {
    if (saving || (change.action === "set_channel_access_mode" && change.mode === "everyone" && !window.confirm(t("settings.bots.allowAllConfirm")))) return;
    setSaving(true); setError("");
    try {
      setSettings(await changeTauriBotSettings(change));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveChannelAllowlist = async (channelId: string, list: "users" | "groups" | "approvers" | "admins") => {
    const key = `channel:${channelId}:${list}`;
    const access = settings?.channels.find(channel => channel.id === channelId)?.access;
    const raw = accessDrafts[key] ?? joinBotAllowlist(access?.[list]);
    const values = [...new Set(raw.split(/[\n,，]/).map(value => value.trim()).filter(Boolean))];
    if (values.some(value => value.length > 512) || values.length > 100 || saving) {
      setError(t("settings.bots.allowlistInvalid"));
      return;
    }
    setSaving(true); setError("");
    try {
      setSettings(await changeTauriBotSettings({ action: "set_channel_allowlist", channelId, list, values }));
      setAccessDrafts(current => ({ ...current, [key]: values.join("\n") }));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveChannelRuntime = async (change: Extract<TauriBotSettingsChange, { action: "set_channel_runtime" }>) => {
    if (saving) return;
    setSaving(true); setError("");
    try {
      setSettings(await changeTauriBotSettings(change));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveAllowlist = async (platform: "qq" | "feishu" | "weixin" | "dingtalk", list: "users" | "groups" | "approvers" | "admins") => {
    const key = `${platform}.${list}`;
    const raw = accessDrafts[key] ?? joinBotAllowlist(settings?.allowlist?.[platform]?.[list]);
    const values = [...new Set(raw.split(/[\n,，]/).map(value => value.trim()).filter(Boolean))];
    if (values.some(value => value.length > 512) || values.length > 100 || saving) {
      setError(t("settings.bots.allowlistInvalid"));
      return;
    }
    setSaving(true); setError("");
    try {
      const next = await changeTauriBotSettings({ action: "set_allowlist", platform, list, values });
      setSettings(next);
      setAccessDrafts(current => ({ ...current, [key]: values.join("\n") }));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const saveCredentials = async (channelId: string) => {
    const draft = credentialDrafts[channelId];
    if (saving || !draft?.identity.trim() || !draft.secret) return;
    setSaving(true); setError("");
    try {
      const next = await changeTauriBotSettings({ action: "set_credentials", channelId, identity: draft.identity, secret: draft.secret });
      setSettings(next);
      setCredentialDrafts(current => ({ ...current, [channelId]: { identity: draft.identity, secret: "" } }));
      setStatus(await tauriBotRuntimeStatus());
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 15_000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const statusLabel = (value: string) => {
    const keys: Record<string, "settings.bots.stateRunning" | "settings.bots.stateStopped" | "settings.bots.stateBlocked" | "settings.bots.stateDegraded" | "settings.bots.stateError" | "settings.bots.stateUnknown"> = {
      running: "settings.bots.stateRunning", connected: "settings.bots.stateRunning", stopped: "settings.bots.stateStopped",
      blocked: "settings.bots.stateBlocked", degraded: "settings.bots.stateDegraded", error: "settings.bots.stateError",
    };
    return t(keys[value] ?? "settings.bots.stateUnknown");
  };
  return <div className="tauri-settings-section tauri-bot-settings">
    <div className="tauri-settings-actions">
      <span className={`tauri-bot-status tauri-bot-status--${status?.status ?? "unknown"}`}><i aria-hidden="true" />{loading && !status ? t("common.loading") : status ? statusLabel(status.status) : t("settings.bots.stateUnknown")}</span>
      <button type="button" className="tauri-settings-button" disabled={loading} onClick={() => void refresh()}><RefreshCw size={14} />{t("settings.bots.refresh")}</button>
    </div>
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {settings && <>
      <h3>{t("settings.bots.runtimeTitle")}</h3>
      <label className="tauri-settings-toggle"><Bot className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.bots.enabled")}</strong><small>{t("settings.bots.enabledHint")}</small></span><input type="checkbox" checked={settings.enabled} disabled={saving || (!settings.enabled && !settings.accessControlConfigured)} onChange={event => void change("set_enabled", event.target.checked)} /></label>
      {!settings.accessControlConfigured && <p className="tauri-settings-hint">{t("settings.bots.accessRequired")}</p>}
      <h3>{t("settings.bots.accessTitle")}</h3>
      <label className="tauri-settings-toggle"><ShieldAlert className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.bots.pairingEnabled")}</strong><small>{t("settings.bots.pairingHint")}</small></span><input type="checkbox" checked={settings.pairingEnabled} disabled={saving} onChange={event => void saveAccessPolicy({ action: "set_pairing", enabled: event.target.checked })} /></label>
      <label className="tauri-settings-toggle"><ShieldAlert className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.bots.allowAll")}</strong><small>{t("settings.bots.allowAllHint")}</small></span><input type="checkbox" checked={settings.allowAll} disabled={saving} onChange={event => void saveAllowAll(event.target.checked)} /></label>
      {settings.allowAll && <div className="tauri-bot-notice" role="alert"><ShieldAlert size={17} /><span>{t("settings.bots.allowAllWarning")}</span></div>}
      <p className="tauri-settings-hint">{settings.allowlistEnabled && !settings.allowAll ? t("settings.bots.allowlistEnabled") : t("settings.bots.allowlistDisabled")}</p>
      <div className="tauri-bot-access-list">{(["qq", "feishu", "weixin", "dingtalk"] as const).map(platform => <section key={platform} className="tauri-bot-access-platform">
        <h4>{t((`settings.bots.channel.${platform}`) as "settings.bots.channel.qq" | "settings.bots.channel.feishu" | "settings.bots.channel.weixin" | "settings.bots.channel.dingtalk")}</h4>
        {(["users", "groups", "approvers", "admins"] as const).map(list => {
          const key = `${platform}.${list}`;
          return <div className="tauri-bot-access-field" key={key}><label htmlFor={`bot-access-${key}`}>{t(`settings.bots.access.${list}` as "settings.bots.access.users" | "settings.bots.access.groups" | "settings.bots.access.approvers" | "settings.bots.access.admins")}</label><textarea id={`bot-access-${key}`} className="tauri-settings-input" rows={2} maxLength={102400} disabled={saving} value={accessDrafts[key] ?? joinBotAllowlist(settings.allowlist?.[platform]?.[list])} placeholder={t("settings.bots.allowlistPlaceholder")} onChange={event => setAccessDrafts(current => ({ ...current, [key]: event.target.value }))} /><button type="button" className="tauri-settings-button" disabled={saving} onClick={() => void saveAllowlist(platform, list)}>{t("settings.bots.saveAllowlist")}</button></div>;
        })}
      </section>)}</div>
      <h3>{t("settings.bots.channelsTitle")}</h3>
      <div className="tauri-bot-channel-list">{settings.channels.map(channel => {
        const draft = credentialDrafts[channel.id] ?? { identity: channel.credentialIdentity ?? "", secret: "" };
        const access = channel.access;
        return <section className="tauri-bot-channel-card" key={channel.id}>
          <label className="tauri-settings-toggle tauri-bot-channel">
            <Bot className="tauri-settings-field-icon" size={18} />
            <span><strong>{channel.id.startsWith("legacy:") ? t((`settings.bots.channel.${channel.platform}`) as "settings.bots.channel.qq" | "settings.bots.channel.feishu" | "settings.bots.channel.weixin" | "settings.bots.channel.dingtalk") : channel.label}</strong><small>{channel.domain ? `${channel.platform} · ${channel.domain}` : channel.platform} · {channel.credentialsSet ? t("settings.bots.credentialsReady") : t("settings.bots.credentialsMissing")}</small></span>
            <input type="checkbox" checked={channel.enabled} disabled={saving || (!channel.enabled && (!settings.accessControlConfigured || channel.credentialMissing))} onChange={event => void change("set_channel_enabled", event.target.checked, channel.id)} />
          </label>
          <div className="tauri-bot-credential-editor">
            <input className="tauri-settings-input" aria-label={`${t("settings.bots.identity")} · ${channel.label}`} autoComplete="off" maxLength={512} value={draft.identity} disabled={saving} placeholder={t("settings.bots.identity")} onChange={event => setCredentialDrafts(current => ({ ...current, [channel.id]: { ...draft, identity: event.target.value } }))} />
            <input className="tauri-settings-input" aria-label={`${t("settings.bots.credential")} · ${channel.label}`} type="password" autoComplete="new-password" maxLength={8192} value={draft.secret} disabled={saving} placeholder={channel.credentialsSet ? t("settings.bots.replaceCredential") : t("settings.bots.credential")} onChange={event => setCredentialDrafts(current => ({ ...current, [channel.id]: { ...draft, secret: event.target.value } }))} />
            <button type="button" className="tauri-settings-button" disabled={saving || !draft.identity.trim() || !draft.secret} onClick={() => void saveCredentials(channel.id)}>{t("settings.bots.saveCredentials")}</button>
          </div>
          {channel.runtimeSettings && <details className="tauri-bot-channel-access tauri-bot-channel-runtime">
            <summary>{t("settings.botRuntimeSettings")}</summary>
            <div className="tauri-bot-channel-runtime__body">
              <label className="tauri-bot-runtime-field"><span>{t("settings.botChannelModel")}</span><select className="tauri-settings-input" value={channel.model ?? ""} disabled={saving} onChange={event => void saveChannelRuntime({ action: "set_channel_runtime", channelId: channel.id, model: event.target.value })}>
                <option value="">{t("settings.botChannelModelAuto")}</option>
                {providerSummary?.providers.flatMap(provider => provider.models.map(model => `${provider.name}/${model}`)).filter((model, index, all) => all.indexOf(model) === index).map(model => <option value={model} key={model}>{model}</option>)}
                {channel.model && !providerSummary?.providers.some(provider => provider.models.some(model => `${provider.name}/${model}` === channel.model)) && <option value={channel.model}>{channel.model}</option>}
              </select></label>
              <label className="tauri-bot-runtime-field"><span>{t("settings.botToolApprovalMode")}</span><select className="tauri-settings-input" value={channel.toolApprovalMode ?? ""} disabled={saving} onChange={event => void saveChannelRuntime({ action: "set_channel_runtime", channelId: channel.id, toolApprovalMode: event.target.value as "" | "ask" | "auto" | "yolo" })}>
                <option value="">{t("settings.botToolApprovalMode.inherit")}</option>
                {(["ask", "auto", "yolo"] as const).map(mode => <option value={mode} key={mode}>{t(`settings.botToolApprovalMode.${mode}` as "settings.botToolApprovalMode.ask" | "settings.botToolApprovalMode.auto" | "settings.botToolApprovalMode.yolo")}</option>)}
              </select></label>
              <label className="tauri-bot-runtime-field"><span>{t("settings.botWorkspaceRoot")}</span><input className="tauri-settings-input" value={workspaceDrafts[channel.id] ?? channel.workspaceRoot ?? ""} disabled={saving} placeholder={t("settings.botWorkspaceRootPlaceholder")} spellCheck={false} maxLength={4096} onChange={event => setWorkspaceDrafts(current => ({ ...current, [channel.id]: event.target.value }))} onBlur={event => event.currentTarget.value !== (channel.workspaceRoot ?? "") && void saveChannelRuntime({ action: "set_channel_runtime", channelId: channel.id, workspaceRoot: event.currentTarget.value })} /></label>
            </div>
          </details>}
          {access && <details className="tauri-bot-channel-access">
            <summary>{t("settings.bots.channelAccess")}</summary>
            <div className="tauri-bot-channel-access__body">
              <span className="tauri-settings-field-label">{t("settings.bots.channelAccessMode")}</span>
              <div className="tauri-settings-radio-group" role="radiogroup" aria-label={`${channel.label} · ${t("settings.bots.channelAccessMode")}`}>
                <button type="button" role="radio" aria-checked={!access.allowAll} disabled={saving} className={`tauri-settings-radio${!access.allowAll ? " is-active" : ""}`} onClick={() => void saveChannelAccess({ action: "set_channel_access_mode", channelId: channel.id, mode: "trusted" })}>{t("settings.bots.accessTrusted")}</button>
                <button type="button" role="radio" aria-checked={access.allowAll} disabled={saving} className={`tauri-settings-radio${access.allowAll ? " is-active" : ""}`} onClick={() => void saveChannelAccess({ action: "set_channel_access_mode", channelId: channel.id, mode: "everyone" })}>{t("settings.bots.accessEveryone")}</button>
              </div>
              {access.allowAll ? <div className="tauri-bot-notice" role="alert"><ShieldAlert size={17} /><span>{t("settings.bots.allowAllWarning")}</span></div> : <div className="tauri-bot-channel-access__lists">{(["users", "groups", "approvers", "admins"] as const).map(list => {
                const key = `channel:${channel.id}:${list}`;
                return <div className="tauri-bot-access-field" key={key}><label htmlFor={`bot-channel-access-${channel.id}-${list}`}>{t(`settings.bots.access.${list}` as "settings.bots.access.users" | "settings.bots.access.groups" | "settings.bots.access.approvers" | "settings.bots.access.admins")}</label><textarea id={`bot-channel-access-${channel.id}-${list}`} className="tauri-settings-input" rows={2} maxLength={102400} disabled={saving} value={accessDrafts[key] ?? joinBotAllowlist(access?.[list])} placeholder={t("settings.bots.allowlistPlaceholder")} onChange={event => setAccessDrafts(current => ({ ...current, [key]: event.target.value }))} /><button type="button" className="tauri-settings-button" disabled={saving} onClick={() => void saveChannelAllowlist(channel.id, list)}>{t("settings.bots.saveAllowlist")}</button></div>;
              })}</div>}
              <label className="tauri-settings-toggle"><ShieldAlert className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.bots.pairingEnabled")}</strong><small>{t("settings.bots.pairingHint")}</small></span><input type="checkbox" checked={access.pairingEnabled} disabled={saving} onChange={event => void saveChannelAccess({ action: "set_channel_pairing", channelId: channel.id, enabled: event.target.checked })} /></label>
            </div>
          </details>}
        </section>;
      })}</div>
      <p className="tauri-settings-hint">{t("settings.bots.configPath", { path: settings.configPath })}</p>
    </>}
    {status && <>
      <p>{t("settings.bots.runtimeMessage", { message: status.message })}</p>
      <div className="tauri-bot-summary">
        <span><strong>{status.connections}</strong><small>{t("settings.bots.connections")}</small></span>
        {status.startedAt && <span><strong>{new Date(status.startedAt).toLocaleString()}</strong><small>{t("settings.bots.startedAt")}</small></span>}
      </div>
      {status.status === "blocked" && <div className="tauri-bot-notice"><ShieldAlert size={17} /><span>{t("settings.bots.accessRequired")}</span></div>}
      {!status.desktopBridgeAvailable && <div className="tauri-bot-notice"><ShieldAlert size={17} /><span>{t("settings.bots.desktopUnavailable")}</span></div>}
      {status.adapterHealth?.length ? <div className="tauri-bot-adapters">{status.adapterHealth.map(adapter => <section className="tauri-bot-adapter" key={adapter.id}>
        <div className="tauri-bot-adapter-heading"><Bot size={17} /><strong>{adapter.name || adapter.domain || adapter.platform}</strong><span className={`tauri-bot-adapter-state is-${adapter.status}`}>{statusLabel(adapter.status)}</span></div>
        <small>{adapter.platform}{adapter.domain ? ` · ${adapter.domain}` : ""}{adapter.id ? ` · ${adapter.id}` : ""}</small>
        <div className="tauri-bot-adapter-metrics"><span>{t("settings.bots.messages", { count: adapter.messages })}</span><span>{t("settings.bots.sends", { count: adapter.sends })}</span>{adapter.send_errors > 0 && <span>{t("settings.bots.sendErrors", { count: adapter.send_errors })}</span>}</div>
        {adapter.last_error && <p className="tauri-diagnostic-error">{adapter.last_error}</p>}
      </section>)}</div> : <div className="tauri-settings-empty"><Activity size={18} /><span>{t("settings.bots.noAdapters")}</span></div>}
    </>}
  </div>;
}

function joinBotAllowlist(value: unknown): string {
  return Array.isArray(value)
    ? value.filter((entry): entry is string => typeof entry === "string").join("\n")
    : "";
}
