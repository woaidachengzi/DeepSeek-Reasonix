import { useEffect, useRef, useState } from "react";
import { changeTauriNetworkSettings, tauriMessageFrom, tauriNetworkSettings, type TauriNetworkChange, type TauriNetworkSettings } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

const MODES = [
  { value: "auto", label: "settings.proxyMode.auto" },
  { value: "env", label: "settings.proxyMode.env" },
  { value: "custom", label: "settings.proxyMode.custom" },
  { value: "off", label: "settings.proxyMode.off" },
] as const;
const TYPES = ["http", "https", "socks5", "socks5h"] as const;

function draftFromView(view: TauriNetworkSettings): TauriNetworkChange {
  return {
    proxyMode: view.proxyMode,
    noProxy: view.noProxy,
    proxyType: view.proxyType || "socks5",
    proxyServer: view.proxyServer,
    proxyPort: view.proxyPort,
    proxyUsername: view.proxyUsername,
    proxyUrlAction: "keep",
    proxyUrl: "",
    proxyPasswordAction: "keep",
    proxyPassword: "",
  };
}

export function TauriNetworkSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [view, setView] = useState<TauriNetworkSettings | null>(null);
  const [draft, setDraft] = useState<TauriNetworkChange | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  const reload = () => {
    setLoading(true);
    setError("");
    void tauriNetworkSettings().then(next => { setView(next); setDraft(draftFromView(next)); })
      .catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const save = async () => {
    if (!draft || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriNetworkSettings(draft);
      setView(next);
      setDraft(draftFromView(next));
      setPendingApply(true);
      setNotice(t("settings.network.saved"));
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
        setNotice(t("settings.network.applied"));
      } else setError(t("settings.mcp.applyFailed"));
    } catch { setError(t("settings.mcp.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const dirty = Boolean(view && draft && JSON.stringify(draft) !== JSON.stringify(draftFromView(view)));
  const overrideURL = Boolean(view && draft && (draft.proxyUrlAction === "replace" || (view.proxyUrlSet && draft.proxyUrlAction === "keep")));
  return <div className="tauri-settings-section tauri-network-settings">
    <h3>{t("settings.proxyMode")}</h3>
    {loading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : view && draft ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.proxyMode")}<small>{t("settings.network.modeHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.proxyMode")}>{MODES.map(mode => <button type="button" role="radio" aria-checked={draft.proxyMode === mode.value} className={`tauri-settings-radio${draft.proxyMode === mode.value ? " is-active" : ""}`} key={mode.value} disabled={busy} onClick={() => setDraft({ ...draft, proxyMode: mode.value })}>{t(mode.label)}</button>)}</div></div>
      {draft.proxyMode === "custom" && <>
        <h3>{t("settings.proxyMode.custom")}</h3>
        <p>{t("settings.proxyUrlHint")}</p>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.proxyUrl")}<small>{view.proxyUrlSet && draft.proxyUrlAction === "keep" ? t("settings.network.keepHint") : t("settings.proxyUrlHint")}</small></span><div className="tauri-network-secret"><input className="tauri-settings-input" aria-label={t("settings.proxyUrl")} value={draft.proxyUrl} disabled={busy} maxLength={4096} placeholder={view.proxyUrlSet && draft.proxyUrlAction === "keep" ? t("settings.network.secretHidden") : "socks5://127.0.0.1:7890"} onChange={event => setDraft({ ...draft, proxyUrl: event.target.value, proxyUrlAction: event.target.value ? "replace" : "keep" })} />{(view.proxyUrlSet || draft.proxyUrlAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDraft({ ...draft, proxyUrl: "", proxyUrlAction: "clear" })}>{t("settings.network.clear")}</button>}</div></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.proxyType")}</span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.proxyType")}>{TYPES.map(type => <button type="button" role="radio" aria-checked={draft.proxyType === type} className={`tauri-settings-radio${draft.proxyType === type ? " is-active" : ""}`} key={type} disabled={busy || overrideURL} onClick={() => setDraft({ ...draft, proxyType: type })}>{type.toUpperCase()}</button>)}</div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-proxy-server">{t("settings.proxyServer")}</label><div className="tauri-network-inline"><input id="tauri-proxy-server" className="tauri-settings-input" aria-label={t("settings.proxyServer")} value={draft.proxyServer} disabled={busy || overrideURL} maxLength={2048} placeholder="127.0.0.1" onChange={event => setDraft({ ...draft, proxyServer: event.target.value })} /><input className="tauri-settings-input" aria-label={t("settings.proxyPort")} inputMode="numeric" value={draft.proxyPort || ""} disabled={busy || overrideURL} placeholder="7890" onChange={event => setDraft({ ...draft, proxyPort: Number(event.target.value) || 0 })} /></div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-proxy-username">{t("settings.proxyUsername")}</label><input id="tauri-proxy-username" className="tauri-settings-input" aria-label={t("settings.proxyUsername")} value={draft.proxyUsername} disabled={busy || overrideURL} maxLength={1024} onChange={event => setDraft({ ...draft, proxyUsername: event.target.value })} /></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.proxyPassword")}<small>{view.proxyPasswordSet && draft.proxyPasswordAction === "keep" ? t("settings.network.keepHint") : t("settings.network.passwordHint")}</small></span><div className="tauri-network-secret"><input className="tauri-settings-input" aria-label={t("settings.proxyPassword")} type="password" value={draft.proxyPassword} disabled={busy || overrideURL} maxLength={4096} placeholder={view.proxyPasswordSet && draft.proxyPasswordAction === "keep" ? t("settings.network.secretHidden") : "••••••••"} onChange={event => setDraft({ ...draft, proxyPassword: event.target.value, proxyPasswordAction: event.target.value ? "replace" : "keep" })} />{(view.proxyPasswordSet || draft.proxyPasswordAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy || overrideURL} onClick={() => setDraft({ ...draft, proxyPassword: "", proxyPasswordAction: "clear" })}>{t("settings.network.clear")}</button>}</div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-no-proxy">{t("settings.noProxy")}</label><input id="tauri-no-proxy" className="tauri-settings-input" aria-label={t("settings.noProxy")} value={draft.noProxy} disabled={busy} maxLength={4096} placeholder="localhost,127.0.0.1,.local" onChange={event => setDraft({ ...draft, noProxy: event.target.value })} /></div>
      </>}
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => setDraft(draftFromView(view))}>{t("common.cancel")}</button><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => void save()}>{t("settings.saveNetwork")}</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || dirty || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>{t("settings.mcp.applyCurrent")}</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>{t("common.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
