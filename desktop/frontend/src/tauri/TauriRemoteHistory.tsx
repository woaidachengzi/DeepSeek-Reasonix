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
  const [snapshot,setSnapshot] = useState<{surface:string;view:BridgeRemoteControllerSessionView;items:Item[];live?:LiveStream;running:boolean;turnId?:string;eventSeq?:number;revision:number} | null>(null);
  const [surfaceId] = useState(() => `remote:${crypto.randomUUID()}`);
  const generation = useRef(0);
  const owner = useRef<object | null>(null);
  const stream = useRef<ReturnType<typeof openRemoteSessionSnapshot> | null>(null);
  const [busy,setBusy] = useState(true);
  const [failed,setFailed] = useState(false);
  const [fileNotice,setFileNotice] = useState(false);
  const request = useRef<object | null>(null);
  const mounted = useRef(false);
  const stopRequest=useRef<object|null>(null);
  const [stopState,setStopState]=useState<{surface:string;turnId:string;outcome:"pending"|"sent"|"unknown"}|null>(null);
  const sendRequest=useRef<object|null>(null);
  const displayIdentity=useRef<{surface:string;turnId?:string;eventSeq:number;running:boolean}|null>(null);
  const [draft,setDraft]=useState("");
  const [sendState,setSendState]=useState<{surface:string;outcome:"pending"|"sent"|"changed"|"unknown"}|null>(null);
  const load = useCallback(async () => {
    if (!mounted.current || request.current) return;
    const token = {}; request.current = token; owner.current = token;
    stopRequest.current=null;setStopState(null);
    sendRequest.current=null;setSendState(null);displayIdentity.current=null;
    stream.current?.dispose();stream.current = null;
    const currentGeneration = ++generation.current;
    const alive = () => mounted.current && owner.current === token;
    setBusy(true); setFailed(false);
    try {
      const view = await lease.sessionView(sessionPath);
      if(!alive())return;
      if(view.ownership === "serve") {
        let projection:ReturnType<typeof createRemoteConversationProjection> | undefined;
        let turnId:string|undefined;
        let eventSeq=0;
        const publish = () => {
          if(!alive() || !projection)return;
          const display=projection.view();
          displayIdentity.current={surface,turnId,eventSeq,running:display.running};
          setSnapshot(previous => alive() ? {surface,view,...display,turnId,eventSeq,revision:(previous?.revision??0)+1} : previous);
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
          snapshot:cut => {if(!alive())return;if(turnId!==cut.projection.activeTurnId){stopRequest.current=null;setStopState(null);}turnId=cut.projection.activeTurnId;eventSeq=cut.capturedThrough;projection=createRemoteConversationProjection(cut,surface,{protocol:t("notice.protocolRecoveryBody"),readiness:t("notice.finalReadiness")});publish();},
          event:frame => {if(!alive() || !projection)return;projection.event(frame);eventSeq=frame.seq as number;publish();},
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
    mounted.current = true; request.current = null; setSnapshot(null); setFileNotice(false);setDraft("");
    void load();
    return () => { mounted.current = false; request.current = null;owner.current=null;stopRequest.current=null;sendRequest.current=null;displayIdentity.current=null;stream.current?.dispose();stream.current=null; };
  },[load]);
  const blockedFile = useCallback(() => setFileNotice(true),[]);
  const current = snapshot?.surface === surface ? snapshot : null;
  const stopped=stopState?.surface===surface&&stopState.turnId===current?.turnId?stopState:null;
  const canStop=!failed&&!!lease.sessionCancel&&current?.view.ownership==="serve"&&current.running&&!!current.turnId&&!!current.view.runtimeState?.runtimeEpoch;
  const sending=sendState?.surface===surface?sendState:null;
  const hasComposer=!!lease.sessionSubmit&&current?.view.ownership==="serve"&&!!current.view.runtimeState?.runtimeEpoch;
  const canSend=hasComposer&&!failed&&!busy&&!current?.running&&sending?.outcome!=="pending"&&sending?.outcome!=="unknown"&&sending?.outcome!=="changed";
  const send=async()=>{
    if(!canSend||sendRequest.current||!draft.trim()||!lease.sessionSubmit||!current)return;
    const token={},sourceOwner=owner.current,baseTurn=current.turnId,baseSeq=current.eventSeq,text=draft,epoch=current.view.runtimeState!.runtimeEpoch;
    sendRequest.current=token;setSendState({surface,outcome:"pending"});
    const alive=()=>mounted.current&&owner.current===sourceOwner&&sendRequest.current===token;
    let dispatched=false;
    try {
      const fresh=await lease.sessionView(sessionPath);
      if(!alive())return;
      const state=fresh.runtimeState,display=displayIdentity.current;
      // Settled cuts intentionally omit activeTurnId. Their verified sequence,
      // not a guessed last-question identity, fences the fresh idle observation.
      if(fresh.sessionPath!==sessionPath||fresh.ownership!=="serve"||!state||state.schemaVersion!==1||state.runtimeEpoch!==epoch||state.phase!=="idle"||state.running||state.pendingPrompt||state.cancelRequested||!Number.isSafeInteger(state.revision)||state.revision<1||state.turnEventSeq!==baseSeq||(baseTurn!==undefined&&state.turnId!==baseTurn)||display?.surface!==surface||display.turnId!==baseTurn||display.eventSeq!==baseSeq||display.running)throw new Error("changed remote session");
      dispatched=true;
      await lease.sessionSubmit({sessionPath,runtimeEpoch:epoch,revision:state.revision,text});
      if(alive()){setDraft(previous=>previous===text?"":previous);setSendState({surface,outcome:"sent"});}
    } catch {if(alive())setSendState({surface,outcome:dispatched?"unknown":"changed"});}
    finally {if(alive())sendRequest.current=null;}
  };
  const stop=async()=>{
    if(!canStop||busy||stopRequest.current||stopped||!current?.turnId||!lease.sessionCancel)return;
    const token={},sourceOwner=owner.current,turnId=current.turnId;
    stopRequest.current=token;setStopState({surface,turnId,outcome:"pending"});
    const alive=()=>mounted.current&&owner.current===sourceOwner&&stopRequest.current===token;
    try {
      await lease.sessionCancel({sessionPath,runtimeEpoch:current.view.runtimeState!.runtimeEpoch,turnId});
      if(alive())setStopState({surface,turnId,outcome:"sent"});
    } catch {if(alive())setStopState({surface,turnId,outcome:"unknown"});}
    finally {if(alive())stopRequest.current=null;}
  };
  return <section className="tauri-remote-history" aria-label={t("settings.remote.historyTitle")} aria-busy={busy}>
    <div className="tauri-remote-history-header"><h4>{title || t("settings.remote.historyTitle")}</h4>
      <button type="button" className="tauri-settings-button" onClick={onClose}>{t("common.close")}</button></div>
    <p className="tauri-settings-hint">{t(hasComposer ? "settings.remote.historySendHint" : current?.view.ownership === "serve" ? "settings.remote.historyLiveReadOnly" : "settings.remote.historyReadOnly")}</p>
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
    {stopped?.outcome==="sent"&&current?.running?<p role="status">{t("settings.remote.stopSent")}</p>:null}
    {stopped?.outcome==="unknown"?<p role="alert" className="tauri-diagnostic-error">{t("settings.remote.stopUnknown")}</p>:null}
    <form className="tauri-remote-composer" hidden={!hasComposer} onSubmit={event=>{event.preventDefault();void send();}}>
      <textarea className="tauri-settings-input" aria-label={t("settings.remote.messageLabel")} placeholder={t("settings.remote.messageLabel")} value={draft} disabled={sending?.outcome==="pending"} onChange={event=>setDraft(event.target.value)} onKeyDown={event=>{if(event.key==="Enter"&&(event.ctrlKey||event.metaKey)&&!event.nativeEvent.isComposing){event.preventDefault();void send();}}}/>
      <button type="submit" className="tauri-settings-button" disabled={!canSend||!draft.trim()}>{t("settings.remote.sendMessage")}</button>
    </form>
    {sending?.outcome==="sent"?<p role="status">{t("settings.remote.sendSent")}</p>:null}
    {sending?.outcome==="unknown"||sending?.outcome==="changed"?<p role="alert" className="tauri-diagnostic-error">{t(sending.outcome==="unknown"?"settings.remote.sendUnknown":"settings.remote.sendChanged")}</p>:null}
    <button type="button" className="tauri-settings-button" hidden={!canStop} disabled={busy||!!stopped||sending?.outcome==="pending"} onClick={()=>void stop()}>{t("composer.stopShort")}</button>
    <button type="button" className="tauri-settings-button" disabled={busy||stopped?.outcome==="pending"||sending?.outcome==="pending"} onClick={() => void load()}>{t("settings.remote.historyRefresh")}</button>
  </section>;
}
