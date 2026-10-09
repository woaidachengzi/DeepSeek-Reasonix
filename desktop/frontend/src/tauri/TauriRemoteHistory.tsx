import { useCallback, useEffect, useRef, useState } from "react";
import { Transcript } from "../components/Transcript";
import { TauriRemoteImageScope } from "./TauriRemoteImageScope";
import { LocalPathActionContext, SourceReferenceContext } from "../components/SourceReferenceContext";
import { useT } from "../lib/i18n";
import { remoteHistoryItems } from "../lib/remoteHistoryItems";
import { createRemoteConversationProjection } from "../lib/remoteConversationProjection";
import { openRemoteSessionSnapshot, type RemoteSnapshotTransport } from "../lib/remoteSessionSnapshot";
import { nativeRemoteSnapshotTransport } from "../lib/nativeRemoteSessionSnapshot";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerView, BridgeRemoteControllerSessionView } from "../lib/bridgeProtocol.generated";
import type { Item, LiveStream } from "../lib/useController";

const noPrompt = () => {};

export function TauriRemoteHistory({lease,controller,sessionPath,title,onClose,transport=nativeRemoteSnapshotTransport}: {lease:RemoteControllerLease;controller:BridgeRemoteControllerView;sessionPath:string;title:string;onClose:()=>void;transport?:RemoteSnapshotTransport}) {
  const t = useT();
  const {id:controllerId,name:controllerName,workspace:controllerWorkspace,readOnly:controllerReadOnly}=controller;
  const surface = JSON.stringify([controllerId,controllerName,controllerWorkspace,sessionPath]);
  const [snapshot,setSnapshot] = useState<{surface:string;view:BridgeRemoteControllerSessionView;items:Item[];live?:LiveStream;running:boolean;revision:number} | null>(null);
  const [surfaceId] = useState(() => `remote:${crypto.randomUUID()}`);
  const generation = useRef(0);
  const owner = useRef<object | null>(null);
  const stream = useRef<ReturnType<typeof openRemoteSessionSnapshot> | null>(null);
  const [busy,setBusy] = useState(true);
  const [failed,setFailed] = useState(false);
  const [fileNotice,setFileNotice] = useState(false);
  const request = useRef<object | null>(null);
  const mounted = useRef(false);
  const load = useCallback(async () => {
    if (!mounted.current || request.current) return;
    const token = {}; request.current = token; owner.current = token;
    stream.current?.dispose();stream.current = null;
    const currentGeneration = ++generation.current;
    const alive = () => mounted.current && owner.current === token;
    setBusy(true); setFailed(false);
    try {
      const view = await lease.sessionView(sessionPath);
      if(!alive())return;
      if(view.ownership === "serve") {
        let projection:ReturnType<typeof createRemoteConversationProjection> | undefined;
        const publish = () => {
          if(!alive() || !projection)return;
          const display=projection.view();
          setSnapshot(previous => alive() ? {surface,view,...display,revision:(previous?.revision??0)+1} : previous);
        };
        const current = openRemoteSessionSnapshot(transport,{id:controllerId,name:controllerName,workspace:controllerWorkspace,readOnly:controllerReadOnly},{controllerId,sessionPath,surfaceId,generation:currentGeneration},{
          state:state => {if(!alive())return;
            if(state === "syncing")setBusy(true);
            else if(state === "live")setBusy(false);
            else if(state === "reconcile") {
            // A cancelled read may still be settling. It no longer owns the
            // refresh lock; its finally may not release a newer request.
            if(request.current === token)request.current=null;
            setFailed(true);setBusy(false);
          }},
          snapshot:cut => {if(!alive())return;projection=createRemoteConversationProjection(cut,surface,{protocol:t("notice.protocolRecoveryBody"),readiness:t("notice.finalReadiness")});publish();},
          event:frame => {if(!alive() || !projection)return;projection.event(frame);publish();},
        });
        stream.current = current;
        await current.settled;
        return;
      }
      const items = remoteHistoryItems(view,surface,{protocol:t("notice.protocolRecoveryBody"),readiness:t("notice.finalReadiness")});
      if (!alive()) return;
      setSnapshot(previous => alive() ? {surface,view,items,running:false,revision:(previous?.revision ?? 0)+1} : previous);
    } catch {
      if (mounted.current && request.current === token) { setFailed(true); setSnapshot(null); }
    } finally {
      if (mounted.current && request.current === token) { request.current = null; setBusy(false); }
    }
  },[lease,sessionPath,surface,surfaceId,t,transport,controllerId,controllerName,controllerWorkspace,controllerReadOnly]);
  useEffect(() => {
    mounted.current = true; request.current = null; setSnapshot(null); setFileNotice(false);
    void load();
    return () => { mounted.current = false; request.current = null;owner.current=null;stream.current?.dispose();stream.current=null; };
  },[load]);
  const blockedFile = useCallback(() => setFileNotice(true),[]);
  const current = snapshot?.surface === surface ? snapshot : null;
  return <section className="tauri-remote-history" aria-label={t("settings.remote.historyTitle")} aria-busy={busy}>
    <div className="tauri-remote-history-header"><h4>{title || t("settings.remote.historyTitle")}</h4>
      <button type="button" className="tauri-settings-button" onClick={onClose}>{t("common.close")}</button></div>
    <p className="tauri-settings-hint">{t(current?.view.ownership === "serve" ? "settings.remote.historyLiveReadOnly" : "settings.remote.historyReadOnly")}</p>
    <code className="tauri-remote-history-path">{sessionPath}</code>
    {current?.running ? <p>{t("settings.remote.sessionsRunning")}{current.view.modelRef ? ` · ${current.view.modelRef}` : ""}</p> : null}
    <p className="tauri-settings-hint" role={fileNotice ? "status" : undefined}>{t("settings.remote.historyMediaUnavailable")}</p>
    {busy ? <p role="status">{t("common.loading")}</p> : null}
    {failed ? <p role="alert" className="tauri-diagnostic-error">{t("settings.remote.historyFailed")}</p> : null}
    {current && current.items.length === 0 ? <p>{t("settings.remote.historyEmpty")}</p> : null}
    {current && current.items.length > 0 ? <div className="tauri-remote-history-viewport">
      <TauriRemoteImageScope lease={lease} sessionPath={sessionPath} surface={surface}>
        <LocalPathActionContext.Provider value={blockedFile}><SourceReferenceContext.Provider value={blockedFile}>
          <Transcript key={surface} items={current.items} tabId={`remote:${surface}`} geometrySessionKey={`remote:${surface}`}
            live={current.live} contentRevision={current.revision} onPrompt={noPrompt} actionPending rewindDisabled />
        </SourceReferenceContext.Provider></LocalPathActionContext.Provider>
      </TauriRemoteImageScope>
    </div> : null}
    <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void load()}>{t("settings.remote.historyRefresh")}</button>
  </section>;
}
