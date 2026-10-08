import { useEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import { nativeRemoteControllers } from "../lib/nativeRemoteControllers";
import type { RemoteControllerLease, RemoteControllerPool } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerView, BridgeRemoteControllerSession } from "../lib/bridgeProtocol.generated";

const PAGE_SIZE = 50;

export function TauriRemoteSessions({ name,workspace,pool = nativeRemoteControllers }: { name:string;workspace:string;pool?:RemoteControllerPool }) {
  const t = useT();
  const [view,setView] = useState<BridgeRemoteControllerView | null>(null);
  const [rows,setRows] = useState<BridgeRemoteControllerSession[]>([]);
  const [busy,setBusy] = useState(true);
  const [failed,setFailed] = useState(false);
  const [page,setPage] = useState(0);
  const scope = JSON.stringify([name,workspace]);
  const [dataScope,setDataScope] = useState(scope);
  const belongs = dataScope === scope;
  const loading = busy || !belongs;
  const lease = useRef<RemoteControllerLease | null>(null);
  const request = useRef<object | null>(null);

  useEffect(() => {
    const current = pool.acquire(name,workspace);
    const token = {};
    lease.current = current; request.current = token;
    setDataScope(JSON.stringify([name,workspace]));
    setView(null); setRows([]); setPage(0); setBusy(true); setFailed(false);
    void current.ready.then(async view => {
      const entries = await current.sessions();
      if (request.current !== token || lease.current !== current) return;
      setView(view); setRows(entries);
    }).catch(() => { if (request.current === token) setFailed(true); })
      .finally(() => { if (request.current === token) { request.current = null; setBusy(false); } });
    return () => {
      if (lease.current === current) { lease.current = null; request.current = null; }
      current.release();
    };
  },[name,workspace,pool]);

  const refresh = async () => {
    const current = lease.current;
    if (!current || request.current) return;
    const token = {}; request.current = token;
    setBusy(true); setFailed(false);
    try {
      const entries = await current.sessions();
      if (request.current !== token || lease.current !== current) return;
      setRows(entries); setPage(0);
    } catch { if (request.current === token) { setRows([]); setFailed(true); } }
    finally { if (request.current === token) { request.current = null; setBusy(false); } }
  };

  const count = Math.max(1,Math.ceil(rows.length/PAGE_SIZE));
  const visible = belongs ? rows.slice(page*PAGE_SIZE,(page+1)*PAGE_SIZE) : [];
  return <section className="tauri-remote-sessions" aria-label={t("settings.remote.sessionsTitle")} aria-busy={loading}>
    <h4>{t("settings.remote.sessionsTitle")}</h4>
    <p className="tauri-settings-hint">{t("settings.remote.sessionsReadOnly")}</p>
    {belongs && view ? <p className="tauri-settings-muted"><code>{view.name}: {view.workspace}</code></p> : null}
    {loading ? <p role="status">{t("common.loading")}</p> : null}
    {belongs && failed ? <p role="alert" className="tauri-diagnostic-error">{t("settings.remote.sessionsFailed")}</p> : null}
    {!loading && !failed && rows.length === 0 ? <p>{t("settings.remote.sessionsEmpty")}</p> : null}
    {belongs && !failed ? <ul className="tauri-remote-sessions-list">{visible.map(row => <li key={row.path}>
      <strong>{row.title || row.name}</strong><code title={row.path}>{row.path}</code>
      <small>{t("settings.remote.sessionsTurns",{count:row.turns})}{row.current ? ` · ${t("settings.remote.sessionsCurrent")}` : ""}{row.running ? ` · ${t("settings.remote.sessionsRunning")}` : ""}{row.takenOver ? ` · ${t("settings.remote.sessionsTakenOver")}` : ""}</small>
    </li>)}</ul> : null}
    <div className="tauri-settings-actions">
      <button type="button" className="tauri-settings-button" disabled={loading} onClick={() => void refresh()}>{t("settings.remote.sessionsRefresh")}</button>
      {belongs && !failed && count > 1 ? <><button type="button" className="tauri-settings-button" disabled={loading || page === 0} onClick={() => setPage(current => current-1)}>{t("settings.remote.sessionsPrevious")}</button><span>{t("settings.remote.sessionsPage",{page:page+1,count})}</span><button type="button" className="tauri-settings-button" disabled={loading || page+1 >= count} onClick={() => setPage(current => current+1)}>{t("settings.remote.sessionsNext")}</button></> : null}
    </div>
  </section>;
}
