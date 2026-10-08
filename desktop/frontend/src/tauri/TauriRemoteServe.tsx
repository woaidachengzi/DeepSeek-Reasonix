import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { openTauriRemoteController, tauriMessageFrom, tauriRemoteServe, type TauriRemoteServeView } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

type ServeAction = "status" | "start" | "stop" | "logs" | "controller";
const RemoteSessions = lazy(() => import("./TauriRemoteSessions").then(module => ({default:module.TauriRemoteSessions})));

export function TauriRemoteServe({ name, workspace, credentialMode }: { name: string; workspace: string; credentialMode: string }) {
  const t = useT();
  const [view, setView] = useState<TauriRemoteServeView | null>(null);
  // The first paint precedes the status effect; don't expose briefly enabled
  // controls which can lose a click when that effect initializes the view.
  const [busy, setBusy] = useState<ServeAction | "">(workspace ? "status" : "");
  const [error, setError] = useState("");
  const [sessionsOpen,setSessionsOpen] = useState(false);
  const mounted = useRef(true);
  const epoch = useRef(0);

  const run = async (action: Exclude<ServeAction, "controller">) => {
    if (!workspace || busy) return;
    const requestEpoch = ++epoch.current;
    setBusy(action);
    setError("");
    try {
      const next = await tauriRemoteServe({ name, workspace, action, ...(action === "logs" ? { tailLines: 100 } : {}) });
      if (!mounted.current || requestEpoch !== epoch.current) return;
      setView(current => action === "logs" ? { ...(current ?? next), state: current?.state ?? "stopped", logs: next.logs ?? "" } : next);
      if (action === "stop") setSessionsOpen(false);
    } catch (cause) {
      if (mounted.current && requestEpoch === epoch.current) setError(tauriMessageFrom(cause));
    } finally {
      if (mounted.current && requestEpoch === epoch.current) setBusy("");
    }
  };

  const openController = async () => {
    if (!workspace || busy) return;
    const requestEpoch = ++epoch.current;
    setBusy("controller");
    setError("");
    try {
      await openTauriRemoteController(name, workspace);
      if (!mounted.current || requestEpoch !== epoch.current) return;
      const next = await tauriRemoteServe({ name, workspace, action: "status" });
      if (mounted.current && requestEpoch === epoch.current) setView(next);
    } catch (cause) {
      if (mounted.current && requestEpoch === epoch.current) setError(tauriMessageFrom(cause));
    } finally {
      if (mounted.current && requestEpoch === epoch.current) setBusy("");
    }
  };

  useEffect(() => {
    mounted.current = true;
    const requestEpoch = ++epoch.current;
    setView(null);
    setSessionsOpen(false);
    setError("");
    if (!workspace) {
      setBusy("");
      return () => { mounted.current = false; ++epoch.current; };
    }
    setBusy("status");
    void tauriRemoteServe({ name, workspace, action: "status" })
      .then(next => {
        if (mounted.current && requestEpoch === epoch.current) setView(next);
      })
      .catch(cause => {
        if (mounted.current && requestEpoch === epoch.current) setError(tauriMessageFrom(cause));
      })
      .finally(() => {
        if (mounted.current && requestEpoch === epoch.current) setBusy("");
      });
    return () => { mounted.current = false; ++epoch.current; };
  }, [name, workspace]);

  if (!workspace) {
    return <p className="tauri-settings-muted">{t("settings.remote.serveWorkspaceRequired")}</p>;
  }
  const localProxyUnavailable = credentialMode === "local-proxy";
  const state = view?.state ?? "stopped";
  return <details className="tauri-bot-channel-access tauri-remote-serve">
    <summary>{t("settings.remote.serve")}</summary>
    <div className="tauri-bot-channel-access__body">
      <p className="tauri-settings-hint">{t("settings.remote.serveHint")}</p>
      <p className="tauri-settings-hint">{t("settings.remote.controllerHint")}</p>
      <p>{t("settings.remote.serveState", { state: t(`settings.remote.serveState.${state}`) })}</p>
      {view?.localUrl && <p><code>{view.localUrl}</code></p>}
      {view?.message && <p className="tauri-settings-muted">{view.message}</p>}
      {localProxyUnavailable && <p className="tauri-diagnostic-error" role="alert">{t("settings.remote.serveLocalProxyUnavailable")}</p>}
      {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
      <div className="tauri-settings-actions">
        <button type="button" className="tauri-settings-button" disabled={Boolean(busy)} onClick={() => void run("status")}>{busy === "status" ? t("common.loading") : t("settings.bots.refresh")}</button>
        <button type="button" className="tauri-settings-button" disabled={Boolean(busy) || localProxyUnavailable} onClick={() => void run("start")}>{busy === "start" ? t("settings.remote.serveStarting") : t("settings.remote.serveStart")}</button>
        <button type="button" className="tauri-settings-button" disabled={Boolean(busy) || localProxyUnavailable} onClick={() => void openController()}>{busy === "controller" ? t("settings.remote.controllerOpening") : t("settings.remote.controllerOpen")}</button>
        <button type="button" className="tauri-settings-button" aria-expanded={sessionsOpen} disabled={Boolean(busy) || localProxyUnavailable} onClick={() => setSessionsOpen(current => !current)}>{t(sessionsOpen ? "settings.remote.sessionsClose" : "settings.remote.sessionsOpen")}</button>
        <button type="button" className="tauri-settings-button" disabled={Boolean(busy) || state === "stopped"} onClick={() => void run("stop")}>{busy === "stop" ? t("settings.remote.serveStopping") : t("settings.remote.serveStop")}</button>
        <button type="button" className="tauri-settings-button" disabled={Boolean(busy)} onClick={() => void run("logs")}>{busy === "logs" ? t("common.loading") : t("settings.remote.serveLogs")}</button>
      </div>
      {sessionsOpen ? <Suspense fallback={<p role="status">{t("common.loading")}</p>}><RemoteSessions key={JSON.stringify([name,workspace])} name={name} workspace={workspace}/></Suspense> : null}
      {view?.logs !== undefined && <pre className="tauri-remote-serve-logs">{view.logs || t("settings.remote.serveLogsEmpty")}</pre>}
    </div>
  </details>;
}
