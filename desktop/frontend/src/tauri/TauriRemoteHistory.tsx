import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Transcript } from "../components/Transcript";
import { MarkdownImageResolverContext } from "../components/MarkdownImageContext";
import { LocalPathActionContext, SourceReferenceContext } from "../components/SourceReferenceContext";
import { useT } from "../lib/i18n";
import { remoteHistoryItems } from "../lib/remoteHistoryItems";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerView, BridgeRemoteControllerSessionView } from "../lib/bridgeProtocol.generated";
import type { Item } from "../lib/useController";

const noPrompt = () => {};

export function TauriRemoteHistory({lease,controller,sessionPath,title,onClose}: {lease:RemoteControllerLease;controller:BridgeRemoteControllerView;sessionPath:string;title:string;onClose:()=>void}) {
  const t = useT();
  const surface = JSON.stringify([controller.id,controller.name,controller.workspace,sessionPath]);
  const [snapshot,setSnapshot] = useState<{surface:string;view:BridgeRemoteControllerSessionView;items:Item[];revision:number} | null>(null);
  const [busy,setBusy] = useState(true);
  const [failed,setFailed] = useState(false);
  const [fileNotice,setFileNotice] = useState(false);
  const request = useRef<object | null>(null);
  const mounted = useRef(false);
  const load = useCallback(async () => {
    if (!mounted.current || request.current) return;
    const token = {}; request.current = token; setBusy(true); setFailed(false);
    try {
      const view = await lease.sessionView(sessionPath);
      const items = remoteHistoryItems(view,surface,{protocol:t("notice.protocolRecoveryBody"),readiness:t("notice.finalReadiness")});
      if (!mounted.current || request.current !== token) return;
      setSnapshot(previous => ({surface,view,items,revision:(previous?.revision ?? 0)+1}));
    } catch {
      if (mounted.current && request.current === token) { setFailed(true); setSnapshot(null); }
    } finally {
      if (mounted.current && request.current === token) { request.current = null; setBusy(false); }
    }
  },[lease,sessionPath,surface,t]);
  useEffect(() => {
    mounted.current = true; request.current = null; setSnapshot(null); setFileNotice(false);
    void load();
    return () => { mounted.current = false; request.current = null; };
  },[load]);
  const blockedFile = useCallback(() => setFileNotice(true),[]);
  const imageOwner = useRef<object | null>(null);
  const imageScope = useMemo(() => ({}),[lease,sessionPath,surface]);
  useLayoutEffect(() => {
    imageOwner.current = imageScope;
    return () => { if (imageOwner.current === imageScope) imageOwner.current = null; };
  },[imageScope]);
  const resolveImage = useCallback(async (source:string) => {
    if (imageOwner.current !== imageScope) return {url:"",errorCode:"remote-preview-unavailable"};
    try {
      const image = await lease.sessionImage(sessionPath,source);
      if (imageOwner.current !== imageScope) return {url:"",errorCode:"remote-preview-unavailable"};
      return image;
    } catch { return {url:"",errorCode:"remote-preview-unavailable"}; }
  },[imageScope,lease,sessionPath]);
  const current = snapshot?.surface === surface ? snapshot : null;
  return <section className="tauri-remote-history" aria-label={t("settings.remote.historyTitle")} aria-busy={busy}>
    <div className="tauri-remote-history-header"><h4>{title || t("settings.remote.historyTitle")}</h4>
      <button type="button" className="tauri-settings-button" onClick={onClose}>{t("common.close")}</button></div>
    <p className="tauri-settings-hint">{t("settings.remote.historyReadOnly")}</p>
    <code className="tauri-remote-history-path">{sessionPath}</code>
    {current?.view.runtimeState?.running ? <p>{t("settings.remote.sessionsRunning")}{current.view.modelRef ? ` · ${current.view.modelRef}` : ""}</p> : null}
    <p className="tauri-settings-hint" role={fileNotice ? "status" : undefined}>{t("settings.remote.historyMediaUnavailable")}</p>
    {busy ? <p role="status">{t("common.loading")}</p> : null}
    {failed ? <p role="alert" className="tauri-diagnostic-error">{t("settings.remote.historyFailed")}</p> : null}
    {current && current.items.length === 0 ? <p>{t("settings.remote.historyEmpty")}</p> : null}
    {current && current.items.length > 0 ? <div className="tauri-remote-history-viewport">
      <MarkdownImageResolverContext.Provider value={resolveImage}>
        <LocalPathActionContext.Provider value={blockedFile}><SourceReferenceContext.Provider value={blockedFile}>
          <Transcript key={surface} items={current.items} tabId={`remote:${surface}`} geometrySessionKey={`remote:${surface}`}
            contentRevision={current.revision} onPrompt={noPrompt} actionPending rewindDisabled />
        </SourceReferenceContext.Provider></LocalPathActionContext.Provider>
      </MarkdownImageResolverContext.Provider>
    </div> : null}
    <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void load()}>{t("settings.remote.historyRefresh")}</button>
  </section>;
}
