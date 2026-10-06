import { useEffect, useRef, useState } from "react";
import { tauriRemoteForwards, tauriMessageFrom, type TauriRemoteForwards as ForwardView } from "../lib/tauriBridge";
import { useManagementT } from "./tauriManagementI18n";

export function TauriRemoteForwards({name}: {name: string}) {
  const t=useManagementT();
  const [view,setView]=useState<ForwardView|null>(null);
  const [id,setID]=useState("");
  const [localPort,setLocalPort]=useState("");
  const [remoteHost,setRemoteHost]=useState("127.0.0.1");
  const [remotePort,setRemotePort]=useState("");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const mounted=useRef(true);
  const operation=useRef(false);
  const run=async (action: "list"|"add"|"remove",removeID?:string) => {
    if(operation.current)return;
    operation.current=true;setBusy(true);setError("");
    try {
      const next=await tauriRemoteForwards(action==="add" ? {name,action,id,localPort:Number(localPort),remoteHost,remotePort:Number(remotePort)} : {name,action,id:removeID});
      if(!mounted.current)return;
      setView(next);if(action==="add"){setID("");setLocalPort("");setRemotePort("");}
    } catch(error){if(mounted.current)setError(tauriMessageFrom(error));}
    finally{operation.current=false;if(mounted.current)setBusy(false);}
  };
  useEffect(()=>{mounted.current=true;void run("list");return()=>{mounted.current=false;};},[name]);
  return <details className="tauri-bot-channel-access tauri-remote-forwards">
    <summary>{t("settings.remote.forwards")}</summary>
    <div className="tauri-bot-channel-access__body">
      <p className="tauri-settings-hint">{t("settings.remote.forwardsHint")}</p>
      {error&&<p role="alert" className="tauri-diagnostic-error">{error}</p>}
      <button type="button" className="tauri-settings-button" disabled={busy} onClick={()=>void run("list")}>{t("settings.bots.refresh")}</button>
      {view?.forwards.map(item=><div className="tauri-settings-actions" key={item.id}><code>{item.id}: {item.localAddress} → {item.remoteAddress}</code><span>{t(item.active?"settings.remote.forwardActive":"settings.remote.forwardInactive")}</span><button type="button" className="tauri-settings-button" disabled={busy} onClick={()=>void run("remove",item.id)}>{t("common.delete")}</button></div>)}
      <form onSubmit={event=>{event.preventDefault();void run("add");}}>
        <label>{t("settings.remote.forwardID")}<input className="tauri-settings-input" required maxLength={64} pattern={"[a-zA-Z0-9][a-zA-Z0-9._\\-]*"} value={id} disabled={busy} onChange={event=>setID(event.target.value)}/></label>
        <label>{t("settings.remote.localPort")}<input className="tauri-settings-input" type="number" required min={1} max={65535} value={localPort} disabled={busy} onChange={event=>setLocalPort(event.target.value)}/></label>
        <label>{t("settings.remote.remoteHost")}<input className="tauri-settings-input" required maxLength={253} value={remoteHost} disabled={busy} onChange={event=>setRemoteHost(event.target.value)}/></label>
        <label>{t("settings.remote.remotePort")}<input className="tauri-settings-input" type="number" required min={1} max={65535} value={remotePort} disabled={busy} onChange={event=>setRemotePort(event.target.value)}/></label>
        <button type="submit" className="tauri-settings-button" disabled={busy}>{t("settings.remote.addForward")}</button>
      </form>
    </div>
  </details>;
}
