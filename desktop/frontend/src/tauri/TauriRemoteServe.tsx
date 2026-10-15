import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { openTauriRemoteController, tauriMessageFrom, tauriRemoteServe, type TauriRemoteServeView } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

type ServeAction = "status" | "start" | "stop" | "logs" | "controller";
const RemoteSessions = lazy(() => import("./TauriRemoteSessions").then(module => ({default:module.TauriRemoteSessions})));
const RemoteForwards = lazy(() => import("./TauriRemoteForwards").then(module => ({default:module.TauriRemoteForwards})));

export function TauriRemoteServe({ name, workspace, credentialMode, fingerprint, onChooseWorkspace }: { name: string; workspace: string; credentialMode: string; fingerprint?: string; onChooseWorkspace?: () => void }) {
  const t = useT();
  const [view, setView] = useState<TauriRemoteServeView | null>(null);
  // The first paint precedes the status effect; don't expose briefly enabled
  // controls which can lose a click when that effect initializes the view.
  const [busy, setBusy] = useState<ServeAction | "">(workspace ? "status" : "");
  const [error, setError] = useState("");
  const [sessionsOpen,setSessionsOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const operation = useRef(false);
  const mounted = useRef(true);
  const epoch = useRef(0);

  const run = async (action: Exclude<ServeAction, "controller">) => {
    if (!workspace || busy || operation.current) return;
    operation.current = true;
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
      if (mounted.current && requestEpoch === epoch.current) { operation.current = false; setBusy(""); }
    }
  };

  const openController = async () => {
    if (!workspace || busy || operation.current) return;
    operation.current = true;
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
      if (mounted.current && requestEpoch === epoch.current) { operation.current = false; setBusy(""); }
    }
  };

  useEffect(() => {
    mounted.current = true;
    const requestEpoch = ++epoch.current;
    setView(null);
    setSessionsOpen(false);
    setAdvancedOpen(false);
    setError("");
    if (!workspace) {
      operation.current = false;
      setBusy("");
      return () => { mounted.current = false; ++epoch.current; };
    }
    setBusy("status");
    operation.current = true;
    void tauriRemoteServe({ name, workspace, action: "status" })
      .then(next => {
        if (mounted.current && requestEpoch === epoch.current) setView(next);
      })
      .catch(cause => {
        if (mounted.current && requestEpoch === epoch.current) setError(tauriMessageFrom(cause));
      })
      .finally(() => {
        if (mounted.current && requestEpoch === epoch.current) { operation.current = false; setBusy(""); }
      });
    return () => { mounted.current = false; ++epoch.current; };
  }, [name, workspace]);

  if (!workspace) {
    return <div className="tauri-remote-serve"><p className="tauri-settings-muted">{t("settings.remote.serveWorkspaceRequired")}</p>{onChooseWorkspace && <button type="button" className="tauri-settings-button" onClick={onChooseWorkspace}>{t("settings.remote.chooseWorkspace")}</button>}</div>;
  }
  const localProxyUnavailable = credentialMode === "local-proxy";
  const state = view?.state ?? "stopped";
  return <div className="tauri-remote-serve">
      <p className="tauri-settings-hint">{t("settings.remote.controllerHint")}</p>
      {localProxyUnavailable && <p className="tauri-diagnostic-error" role="alert">{t("settings.remote.serveLocalProxyUnavailable")}</p>}
      {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
      <div className="tauri-settings-actions">
        <button type="button" className="tauri-settings-button tauri-remote-workspace-enter" disabled={Boolean(busy) || localProxyUnavailable} onClick={() => void openController()}>{busy === "status" ? t("common.loading") : busy === "controller" ? t("settings.remote.controllerOpening") : t("settings.remote.controllerOpen")}</button>
        <button type="button" className="tauri-settings-button" aria-expanded={sessionsOpen} disabled={Boolean(busy) || localProxyUnavailable} onClick={() => setSessionsOpen(current => !current)}>{t(sessionsOpen ? "settings.remote.sessionsClose" : "settings.remote.sessionsOpen")}</button>
      </div>
      {sessionsOpen ? <Suspense fallback={<p role="status">{t("common.loading")}</p>}><RemoteSessions key={JSON.stringify([name,workspace])} name={name} workspace={workspace}/></Suspense> : null}
      <details className="tauri-remote-advanced" open={advancedOpen} onToggle={event => setAdvancedOpen(event.currentTarget.open)}>
        <summary>{t("settings.remote.advanced")}</summary>
        {advancedOpen && <div className="tauri-bot-channel-access__body">
          {fingerprint && <p className="tauri-settings-muted">{t("settings.remote.connectionVerified", { fingerprint })}</p>}
          <p className="tauri-settings-hint">{t("settings.remote.serveHint")}</p>
          <p>{t("settings.remote.serveState", { state: t(`settings.remote.serveState.${state}`) })}</p>
          {view?.localUrl && <p><code>{view.localUrl}</code></p>}
          {view?.message && <p className="tauri-settings-muted">{view.message}</p>}
          <div className="tauri-settings-actions">
            <button type="button" className="tauri-settings-button" disabled={Boolean(busy)} onClick={() => void run("status")}>{busy === "status" ? t("common.loading") : t("settings.bots.refresh")}</button>
            <button type="button" className="tauri-settings-button" disabled={Boolean(busy) || localProxyUnavailable} onClick={() => void run("start")}>{busy === "start" ? t("settings.remote.serveStarting") : t("settings.remote.serveStart")}</button>
            <button type="button" className="tauri-settings-button" disabled={Boolean(busy) || state === "stopped"} onClick={() => void run("stop")}>{busy === "stop" ? t("settings.remote.serveStopping") : t("settings.remote.serveStop")}</button>
            <button type="button" className="tauri-settings-button" disabled={Boolean(busy)} onClick={() => void run("logs")}>{busy === "logs" ? t("common.loading") : t("settings.remote.serveLogs")}</button>
          </div>
          <Suspense fallback={<p role="status">{t("common.loading")}</p>}><RemoteForwards name={name}/></Suspense>
          {view?.logs !== undefined && <pre className="tauri-remote-serve-logs">{view.logs || t("settings.remote.serveLogsEmpty")}</pre>}
        </div>}
      </details>
  </div>;
}
